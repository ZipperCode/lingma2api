package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"lingma2api/internal/config"
)

// TokenRefreshFn is called when credentials need refreshing.
type TokenRefreshFn func(ctx context.Context) error

type CredentialManager struct {
	mu      sync.RWMutex
	cfg     config.CredentialConfig
	now     func() time.Time
	current CredentialSnapshot
	loaded  bool
	// refreshFn is called when token is expired; if nil, no auto-refresh.
	refreshFn TokenRefreshFn
}

func NewCredentialManager(cfg config.CredentialConfig, now func() time.Time) *CredentialManager {
	if now == nil {
		now = time.Now
	}
	if cfg.AuthFile == "" {
		cfg.AuthFile = "./auth/credentials.json"
	}
	return &CredentialManager{
		cfg: cfg,
		now: now,
	}
}

// SetRefreshFn sets the callback used for auto-refreshing expired tokens.
func (manager *CredentialManager) SetRefreshFn(fn TokenRefreshFn) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.refreshFn = fn
}

func (manager *CredentialManager) Current(ctx context.Context) (CredentialSnapshot, error) {
	manager.mu.RLock()
	snapshot := manager.current
	loaded := manager.loaded
	refreshFn := manager.refreshFn
	manager.mu.RUnlock()

	if loaded {
		// Check if token is expired and auto-refresh if possible
		if snapshot.IsTokenExpired(5*time.Minute) && refreshFn != nil {
			if err := refreshFn(ctx); err == nil {
				// Refresh successful, reload snapshot
				return manager.Refresh(ctx)
			}
			// Refresh failed, return current (caller may retry or use anyway)
		}
		return snapshot, nil
	}

	return manager.Refresh(ctx)
}

func (manager *CredentialManager) Refresh(_ context.Context) (CredentialSnapshot, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	snapshot, err := manager.loadSnapshot()
	if err != nil {
		return CredentialSnapshot{}, err
	}

	manager.current = snapshot
	manager.loaded = true
	return snapshot, nil
}

func (manager *CredentialManager) Status() CredentialStatus {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	return CredentialStatus{
		Loaded:         manager.loaded,
		HasCredentials: manager.current.CosyKey != "" && manager.current.EncryptUserInfo != "",
		Source:         manager.current.Source,
		LoadedAt:       manager.current.LoadedAt,
		TokenExpired:   manager.current.IsTokenExpired(5 * time.Minute),
	}
}

// StoredMeta reads the credential file and returns file-level metadata
// without exposing sensitive field values.
func (manager *CredentialManager) StoredMeta() StoredMetaInfo {
	if manager.cfg.AuthFile == "" {
		return StoredMetaInfo{}
	}
	data, err := os.ReadFile(manager.cfg.AuthFile)
	if err != nil {
		return StoredMetaInfo{}
	}
	var stored StoredCredentialFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return StoredMetaInfo{}
	}
	return StoredMetaInfo{
		SchemaVersion:     stored.SchemaVersion,
		Source:            stored.Source,
		LingmaVersionHint: stored.LingmaVersionHint,
		ObtainedAt:        stored.ObtainedAt,
		UpdatedAt:         stored.UpdatedAt,
		TokenExpireTime:   stored.TokenExpireTime,
	}
}

// HasOAuth reads the credential file and reports whether access_token and
// refresh_token are present. Returns (hasAccessToken, hasRefreshToken).
func (manager *CredentialManager) HasOAuth() (bool, bool) {
	if manager.cfg.AuthFile == "" {
		return false, false
	}
	data, err := os.ReadFile(manager.cfg.AuthFile)
	if err != nil {
		return false, false
	}
	var stored StoredCredentialFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return false, false
	}
	return stored.OAuth.AccessToken != "", stored.OAuth.RefreshToken != ""
}

func (manager *CredentialManager) loadSnapshot() (CredentialSnapshot, error) {
	if manager.cfg.AuthFile == "" {
		return CredentialSnapshot{}, fmt.Errorf("%w: missing auth_file", ErrCredentialsUnavailable)
	}

	data, err := os.ReadFile(manager.cfg.AuthFile)
	if err != nil {
		return CredentialSnapshot{}, fmt.Errorf("%w: read auth file: %v", ErrCredentialsUnavailable, err)
	}

	var stored StoredCredentialFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return CredentialSnapshot{}, fmt.Errorf("%w: parse auth file: %v", ErrCredentialsUnavailable, err)
	}
	if stored.Source == "" {
		stored.Source = "project_auth_file"
	}

	expireTime := parseExpireTime(stored.TokenExpireTime)

	// Diagnostic logging for credential fields
	log.Printf("[credential] loaded from %s: cosy_key=%d chars, encrypt_user_info=%d chars, user_id=%s, machine_id=%s, source=%s, token_expire=%d",
		manager.cfg.AuthFile,
		len(stored.Auth.CosyKey),
		len(stored.Auth.EncryptUserInfo),
		stored.Auth.UserID,
		stored.Auth.MachineID,
		stored.Source,
		expireTime)

	snapshot := CredentialSnapshot{
		CosyKey:         stored.Auth.CosyKey,
		EncryptUserInfo: stored.Auth.EncryptUserInfo,
		UserID:          stored.Auth.UserID,
		MachineID:       stored.Auth.MachineID,
		Source:          stored.Source,
		LoadedAt:        manager.now(),
		TokenExpireTime: expireTime,
	}
	if err := validateSnapshot(snapshot); err != nil {
		log.Printf("[credential] validation failed, credentials unavailable")
		return CredentialSnapshot{}, err
	}
	log.Printf("[credential] validation passed, credentials ready")
	return snapshot, nil
}

func parseExpireTime(s string) int64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func validateSnapshot(snapshot CredentialSnapshot) error {
	var missing []string
	if snapshot.CosyKey == "" {
		missing = append(missing, "cosy_key")
	}
	if snapshot.EncryptUserInfo == "" {
		missing = append(missing, "encrypt_user_info")
	}
	if snapshot.UserID == "" {
		missing = append(missing, "user_id")
	}
	if snapshot.MachineID == "" {
		missing = append(missing, "machine_id")
	}
	if len(missing) > 0 {
		log.Printf("[credential] validation failed: missing fields: %v", missing)
		return fmt.Errorf("%w: missing %s", ErrCredentialsUnavailable, missing[0])
	}
	return nil
}
