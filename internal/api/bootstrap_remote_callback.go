package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"lingma2api/internal/auth"
)

// remoteCallbackTimeout bounds how long a Remote Callback bootstrap session
// waits for the user to complete the browser-side flow.
const remoteCallbackTimeout = 5 * time.Minute

// remoteCallbackTimeoutForTest can be overridden by tests to shorten the
// timeout. Defaults to remoteCallbackTimeout for production code.
var remoteCallbackTimeoutForTest = remoteCallbackTimeout

// StartRemoteCallback starts a "no client_id, no local Lingma" bootstrap
// session. It builds a Lingma login URL, opens a 127.0.0.1:37510 callback
// server with auto-inject HTML, and once user_info arrives derives credentials
// remotely via DeriveCredentialsRemotely.
//
// Preconditions: none (no client_id, no local Lingma binary required).
func (m *BootstrapManager) StartRemoteCallback() (*BootstrapSession, error) {
	m.mu.Lock()
	if existing := m.findActiveLocked(); existing != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("another bootstrap in progress (id=%s)", existing.ID)
	}

	port, err := portFromAddr(m.listenAddr)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("invalid listen addr: %w", err)
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
		Status:    "running",
		Method:    "remote_callback",
		AuthURL:   browserURL,
		StartedAt: now,
		ExpiresAt: now.Add(timeout),
		cancel:    cancel,
	}
	m.sessions[id] = sess
	snapshot := *sess
	snapshot.cancel = nil
	m.mu.Unlock()

	go m.runRemoteCallbackFlow(ctx, id, machineID)

	return &snapshot, nil
}

// runRemoteCallbackFlow executes the full callback → derive → save chain.
// It transitions session.Status through awaiting_callback → deriving →
// completed/error/cancelled.
func (m *BootstrapManager) runRemoteCallbackFlow(ctx context.Context, id, machineID string) {
	defer func() {
		m.mu.Lock()
		if s, ok := m.sessions[id]; ok {
			s.cancel = nil
		}
		m.mu.Unlock()
	}()

	m.updateSession(id, "awaiting_callback", "")

	port := portFromAddrOrEmpty(m.listenAddr)
	allowedOrigins := []string{
		"http://" + m.listenAddr,
		"http://127.0.0.1:" + port,
		"",
		"null",
	}

	capture, err := auth.WaitForCallbackWithOptions(ctx, m.listenAddr, "/auth/callback", auth.WaitForCallbackOptions{
		AllowedOrigins: allowedOrigins,
		AutoInjectHTML: true,
	})
	if err != nil {
		// Distinguish cancellation from timeout. If Cancel was called, status
		// has already been set to "cancelled" — don't overwrite it.
		if cancelled := m.statusIs(id, "cancelled"); cancelled {
			return
		}
		switch ctx.Err() {
		case context.Canceled:
			m.updateSession(id, "cancelled", "")
		case context.DeadlineExceeded:
			m.updateSession(id, "error", "timeout: user did not complete login within 5m")
		default:
			m.updateSession(id, "error", fmt.Sprintf("wait for callback: %v", err))
		}
		return
	}

	if len(capture.Body) == 0 {
		m.updateSession(id, "error", fmt.Sprintf("callback did not contain user_info body (path=%s)", capture.Path))
		return
	}

	var submission struct {
		UserInfo string `json:"userInfo"`
		LoginURL string `json:"loginUrl"`
	}
	if err := json.Unmarshal(capture.Body, &submission); err != nil {
		m.updateSession(id, "error", fmt.Sprintf("parse user_info failed: %v", err))
		return
	}
	if submission.UserInfo == "" {
		m.updateSession(id, "error", "submit-userinfo body missing userInfo")
		return
	}

	extracted, err := auth.ExtractFromCallbackPage(submission.UserInfo, submission.LoginURL)
	if err != nil {
		m.updateSession(id, "error", fmt.Sprintf("extract from callback page: %v", err))
		return
	}

	if extracted.MachineID == "" {
		extracted.MachineID = machineID
	}

	m.updateSession(id, "deriving", "")

	stored, err := auth.DeriveCredentialsRemotely(auth.RemoteLoginConfig{
		AccessToken:   extracted.AccessToken,
		RefreshToken:  extracted.RefreshToken,
		UserID:        extracted.UserID,
		Username:      extracted.Username,
		MachineID:     extracted.MachineID,
		TokenExpireMs: extracted.TokenExpireMs,
	})
	if err != nil {
		m.updateSession(id, "error", fmt.Sprintf("derive credentials: %v", err))
		return
	}

	if stored.Auth.UserID == "" {
		stored.Auth.UserID = extracted.UserID
	}
	if stored.Auth.MachineID == "" {
		stored.Auth.MachineID = extracted.MachineID
	}

	if err := auth.SaveCredentialFile(m.authFile, stored); err != nil {
		m.updateSession(id, "error", fmt.Sprintf("save credentials: %v", err))
		return
	}

	m.updateSession(id, "completed", "")
}

// statusIs returns true if the named session's current Status equals want.
func (m *BootstrapManager) statusIs(id, want string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return ok && s.Status == want
}

// portFromAddr extracts the port from a host:port listen address.
func portFromAddr(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "", fmt.Errorf("invalid addr %q", addr)
	}
	return port, nil
}

// portFromAddrOrEmpty returns the port portion of addr, or "" on parse failure.
func portFromAddrOrEmpty(addr string) string {
	if p, err := portFromAddr(addr); err == nil {
		return p
	}
	return ""
}
