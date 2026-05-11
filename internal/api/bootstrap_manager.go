package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"lingma2api/internal/auth"
	"lingma2api/internal/proxy"
)

type BootstrapSession struct {
	ID        string             `json:"id"`
	Status    string             `json:"status"`
	Method    string             `json:"method"`
	Phase     string             `json:"phase,omitempty"`
	AuthURL   string             `json:"auth_url,omitempty"`
	Error     string             `json:"error,omitempty"`
	StartedAt time.Time          `json:"started_at"`
	ExpiresAt time.Time          `json:"expires_at,omitempty"`
	cancel    context.CancelFunc `json:"-"`
}

type BootstrapManager struct {
	mu         sync.Mutex
	sessions   map[string]*BootstrapSession
	authFile   string
	listenAddr string
	lingmaVer  string
}

func NewBootstrapManager(authFile, listenAddr, lingmaVer string) *BootstrapManager {
	if listenAddr == "" {
		listenAddr = "127.0.0.1:37510"
	}
	return &BootstrapManager{
		sessions:   make(map[string]*BootstrapSession),
		authFile:   authFile,
		listenAddr: listenAddr,
		lingmaVer:  lingmaVer,
	}
}

func (m *BootstrapManager) StartOAuth() (*BootstrapSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Extract port from listenAddr for the Lingma login URL callback
	_, port, err := net.SplitHostPort(m.listenAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid listen addr: %w", err)
	}

	// Build Lingma V2 login entry URL (no client_id required)
	loginURL, state, _, err := auth.BuildLingmaLoginEntryURL(auth.LingmaLoginEntryConfig{
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("build lingma login url: %w", err)
	}

	// Rewrite the port in the login URL to match our listener
	loginURL, err = auth.RewriteLingmaLoginURLPort(loginURL, m.listenAddr)
	if err != nil {
		return nil, fmt.Errorf("rewrite login url port: %w", err)
	}

	// Wrap in three-layer nested URL for browser
	browserURL, err := auth.WrapLingmaLoginURLForBrowser(loginURL)
	if err != nil {
		return nil, fmt.Errorf("wrap login url for browser: %w", err)
	}

	id := newSessionID()
	sess := &BootstrapSession{
		ID:        id,
		Status:    "running",
		Method:    "oauth",
		AuthURL:   browserURL,
		StartedAt: time.Now(),
	}
	m.sessions[id] = sess

	go m.runOAuthFlow(id, state)

	return sess, nil
}

func (m *BootstrapManager) runOAuthFlow(id, state string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	m.updateSessionWithPhase(id, "running", "waiting_callback", "")

	capture, err := auth.WaitForCallback(ctx, m.listenAddr, "/auth/callback")
	if err != nil {
		m.updateSession(id, "error", fmt.Sprintf("wait for callback: %v", err))
		return
	}

	m.updateSessionWithPhase(id, "running", "parsing_credentials", "")

	// Parse V2/V1 callback data
	result, err := auth.ParseCallbackV2(capture.Query)
	if err != nil {
		// Try page capture fallback: check if Body contains user_info
		if capture.Body != nil {
			m.updateSessionWithPhase(id, "running", "parsing_page_capture", "")
			result, err = m.parsePageCapture(capture.Body)
			if err != nil {
				m.updateSession(id, "error", fmt.Sprintf("parse callback: %v", err))
				return
			}
		} else {
			m.updateSession(id, "error", fmt.Sprintf("parse callback: %v", err))
			return
		}
	}

	// Validate essential fields
	if result.SecurityOAuthToken == "" {
		m.updateSession(id, "error", "callback missing security_oauth_token")
		return
	}

	m.updateSessionWithPhase(id, "running", "generating_cosy", "")

	// Generate COSY credentials locally
	cosyKey, encryptUserInfo, cosyErr := auth.GenerateCosyCredentials(auth.CosyCredentialInput{
		Name:               result.Name,
		UID:                result.UID,
		AID:                result.AID,
		SecurityOAuthToken: result.SecurityOAuthToken,
		RefreshToken:       result.RefreshToken,
	})

	machineID := auth.NewMachineID()
	expireMs := ""
	if result.ExpireTime > 0 {
		expireMs = fmt.Sprintf("%d", result.ExpireTime)
	}

	now := time.Now().Format(time.RFC3339)
	stored := proxy.StoredCredentialFile{
		SchemaVersion:     1,
		Source:            "project_bootstrap",
		LingmaVersionHint: m.lingmaVer,
		ObtainedAt:        now,
		UpdatedAt:         now,
		TokenExpireTime:   expireMs,
		Auth: proxy.StoredAuthFields{
			UserID:    result.UID,
			MachineID: machineID,
		},
		OAuth: proxy.StoredOAuthFields{
			AccessToken:  result.SecurityOAuthToken,
			RefreshToken: result.RefreshToken,
		},
	}

	if cosyErr == nil {
		stored.Auth.CosyKey = cosyKey
		stored.Auth.EncryptUserInfo = encryptUserInfo
	} else {
		// Fallback to remote derivation for COSY credentials
		m.updateSessionWithPhase(id, "running", "deriving_remote", "")
		remoteStored, remoteErr := auth.DeriveCredentialsRemotely(auth.RemoteLoginConfig{
			AccessToken:   result.SecurityOAuthToken,
			RefreshToken:  result.RefreshToken,
			UserID:        result.UID,
			Username:      result.Name,
			MachineID:     machineID,
			TokenExpireMs: expireMs,
		})
		if remoteErr == nil {
			stored.Auth.CosyKey = remoteStored.Auth.CosyKey
			stored.Auth.EncryptUserInfo = remoteStored.Auth.EncryptUserInfo
		}
	}

	m.updateSessionWithPhase(id, "running", "saving", "")

	if err := auth.SaveCredentialFile(m.authFile, stored); err != nil {
		m.updateSession(id, "error", fmt.Sprintf("save credentials: %v", err))
		return
	}

	m.updateSession(id, "completed", "")
}

func (m *BootstrapManager) StartWS() (*BootstrapSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.authFile)
	if err != nil {
		return nil, fmt.Errorf("no existing credentials to refresh: %w", err)
	}

	var stored proxy.StoredCredentialFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	if stored.OAuth.AccessToken == "" || stored.OAuth.RefreshToken == "" {
		return nil, fmt.Errorf("existing credentials missing oauth tokens (not a remote-login credential)")
	}

	id := newSessionID()
	sess := &BootstrapSession{
		ID:        id,
		Status:    "running",
		Method:    "ws",
		StartedAt: time.Now(),
	}
	m.sessions[id] = sess

	go m.runWSFlow(id, stored)

	return sess, nil
}

