package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lingma2api/internal/auth"
)

func newTestManager(t *testing.T) (*BootstrapManager, string) {
	t.Helper()
	dir := t.TempDir()
	authFile := filepath.Join(dir, "credentials.json")
	callbackAddr := "127.0.0.1:37510"
	mgr := NewBootstrapManager(authFile, callbackAddr, "2.11.2")
	return mgr, callbackAddr
}

func patchUserLoginURL(t *testing.T, target string) func() {
	t.Helper()
	old := auth.SetUserLoginURLForTest(target)
	return func() { auth.SetUserLoginURLForTest(old) }
}

func stubLoginServer(t *testing.T, succeed bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !succeed {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errorCode":"WAF","errorMessage":"blocked"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"key": "cosy-test-key",
			"encrypt_user_info": "eui-test",
			"uid": "user-123"
		}`))
	}))
}

func waitForStatus(t *testing.T, m *BootstrapManager, id string, timeout time.Duration, want ...string) *BootstrapSession {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sess := m.GetStatus(id)
		if sess != nil {
			for _, w := range want {
				if sess.Status == w {
					return sess
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach %v within %v (last: %+v)", id, want, timeout, m.GetStatus(id))
	return nil
}

func encodedCallbackURL(t *testing.T) string {
	t.Helper()
	return "http://127.0.0.1:37510/auth/callback?uid=user-123&aid=acct-1&name=alice&access_token=pt-test&refresh_token=rt-test&expire_time=1782107060847"
}

func TestStartRemoteCallback_ReturnsAuthURLWithoutListener(t *testing.T) {
	mgr, _ := newTestManager(t)

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	if sess.Method != "remote_callback" {
		t.Errorf("method: got %q, want remote_callback", sess.Method)
	}
	if sess.Status != "awaiting_callback_url" {
		t.Errorf("status: got %q, want awaiting_callback_url", sess.Status)
	}
	if sess.AuthURL == "" {
		t.Fatal("auth_url empty")
	}
}

func TestSubmitCallbackURL_HappyPath(t *testing.T) {
	mgr, _ := newTestManager(t)

	stub := stubLoginServer(t, true)
	defer stub.Close()
	defer patchUserLoginURL(t, stub.URL)()

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}

	updated, err := mgr.SubmitCallbackURL(sess.ID, encodedCallbackURL(t))
	if err != nil {
		t.Fatalf("SubmitCallbackURL: %v", err)
	}
	if updated.Status != "completed" {
		t.Fatalf("status: got %q, want completed", updated.Status)
	}

	data, err := os.ReadFile(mgr.authFile)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if !strings.Contains(string(data), "user-123") {
		t.Errorf("credentials missing user id; raw: %s", string(data))
	}
}

func TestSubmitCallbackURL_RejectsWrongHost(t *testing.T) {
	mgr, _ := newTestManager(t)
	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}

	_, err = mgr.SubmitCallbackURL(sess.ID, "http://example.com/auth/callback?auth=a&token=b")
	if err == nil || !strings.Contains(err.Error(), "host must be") {
		t.Fatalf("expected host error, got %v", err)
	}
}

func TestSubmitCallbackURL_BadPayload(t *testing.T) {
	mgr, _ := newTestManager(t)
	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}

	_, err = mgr.SubmitCallbackURL(sess.ID, "http://127.0.0.1:37510/auth/callback?auth=bad&token=bad")
	if err == nil || (!strings.Contains(err.Error(), "parse callback url") && !strings.Contains(err.Error(), "v2 auth decode failed")) {
		t.Fatalf("expected parse error, got %v", err)
	}

	final := mgr.GetStatus(sess.ID)
	if final == nil || final.Status != "error" {
		t.Fatalf("expected error status, got %+v", final)
	}
}

func TestSubmitCallbackURL_PrefersLocalCosyGeneration(t *testing.T) {
	mgr, _ := newTestManager(t)
	stub := stubLoginServer(t, false)
	defer stub.Close()
	defer patchUserLoginURL(t, stub.URL)()

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}

	updated, err := mgr.SubmitCallbackURL(sess.ID, encodedCallbackURL(t))
	if err != nil {
		t.Fatalf("expected local cosy generation to succeed, got %v", err)
	}
	if updated.Status != "completed" {
		t.Fatalf("expected completed, got %s", updated.Status)
	}
}

func TestStartRemoteCallback_Timeout(t *testing.T) {
	old := remoteCallbackTimeoutForTest
	remoteCallbackTimeoutForTest = 150 * time.Millisecond
	defer func() { remoteCallbackTimeoutForTest = old }()

	mgr, _ := newTestManager(t)
	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}

	final := waitForStatus(t, mgr, sess.ID, 2*time.Second, "error")
	if !strings.Contains(final.Error, "timeout") {
		t.Errorf("expected timeout error, got %q", final.Error)
	}
}

func TestHandleAdminAccountBootstrap_DefaultsToRemoteCallback(t *testing.T) {
	mgr, _ := newTestManager(t)
	server := &Server{deps: Dependencies{Bootstrap: mgr, AdminToken: ""}}

	req := httptest.NewRequest(http.MethodPost, "/admin/account/bootstrap", strings.NewReader(`{"method":"remote_callback"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleAdminAccountBootstrap(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var sess BootstrapSession
	if err := json.Unmarshal(w.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sess.Method != "remote_callback" {
		t.Errorf("method: got %q, want remote_callback", sess.Method)
	}
}

func TestHandleAdminAccountBootstrapSubmit(t *testing.T) {
	mgr, _ := newTestManager(t)
	stub := stubLoginServer(t, true)
	defer stub.Close()
	defer patchUserLoginURL(t, stub.URL)()
	server := &Server{deps: Dependencies{Bootstrap: mgr}}

	started, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}

	body := fmt.Sprintf(`{"id":"%s","callback_url":%q}`, started.ID, encodedCallbackURL(t))
	req := httptest.NewRequest(http.MethodPost, "/admin/account/bootstrap/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleAdminAccountBootstrapSubmit(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	var sess BootstrapSession
	if err := json.Unmarshal(w.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sess.Status != "completed" {
		t.Fatalf("expected completed, got %s", sess.Status)
	}
}

func TestHandleAdminAccountBootstrap_DeleteCancelsSession(t *testing.T) {
	mgr, _ := newTestManager(t)
	server := &Server{deps: Dependencies{Bootstrap: mgr}}

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	waitForStatus(t, mgr, sess.ID, 2*time.Second, "awaiting_callback_url")

	req := httptest.NewRequest(http.MethodDelete, "/admin/account/bootstrap?id="+sess.ID, nil)
	w := httptest.NewRecorder()
	server.handleAdminAccountBootstrap(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}

	final := waitForStatus(t, mgr, sess.ID, 2*time.Second, "cancelled")
	if final.Status != "cancelled" {
		t.Errorf("expected cancelled, got %s", final.Status)
	}
}
