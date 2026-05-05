package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lingma2api/internal/auth"
)

// freePort returns an OS-assigned free TCP port. Acceptable race window for tests.
func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port
	listener.Close()
	return fmt.Sprintf("%d", port)
}

// newTestManager wires a BootstrapManager pointing at a temp authFile and
// a free 127.0.0.1 port for the 37510 callback listener.
func newTestManager(t *testing.T) (*BootstrapManager, string) {
	t.Helper()
	dir := t.TempDir()
	authFile := filepath.Join(dir, "credentials.json")
	port := freePort(t)
	listenAddr := "127.0.0.1:" + port
	mgr := NewBootstrapManager(authFile, "", listenAddr, "2.11.2")
	return mgr, listenAddr
}

// patchUserLoginURL temporarily replaces auth.userLoginURL with target.
// Returns a restore function the caller defers.
func patchUserLoginURL(t *testing.T, target string) func() {
	t.Helper()
	old := auth.SetUserLoginURLForTest(target)
	return func() { auth.SetUserLoginURLForTest(old) }
}

// stubLoginServer returns an httptest.Server mimicking the remote
// /algo/api/v3/user/login endpoint. succeed=true returns cosy_key+EUI;
// succeed=false returns 403 (mimicking WAF).
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

// postUserInfo simulates the auto-inject script calling POST /submit-userinfo.
func postUserInfo(t *testing.T, listenAddr, origin string, body any) *http.Response {
	t.Helper()

	// Wait until the callback listener is actually bound. The session may
	// flip to awaiting_callback slightly before net.Listen returns.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", listenAddr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, "http://"+listenAddr+"/submit-userinfo", strings.NewReader(string(raw)))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post user-info: %v", err)
	}
	return resp
}

// waitForStatus polls m.GetStatus(id) until status matches one of want or timeout.
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

// validUserInfoBody returns a JSON body matching what the auto-inject script
// POSTs after a successful Aliyun login.
func validUserInfoBody() map[string]any {
	return map[string]any{
		"userInfo": `{"aid":"acct-1","uid":"user-123","name":"alice","securityOauthToken":"pt-test","refreshToken":"rt-test","expireTime":1782107060847}`,
		"loginUrl": "https://account.alibabacloud.com/logout/logout.htm?oauth_callback=https%3A%2F%2Flingma.alibabacloud.com%2Flingma%2Flogin%3Fmachine_id%3DM-from-login-url%26port%3D37510",
	}
}

func TestStartRemoteCallback_HappyPath(t *testing.T) {
	mgr, listenAddr := newTestManager(t)

	stub := stubLoginServer(t, true)
	defer stub.Close()
	defer patchUserLoginURL(t, stub.URL)()

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	if sess.Method != "remote_callback" {
		t.Errorf("method: got %q, want remote_callback", sess.Method)
	}
	if sess.AuthURL == "" {
		t.Error("auth_url empty")
	}

	waitForStatus(t, mgr, sess.ID, 2*time.Second, "awaiting_callback")

	resp := postUserInfo(t, listenAddr, "http://"+listenAddr, validUserInfoBody())
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit-userinfo status: %d", resp.StatusCode)
	}

	final := waitForStatus(t, mgr, sess.ID, 5*time.Second, "completed")
	if final.Error != "" {
		t.Errorf("expected no error, got %q", final.Error)
	}

	data, err := os.ReadFile(mgr.authFile)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if !strings.Contains(string(data), "cosy-test-key") {
		t.Errorf("credentials missing cosy_key; raw: %s", string(data))
	}
}

func TestStartRemoteCallback_BadOrigin(t *testing.T) {
	mgr, listenAddr := newTestManager(t)
	stub := stubLoginServer(t, true)
	defer stub.Close()
	defer patchUserLoginURL(t, stub.URL)()

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	waitForStatus(t, mgr, sess.ID, 2*time.Second, "awaiting_callback")

	resp := postUserInfo(t, listenAddr, "https://evil.com", validUserInfoBody())
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for evil origin, got %d", resp.StatusCode)
	}

	current := mgr.GetStatus(sess.ID)
	if current == nil || current.Status != "awaiting_callback" {
		t.Errorf("status changed unexpectedly: %+v", current)
	}

	_ = mgr.Cancel(sess.ID)
}