func (m *BootstrapManager) runWSFlow(id string, stored proxy.StoredCredentialFile) {
	var expireTime int64
	if stored.TokenExpireTime != "" {
		if v, err := strconv.ParseInt(stored.TokenExpireTime, 10, 64); err == nil {
			expireTime = v
		}
	}

	refresher := &auth.WSRefresher{}
	result, err := refresher.Refresh(context.Background(), stored)
	if err != nil {
		m.updateSession(id, "error", fmt.Sprintf("WebSocket refresh: %v", err))
		return
	}

	expireMs := fmt.Sprintf("%d", result.ExpireTime)
	if result.ExpireTime == 0 && expireTime > 0 {
		expireMs = fmt.Sprintf("%d", expireTime)
	}

	machineID := stored.Auth.MachineID
	if machineID == "" {
		machineID = auth.NewMachineID()
	}

	userID := stored.Auth.UserID
	if result.UserID != "" {
		userID = result.UserID
	}

	// Derive fresh cosy_key and encrypt_user_info. Use the Lingma binary
	// when available (most reliable), fall back to remote API.
	newStored, err := m.deriveCredentials(result.AccessToken, result.RefreshToken, userID, machineID, expireMs)
	if err != nil {
		m.updateSession(id, "error", fmt.Sprintf("derive credentials: %v", err))
		return
	}

	if err := auth.SaveCredentialFile(m.authFile, newStored); err != nil {
		m.updateSession(id, "error", fmt.Sprintf("save credentials: %v", err))
		return
	}

	m.updateSession(id, "completed", "")
}

