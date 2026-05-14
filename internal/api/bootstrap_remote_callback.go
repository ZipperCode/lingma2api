package api

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"lingma2api/internal/auth"
	"lingma2api/internal/proxy"
)

const remoteCallbackTimeout = 5 * time.Minute

var remoteCallbackTimeoutForTest = remoteCallbackTimeout

func (m *BootstrapManager) StartRemoteCallback() (*BootstrapSession, error) {
	m.mu.Lock()
	if existing := m.findActiveLocked(); existing != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("another bootstrap in progress (id=%s)", existing.ID)
	}

	port, err := portFromAddr(m.listenAddr)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("invalid callback addr: %w", err)
	}

	machineID := auth.NewMachineID()
	loginURL, _, _, err := auth.BuildLingmaLoginEntryURL(auth.LingmaLoginEntryConfig{
		MachineID: machineID,
		Port:      port,
	})
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("build lingma login url: %w", err)
	}
	browserURL, err := auth.WrapLingmaLoginURLForBrowser(loginURL)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("wrap login url: %w", err)
	}

	timeout := remoteCallbackTimeoutForTest
	now := time.Now()
	id := newSessionID()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	sess := &BootstrapSession{
		ID:        id,
		Status:    "awaiting_callback_url",
		Method:    "remote_callback",
		AuthURL:   browserURL,
		StartedAt: now,
		ExpiresAt: now.Add(timeout),
		cancel:    cancel,
		machineID: machineID,
	}
	m.sessions[id] = sess
	snapshot := *sess
	snapshot.cancel = nil
	m.mu.Unlock()

	go func() {
		<-ctx.Done()
		if ctx.Err() != context.DeadlineExceeded {
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		current, ok := m.sessions[id]
		if !ok || current.Status != "awaiting_callback_url" {
			return
		}
		current.Status = "error"
		current.Error = "timeout: user did not complete login within 5m"
		current.Phase = ""
		current.cancel = nil
	}()

	return &snapshot, nil
}

func (m *BootstrapManager) SubmitCallbackURL(id, rawURL string) (*BootstrapSession, error) {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse callback url: %w", err)
	}
	if parsedURL.Scheme != "http" {
		return nil, fmt.Errorf("callback url must use http")
	}
	if parsedURL.Host != m.listenAddr {
		return nil, fmt.Errorf("callback url host must be %s", m.listenAddr)
	}

	m.mu.Lock()
	sess, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("session not found")
	}
	if !sess.ExpiresAt.IsZero() && time.Now().After(sess.ExpiresAt) {
		sess.Status = "error"
		sess.Phase = ""
		sess.Error = "timeout: user did not complete login within 5m"
		snapshot := *sess
		snapshot.cancel = nil
		m.mu.Unlock()
		return &snapshot, fmt.Errorf("%s", sess.Error)
	}
	if sess.Status == "cancelled" {
		m.mu.Unlock()
		return nil, fmt.Errorf("session already cancelled")
	}
	if sess.Status == "completed" {
		m.mu.Unlock()
		return nil, fmt.Errorf("session already completed")
	}
	if sess.Status != "awaiting_callback_url" && sess.Status != "error" {
		m.mu.Unlock()
		return nil, fmt.Errorf("session already %s", sess.Status)
	}
	machineID := sess.machineID
	sess.Status = "running"
	sess.Phase = "parsing_callback"
	sess.Error = ""
	m.mu.Unlock()

	result, err := auth.ParseCallbackV2FromURL(rawURL)
	if err != nil {
		m.updateSession(id, "error", fmt.Sprintf("parse callback url: %v", err))
		return m.GetStatus(id), err
	}
	if result.SecurityOAuthToken == "" {
		err = fmt.Errorf("callback missing security_oauth_token")
		m.updateSession(id, "error", err.Error())
		return m.GetStatus(id), err
	}

	stored, expireTime, err := m.buildStoredCredentialFromCallback(id, result, machineID)
	if err != nil {
		return m.GetStatus(id), err
	}

	m.updateSessionWithPhase(id, "running", "saving", "")
	if err := auth.SaveCredentialFile(m.authFile, stored); err != nil {
		m.updateSession(id, "error", fmt.Sprintf("save credentials: %v", err))
		return m.GetStatus(id), err
	}

	m.logAndReload(id, stored.Auth.UserID, result.AID, result.Name, stored.Auth.CosyKey, stored.Auth.MachineID, expireTime)
	return m.GetStatus(id), nil
}

func (m *BootstrapManager) buildStoredCredentialFromCallback(id string, result *auth.CallbackV2Result, machineID string) (proxy.StoredCredentialFile, string, error) {
	m.updateSessionWithPhase(id, "running", "generating_cosy", "")

	cosyKey, encryptUserInfo, err := auth.GenerateCosyCredentials(auth.CosyCredentialInput{
		Name:               result.Name,
		UID:                result.UID,
		AID:                result.AID,
		SecurityOAuthToken: result.SecurityOAuthToken,
		RefreshToken:       result.RefreshToken,
	})

	expireTime := ""
	if result.ExpireTime > 0 {
		expireTime = fmt.Sprintf("%d", result.ExpireTime)
	}

	if err == nil && cosyKey != "" && encryptUserInfo != "" {
		now := time.Now().Format(time.RFC3339)
		return proxy.StoredCredentialFile{
			SchemaVersion:     1,
			Source:            "oauth_v2_manual_callback",
			LingmaVersionHint: m.lingmaVer,
			ObtainedAt:        now,
			UpdatedAt:         now,
			TokenExpireTime:   expireTime,
			Auth: proxy.StoredAuthFields{
				CosyKey:         cosyKey,
				EncryptUserInfo: encryptUserInfo,
				UserID:          result.UID,
				MachineID:       machineID,
			},
			OAuth: proxy.StoredOAuthFields{
				AccessToken:  result.SecurityOAuthToken,
				RefreshToken: result.RefreshToken,
			},
		}, expireTime, nil
	}

	m.updateSessionWithPhase(id, "running", "deriving_remote", "")
	stored, remoteErr := auth.DeriveCredentialsRemotely(auth.RemoteLoginConfig{
		AccessToken:   result.SecurityOAuthToken,
		RefreshToken:  result.RefreshToken,
		UserID:        result.UID,
		Username:      result.Name,
		MachineID:     machineID,
		TokenExpireMs: expireTime,
	})
	if remoteErr != nil {
		m.updateSession(id, "error", fmt.Sprintf("derive credentials: %v", remoteErr))
		return proxy.StoredCredentialFile{}, "", remoteErr
	}
	if stored.Auth.UserID == "" {
		stored.Auth.UserID = result.UID
	}
	if stored.Auth.MachineID == "" {
		stored.Auth.MachineID = machineID
	}
	if stored.TokenExpireTime == "" {
		stored.TokenExpireTime = expireTime
	}
	return stored, stored.TokenExpireTime, nil
}

func portFromAddr(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "", fmt.Errorf("invalid addr %q", addr)
	}
	return port, nil
}