func TestStartRemoteCallback_Timeout(t *testing.T) {
	old := remoteCallbackTimeoutForTest
	remoteCallbackTimeoutForTest = 200 * time.Millisecond
	defer func() { remoteCallbackTimeoutForTest = old }()

	mgr, _ := newTestManager(t)
	stub := stubLoginServer(t, true)
	defer stub.Close()
	defer patchUserLoginURL(t, stub.URL)()

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}

	final := waitForStatus(t, mgr, sess.ID, 2*time.Second, "error")
	if !strings.Contains(final.Error, "timeout") {
		t.Errorf("expected timeout error, got %q", final.Error)
	}
}

func TestStartRemoteCallback_BadUserInfo(t *testing.T) {
	mgr, listenAddr := newTestManager(t)
	stub := stubLoginServer(t, true)
	defer stub.Close()
	defer patchUserLoginURL(t, stub.URL)()

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	waitForStatus(t, mgr, sess.ID, 2*time.Second, "awaiting_callback")

	resp := postUserInfo(t, listenAddr, "http://"+listenAddr, map[string]any{
		"userInfo": `{}`,
		"loginUrl": "https://lingma.alibabacloud.com/lingma/login?machine_id=M-test",
	})
	resp.Body.Close()

	final := waitForStatus(t, mgr, sess.ID, 3*time.Second, "error")
	if !strings.Contains(final.Error, "missing securityOauthToken") &&
		!strings.Contains(final.Error, "extract from callback page") {
		t.Errorf("expected parse error, got %q", final.Error)
	}
}

func TestStartRemoteCallback_DeriveFailed(t *testing.T) {
	mgr, listenAddr := newTestManager(t)
	stub := stubLoginServer(t, false) // 403 WAF
	defer stub.Close()
	defer patchUserLoginURL(t, stub.URL)()

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	waitForStatus(t, mgr, sess.ID, 2*time.Second, "awaiting_callback")

	resp := postUserInfo(t, listenAddr, "http://"+listenAddr, validUserInfoBody())
	resp.Body.Close()

	final := waitForStatus(t, mgr, sess.ID, 5*time.Second, "error")
	if !strings.Contains(final.Error, "derive credentials") {
		t.Errorf("expected derive error, got %q", final.Error)
	}
}

func TestStartRemoteCallback_Cancel(t *testing.T) {
	mgr, _ := newTestManager(t)

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	waitForStatus(t, mgr, sess.ID, 2*time.Second, "awaiting_callback")

	if err := mgr.Cancel(sess.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	final := waitForStatus(t, mgr, sess.ID, 2*time.Second, "cancelled")
	if final.Status != "cancelled" {
		t.Errorf("expected cancelled, got %s", final.Status)
	}
}

func TestHandleAdminAccountBootstrap_AutoFallsBackToRemoteCallback(t *testing.T) {
	mgr, _ := newTestManager(t)
	server := &Server{deps: Dependencies{Bootstrap: mgr, AdminToken: ""}}

	req := httptest.NewRequest(http.MethodPost, "/admin/account/bootstrap", strings.NewReader(`{"method":"auto"}`))
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
	_ = mgr.Cancel(sess.ID)
}

func TestHandleAdminAccountBootstrap_InvalidMethod(t *testing.T) {
	mgr, _ := newTestManager(t)
	server := &Server{deps: Dependencies{Bootstrap: mgr}}

	req := httptest.NewRequest(http.MethodPost, "/admin/account/bootstrap",
		strings.NewReader(`{"method":"banana"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleAdminAccountBootstrap(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid method") {
		t.Errorf("expected invalid method error, got %s", w.Body.String())
	}
}

func TestHandleAdminAccountBootstrap_DeleteCancelsSession(t *testing.T) {
	mgr, _ := newTestManager(t)
	server := &Server{deps: Dependencies{Bootstrap: mgr}}

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	waitForStatus(t, mgr, sess.ID, 2*time.Second, "awaiting_callback")

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

func TestHandleAdminAccountBootstrap_DeleteSessionNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	server := &Server{deps: Dependencies{Bootstrap: mgr}}

	req := httptest.NewRequest(http.MethodDelete, "/admin/account/bootstrap?id=ghost", nil)
	w := httptest.NewRecorder()
	server.handleAdminAccountBootstrap(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