func (m *BootstrapManager) deriveCredentials(accessToken, refreshToken, userID, machineID, expireMs string) (proxy.StoredCredentialFile, error) {
	// 1. Prefer local COSY credential generation (most reliable, no network needed)
	cosyKey, encryptUserInfo, err := auth.GenerateCosyCredentials(auth.CosyCredentialInput{
		UID:                userID,
		SecurityOAuthToken: accessToken,
		RefreshToken:       refreshToken,
	})
	if err == nil && cosyKey != "" {
		now := time.Now().Format(time.RFC3339)
		return proxy.StoredCredentialFile{
			SchemaVersion:     1,
			Source:            "project_bootstrap",
			LingmaVersionHint: m.lingmaVer,
			ObtainedAt:        now,
			UpdatedAt:         now,
			TokenExpireTime:   expireMs,
			Auth: proxy.StoredAuthFields{
				CosyKey:         cosyKey,
				EncryptUserInfo: encryptUserInfo,
				UserID:          userID,
				MachineID:       machineID,
			},
			OAuth: proxy.StoredOAuthFields{
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
			},
		}, nil
	}

	// 2. Fall back to Lingma binary when available
	lingmaBin, binErr := auth.DefaultLingmaBinary()
	if binErr == nil {
		stored, lingmaErr := auth.DeriveCredentialsWithLingma(auth.LingmaBridgeConfig{
			LingmaBinary:  lingmaBin,
			AccessToken:   accessToken,
			RefreshToken:  refreshToken,
			UserID:        userID,
			TokenExpireMs: expireMs,
		})
		if lingmaErr == nil {
			return stored, nil
		}
	}

	// 3. Fall back to remote derivation
	return auth.DeriveCredentialsRemotely(auth.RemoteLoginConfig{
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		UserID:        userID,
		MachineID:     machineID,
		TokenExpireMs: expireMs,
	})
}

func (m *BootstrapManager) AuthFile() string {
	return m.authFile
}

func (m *BootstrapManager) GetStatus(id string) *BootstrapSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil
	}

	// Clean up sessions older than 10 minutes
	if time.Since(sess.StartedAt) > 10*time.Minute {
		delete(m.sessions, id)
	}

	// Return a copy so callers reading fields after the lock is released
	// don't race with concurrent updaters. The cancel field is non-serializable
	// and intentionally omitted from copies.
	snapshot := *sess
	snapshot.cancel = nil
	return &snapshot
}

func (m *BootstrapManager) updateSession(id, status, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.Status = status
		s.Error = errMsg
	}
}

func (m *BootstrapManager) updateSessionWithPhase(id, status, phase, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.Status = status
		s.Phase = phase
		s.Error = errMsg
	}
}

// parsePageCapture attempts to extract user info from a POST body
// sent by the bookmarklet / page capture mechanism.
func (m *BootstrapManager) parsePageCapture(body []byte) (*auth.CallbackV2Result, error) {
	var payload struct {
		UserInfo string `json:"userInfo"`
		LoginURL string `json:"loginUrl"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse page capture JSON: %w", err)
	}
	if payload.UserInfo == "" {
		return nil, fmt.Errorf("page capture missing userInfo")
	}

	// Try to parse as URL-encoded query string (browser location.search format)
	// The userInfo might be raw text or URL with embedded params
	raw := payload.UserInfo
	if strings.Contains(raw, "?") {
		parts := strings.SplitN(raw, "?", 2)
		raw = parts[1]
	}
	query, err := url.ParseQuery(raw)
	if err == nil && (query.Get("auth") != "" || query.Get("uid") != "") {
		return auth.ParseCallbackV2(query)
	}

	return nil, fmt.Errorf("page capture userInfo format not recognized")
}

// findActiveLocked returns the first session in a non-terminal state, or nil.
// Caller must hold m.mu.
func (m *BootstrapManager) findActiveLocked() *BootstrapSession {
	for _, s := range m.sessions {
		switch s.Status {
		case "running", "awaiting_callback", "deriving":
			return s
		}
	}
	return nil
}

// Start dispatches to the requested bootstrap method. Empty/auto runs the
// fallback chain: remote_callback -> ws.
//
// The standard OAuth grant path (oauth2/v1/auth + oauth.alibabacloud.com/v1/token)
// is not used: Lingma's real flow runs against devops.aliyun.com/lingma/login
// (server-injected client_id) and DeriveCredentialsRemotely against
// /api/v3/user/login. See docs/topics/callback-37510-simulation.md.
func (m *BootstrapManager) Start(method string) (*BootstrapSession, error) {
	switch method {
	case "", "auto":
		if s, err := m.StartRemoteCallback(); err == nil {
			return s, nil
		}
		return m.StartWS()
	case "ws":
		return m.StartWS()
	case "remote_callback":
		return m.StartRemoteCallback()
	default:
		return nil, fmt.Errorf("invalid method: %s", method)
	}
}

// Cancel cancels an in-flight session by id. Only running, awaiting_callback,
// and deriving states are cancellable. The session's context is cancelled and
// its status is synchronously set to "cancelled".
func (m *BootstrapManager) Cancel(id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session not found")
	}
	switch sess.Status {
	case "running", "awaiting_callback", "deriving":
		// fall through
	default:
		m.mu.Unlock()
		return fmt.Errorf("session already %s", sess.Status)
	}
	cancel := sess.cancel
	sess.cancel = nil
	sess.Status = "cancelled"
	sess.Error = ""
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return nil
}

func newSessionID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}
