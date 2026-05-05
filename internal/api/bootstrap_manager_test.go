package api

import (
	"strings"
	"testing"
	"time"
)

func TestBootstrapManager_StartAuto_PrefersRemoteCallback(t *testing.T) {
	mgr, _ := newTestManager(t)

	sess, err := mgr.Start("auto")
	if err != nil {
		t.Fatalf("Start auto: %v", err)
	}
	if sess.Method != "remote_callback" {
		t.Errorf("method: got %q, want remote_callback", sess.Method)
	}
	_ = mgr.Cancel(sess.ID)
}

func TestBootstrapManager_ConcurrentSessionRejected(t *testing.T) {
	mgr, _ := newTestManager(t)

	sess1, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	waitForStatus(t, mgr, sess1.ID, 2*time.Second, "awaiting_callback")

	_, err = mgr.StartRemoteCallback()
	if err == nil {
		t.Fatal("expected error for concurrent start, got nil")
	}
	if !strings.Contains(err.Error(), "in progress") {
		t.Errorf("expected 'in progress' error, got %q", err)
	}

	_ = mgr.Cancel(sess1.ID)
}

func TestBootstrapManager_Cancel_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Cancel("nonexistent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestBootstrapManager_Cancel_AlreadyCompleted(t *testing.T) {
	mgr, _ := newTestManager(t)

	mgr.mu.Lock()
	mgr.sessions["fake"] = &BootstrapSession{
		ID:        "fake",
		Status:    "completed",
		Method:    "remote_callback",
		StartedAt: time.Now(),
	}
	mgr.mu.Unlock()

	err := mgr.Cancel("fake")
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("expected 'already' error, got %v", err)
	}
}

func TestBootstrapManager_Start_InvalidMethod(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.Start("banana")
	if err == nil || !strings.Contains(err.Error(), "invalid method") {
		t.Errorf("expected 'invalid method', got %v", err)
	}
}
