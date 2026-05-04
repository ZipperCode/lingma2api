# Remote Callback Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 lingma2api 用户管理中新增「脱离本地二进制」的远程登录方式 —— `BootstrapManager.StartRemoteCallback()` + 自动注入脚本 + 智能 fallback，让用户无 `client_id`、无本地 Lingma 二进制时也能完成 OAuth 登录并写入 `credentials.json`。

**Architecture:** 复用 `auth.BuildLingmaLoginEntryURL` / `WrapLingmaLoginURLForBrowser` / `ExtractFromCallbackPage` / `DeriveCredentialsRemotely` 已有能力；扩展 `WaitForCallback` 支持 `WaitForCallbackWithOptions`（Origin 白名单 + 自动注入 HTML 分支）；`BootstrapManager` 新增 `Start(method)` 分发入口、`Cancel(id)`、`ExpiresAt`；前端 Account 页加倒计时 + 取消按钮。

**Tech Stack:** Go 1.24（标准库 + `gorilla/websocket` 既有）/ React 18 + TypeScript + Vite + lucide-react

**Spec:** `docs/superpowers/specs/2026-05-05-remote-callback-bootstrap-design.md`

---

## 文件结构

| 文件 | 动作 | 职责 |
|---|---|---|
| `internal/auth/callback_inject_html.go` | 创建 | `CallbackAutoInjectHTML` 常量（避免与已有 `callback_html.go` 命名冲突） |
| `internal/auth/callback_inject_html_test.go` | 创建 | 断言注入脚本含 `/submit-userinfo` 等关键片段 |
| `internal/auth/bootstrap.go` | 修改 | 新增 `WaitForCallbackWithOptions(ctx, listen, path, opts)` + Origin 白名单 + AutoInject 分支；保留旧 `WaitForCallback` |
| `internal/auth/bootstrap_test.go` | 修改 | 新增 `TestWaitForCallback_AutoInjectHTML` + `TestWaitForCallback_OriginWhitelist` |
| `internal/api/bootstrap_remote_callback.go` | 创建 | `StartRemoteCallback()` + `runRemoteCallbackFlow()` |
| `internal/api/bootstrap_remote_callback_test.go` | 创建 | 6 个端到端测试（happy / origin / timeout / bad-userinfo / derive-fail / cancel） |
| `internal/api/bootstrap_manager.go` | 修改 | 扩 session 字段（`ExpiresAt`/`cancel`）；新增 `Start(method)` / `Cancel(id)` |
| `internal/api/bootstrap_manager_test.go` | 创建 | `auto` fallback / 并发 / cancel 测试 |
| `internal/api/bootstrap_handler.go` | 修改 | body `{"method":...}` 解析 + `DELETE /admin/account/bootstrap` |
| `frontend/src/types/index.ts` | 修改 | `BootstrapMethod` / `BootstrapStatus` 枚举 + `expires_at` 字段 |
| `frontend/src/api/client.ts` | 修改 | `startBootstrap(method?)` + `cancelBootstrap(id)` |
| `frontend/src/pages/Account.tsx` | 修改 | 倒计时 / 取消按钮 / 状态文案 |
| `frontend/tests/account-bootstrap.spec.ts` | 创建 | Playwright E2E 3 用例（仅在已配 Playwright 时跑） |

---

## Task 1: 新增 CallbackAutoInjectHTML 常量

**Files:**
- Create: `internal/auth/callback_inject_html.go`
- Create: `internal/auth/callback_inject_html_test.go`

- [ ] **Step 1: 写失败测试**

创建 `internal/auth/callback_inject_html_test.go`：

```go
package auth

import (
	"strings"
	"testing"
)

func TestCallbackAutoInjectHTML_ContainsCriticalParts(t *testing.T) {
	html := CallbackAutoInjectHTML
	mustContain := []string{
		"<!DOCTYPE html>",
		"window.user_info",
		"window.login_url",
		"http://127.0.0.1:37510/submit-userinfo",
		"application/json",
		"提交成功",
	}
	for _, s := range mustContain {
		if !strings.Contains(html, s) {
			t.Errorf("CallbackAutoInjectHTML missing %q", s)
		}
	}
}
```

- [ ] **Step 2: 验证测试失败**

Run: `cd /home/zipper/Projects/lingma2api && go test ./internal/auth/ -run TestCallbackAutoInjectHTML_ContainsCriticalParts -v`
Expected: FAIL，因为 `CallbackAutoInjectHTML` 未定义。

- [ ] **Step 3: 实现常量**

创建 `internal/auth/callback_inject_html.go`：

```go
package auth

// CallbackAutoInjectHTML is the HTML returned by GET /auth/callback when the
// 37510 callback server runs in auto-inject mode. The embedded <script> reads
// window.user_info / window.login_url (set by Lingma's callback page in the
// browser context) and POSTs them to /submit-userinfo on the same listener.
const CallbackAutoInjectHTML = `<!DOCTYPE html>
<html><head><meta charset="UTF-8"><title>Lingma Auth</title></head>
<body>
<h1>正在拓印凭据...</h1>
<p id="status">请稍候。本页面将在凭据提交后自动关闭。</p>
<script>
(function(){
  var statusEl = document.getElementById('status');
  try {
    if (typeof window.user_info === 'undefined') {
      statusEl.textContent = '错误：window.user_info 未定义。请检查是否登录完成。';
      return;
    }
    fetch('http://127.0.0.1:37510/submit-userinfo', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        userInfo: typeof window.user_info === 'string'
                  ? window.user_info
                  : JSON.stringify(window.user_info),
        loginUrl: window.login_url || '',
      })
    }).then(function(r){ return r.text(); })
      .then(function(t){ statusEl.textContent = '提交成功，可以关闭窗口。'; })
      .catch(function(e){ statusEl.textContent = '提交失败: ' + e; });
  } catch (e) {
    statusEl.textContent = '脚本错误: ' + e;
  }
})();
</script>
</body></html>`
```

- [ ] **Step 4: 验证测试通过**

Run: `go test ./internal/auth/ -run TestCallbackAutoInjectHTML_ContainsCriticalParts -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd /home/zipper/Projects/lingma2api
git add internal/auth/callback_inject_html.go internal/auth/callback_inject_html_test.go
git commit -m "feat(auth): add CallbackAutoInjectHTML constant for remote callback flow"
```

---

## Task 2: 扩展 WaitForCallback 支持 Origin 白名单与自动注入

**Files:**
- Modify: `internal/auth/bootstrap.go`
- Modify: `internal/auth/bootstrap_test.go`

- [ ] **Step 1: 写失败测试 — Origin 白名单**

在 `internal/auth/bootstrap_test.go` 文件末尾追加：

```go
func TestWaitForCallback_OriginWhitelistRejectsForeign(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resultCh := make(chan CallbackCapture, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := WaitForCallbackWithOptions(ctx, addr, "/callback", WaitForCallbackOptions{
			AllowedOrigins: []string{"http://" + addr, ""},
			AutoInjectHTML: false,
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- c
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Send foreign Origin request
	req, _ := http.NewRequest("POST", "http://"+addr+"/submit-userinfo", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign origin, got %d", resp.StatusCode)
	}

	// Send legitimate POST (empty origin) → should resolve
	req2, _ := http.NewRequest("POST", "http://"+addr+"/submit-userinfo", strings.NewReader(`{"ok":true}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	if resp2 != nil {
		resp2.Body.Close()
	}

	select {
	case captured := <-resultCh:
		if string(captured.Body) == "" {
			t.Fatal("expected body bytes")
		}
	case err := <-errCh:
		t.Fatalf("WaitForCallbackWithOptions error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for capture")
	}
}

func TestWaitForCallback_AutoInjectHTMLIncludesScript(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = WaitForCallbackWithOptions(ctx, addr, "/auth/callback", WaitForCallbackOptions{
			AutoInjectHTML: true,
		})
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/auth/callback")
	if err != nil {
		t.Fatalf("get callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "/submit-userinfo") {
		t.Fatalf("expected injection script in body, got: %s", string(body))
	}
	cancel() // Stop server
}
```

确保文件顶部 import 包含：`context`, `io`, `net`, `net/http`, `strings`, `time`（如缺则补）。

- [ ] **Step 2: 验证测试失败**

Run: `go test ./internal/auth/ -run TestWaitForCallback_OriginWhitelistRejectsForeign -v`
Expected: FAIL，`WaitForCallbackWithOptions` 未定义。

- [ ] **Step 3: 修改 `internal/auth/bootstrap.go` 添加 options 类型与新函数**

在 `internal/auth/bootstrap.go` 的 `func WaitForCallback(...)` 之前插入：

```go
// WaitForCallbackOptions controls advanced behavior of WaitForCallbackWithOptions.
type WaitForCallbackOptions struct {
	// AllowedOrigins is the whitelist for /submit-userinfo Origin header.
	// Empty string in the slice = no Origin header (same-origin / file:// / direct redirect).
	// nil = allow all (legacy behavior).
	AllowedOrigins []string

	// AutoInjectHTML controls whether GET <callbackPath> returns the auto-injection HTML
	// (CallbackAutoInjectHTML) instead of the legacy short HTML.
	AutoInjectHTML bool
}

// WaitForCallbackWithOptions is the option-aware version of WaitForCallback.
// It listens on listenAddr, serves <callbackPath> + /submit-userinfo + /profile,
// and returns the first CallbackCapture (GET query/path or POST body).
func WaitForCallbackWithOptions(ctx context.Context, listenAddr, callbackPath string, opts WaitForCallbackOptions) (CallbackCapture, error) {
	if listenAddr == "" {
		return CallbackCapture{}, errors.New("missing listen address")
	}
	if callbackPath == "" {
		callbackPath = "/callback"
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return CallbackCapture{}, err
	}
	defer listener.Close()

	resultCh := make(chan CallbackCapture, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	getHandler := func(writer http.ResponseWriter, request *http.Request) {
		captured := CaptureFromRequest(request)
		captured.Referer = request.Header.Get("Referer")
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		if opts.AutoInjectHTML {
			_, _ = writer.Write([]byte(CallbackAutoInjectHTML))
		} else {
			_, _ = writer.Write([]byte(`<!DOCTYPE html>
<html><head><meta charset="UTF-8"><title>Lingma Auth</title></head>
<body>
<h1>Authorization received</h1>
<p>You may close this window.</p>
<p>Run this in console to copy tokens:</p>
<pre style="background:#f0f0f0;padding:8px;border-radius:4px;overflow:auto;max-height:200px">
copy(window.user_info)
</pre>
</body></html>`))
		}
		// Only register a capture for the GET path if it carries query parameters
		// (otherwise the auto-inject script is expected to deliver via POST).
		if !opts.AutoInjectHTML || len(captured.Query) > 0 {
			select {
			case resultCh <- captured:
			default:
			}
		}
	}
	mux.HandleFunc(callbackPath, getHandler)
	if callbackPath != "/profile" {
		mux.HandleFunc("/profile", getHandler)
	}

	mux.HandleFunc("/submit-userinfo", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = writer.Write([]byte(`<h1>Method Not Allowed</h1><p>Use POST with JSON body: {"userInfo":..., "loginUrl":...}</p>`))
			return
		}
		if !originAllowed(request.Header.Get("Origin"), opts.AllowedOrigins) {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = writer.Write([]byte("forbidden origin"))
			return
		}
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			errCh <- fmt.Errorf("read submit-userinfo body: %w", readErr)
			return
		}
		defer request.Body.Close()

		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`<h1>Token data received</h1><p>You may close this window.</p>`))

		select {
		case resultCh <- CallbackCapture{
			Path:       "/submit-userinfo",
			ReceivedAt: time.Now(),
			Body:       body,
		}:
		default:
		}
	})

	server := &http.Server{Handler: mux}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	select {
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return CallbackCapture{}, ctx.Err()
	case err := <-errCh:
		_ = server.Shutdown(context.Background())
		return CallbackCapture{}, err
	case result := <-resultCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return result, nil
	}
}

// originAllowed returns true if origin is in the whitelist or whitelist is nil.
// Empty string in whitelist matches the empty Origin header.
func originAllowed(origin string, allowed []string) bool {
	if allowed == nil {
		return true
	}
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}
```

然后将旧 `WaitForCallback` 改为转调新函数：

```go
func WaitForCallback(ctx context.Context, listenAddr, callbackPath string) (CallbackCapture, error) {
	return WaitForCallbackWithOptions(ctx, listenAddr, callbackPath, WaitForCallbackOptions{})
}
```

确保 `internal/auth/bootstrap.go` 的 import 块包含：`context`, `errors`, `fmt`, `io`, `net`, `net/http`, `time`（按既有内容补齐）。

- [ ] **Step 4: 验证两个新测试通过**

Run: `go test ./internal/auth/ -run TestWaitForCallback -v`
Expected: PASS（含 `TestWaitForCallback_OriginWhitelistRejectsForeign` 与 `TestWaitForCallback_AutoInjectHTMLIncludesScript`）。

- [ ] **Step 5: 验证既有测试未被破坏**

Run: `go test ./internal/auth/ -v`
Expected: 全部 PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/auth/bootstrap.go internal/auth/bootstrap_test.go
git commit -m "feat(auth): add WaitForCallbackWithOptions with origin whitelist + auto-inject"
```

---

## Task 3: 创建 StartRemoteCallback 主体框架

**Files:**
- Create: `internal/api/bootstrap_remote_callback.go`

- [ ] **Step 1: 创建文件骨架**

创建 `internal/api/bootstrap_remote_callback.go`：

```go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"lingma2api/internal/auth"
	"lingma2api/internal/proxy"
)

// remoteCallbackTimeout bounds how long a Remote Callback bootstrap session
// waits for the user to complete the browser-side flow.
const remoteCallbackTimeout = 5 * time.Minute

// StartRemoteCallback starts a "no client_id, no local Lingma" bootstrap session.
// It builds a Lingma login URL, opens a 127.0.0.1:37510 callback server with
// auto-inject HTML, and once user_info arrives derives credentials remotely.
//
// Preconditions: none (no client_id, no local Lingma binary required).
func (m *BootstrapManager) StartRemoteCallback() (*BootstrapSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing := m.findActiveLocked(); existing != nil {
		return nil, fmt.Errorf("another bootstrap in progress (id=%s)", existing.ID)
	}

	port, err := portFromAddr(m.listenAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid listen addr: %w", err)
	}

	machineID := auth.NewMachineID()
	loginURL, _, _, err := auth.BuildLingmaLoginEntryURL(auth.LingmaLoginEntryConfig{
		MachineID: machineID,
		Port:      port,
	})
	if err != nil {
		return nil, fmt.Errorf("build lingma login url: %w", err)
	}
	browserURL, err := auth.WrapLingmaLoginURLForBrowser(loginURL)
	if err != nil {
		return nil, fmt.Errorf("wrap login url: %w", err)
	}

	now := time.Now()
	id := newSessionID()
	ctx, cancel := context.WithTimeout(context.Background(), remoteCallbackTimeout)

	sess := &BootstrapSession{
		ID:        id,
		Status:    "running",
		Method:    "remote_callback",
		AuthURL:   browserURL,
		StartedAt: now,
		ExpiresAt: now.Add(remoteCallbackTimeout),
		cancel:    cancel,
	}
	m.sessions[id] = sess

	go m.runRemoteCallbackFlow(ctx, id, machineID)

	return sess, nil
}

// runRemoteCallbackFlow executes the full callback → derive → save chain.
// It transitions session.Status through awaiting_callback → deriving → completed/error.
func (m *BootstrapManager) runRemoteCallbackFlow(ctx context.Context, id, machineID string) {
	defer func() {
		m.mu.Lock()
		if s, ok := m.sessions[id]; ok && s.cancel != nil {
			s.cancel = nil
		}
		m.mu.Unlock()
	}()

	m.updateSessionStatus(id, "awaiting_callback", "")

	allowedOrigins := []string{
		"http://" + m.listenAddr,
		"http://127.0.0.1:" + portFromAddrOrEmpty(m.listenAddr),
		"",
		"null",
	}

	capture, err := auth.WaitForCallbackWithOptions(ctx, m.listenAddr, "/auth/callback", auth.WaitForCallbackOptions{
		AllowedOrigins: allowedOrigins,
		AutoInjectHTML: true,
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr == context.Canceled {
			m.updateSessionStatus(id, "cancelled", "")
			return
		}
		if ctxErr := ctx.Err(); ctxErr == context.DeadlineExceeded {
			m.updateSessionStatus(id, "error", "timeout: user did not complete login within 5m")
			return
		}
		m.updateSessionStatus(id, "error", fmt.Sprintf("wait for callback: %v", err))
		return
	}

	if len(capture.Body) == 0 {
		m.updateSessionStatus(id, "error", fmt.Sprintf("callback did not contain user_info body (path=%s)", capture.Path))
		return
	}

	var submission struct {
		UserInfo string `json:"userInfo"`
		LoginURL string `json:"loginUrl"`
	}
	if err := json.Unmarshal(capture.Body, &submission); err != nil {
		m.updateSessionStatus(id, "error", fmt.Sprintf("parse user_info failed: %v", err))
		return
	}
	if submission.UserInfo == "" {
		m.updateSessionStatus(id, "error", "submit-userinfo body missing userInfo")
		return
	}

	extracted, err := auth.ExtractFromCallbackPage(submission.UserInfo, submission.LoginURL)
	if err != nil {
		m.updateSessionStatus(id, "error", fmt.Sprintf("extract from callback page: %v", err))
		return
	}

	if extracted.MachineID == "" {
		extracted.MachineID = machineID
	}

	m.updateSessionStatus(id, "deriving", "")

	stored, err := auth.DeriveCredentialsRemotely(auth.RemoteLoginConfig{
		AccessToken:   extracted.AccessToken,
		RefreshToken:  extracted.RefreshToken,
		UserID:        extracted.UserID,
		Username:      extracted.Username,
		MachineID:     extracted.MachineID,
		TokenExpireMs: extracted.TokenExpireMs,
	})
	if err != nil {
		m.updateSessionStatus(id, "error", fmt.Sprintf("derive credentials: %v", err))
		return
	}

	if stored.Auth.UserID == "" {
		stored.Auth.UserID = extracted.UserID
	}
	if stored.Auth.MachineID == "" {
		stored.Auth.MachineID = extracted.MachineID
	}

	if err := auth.SaveCredentialFile(m.authFile, stored); err != nil {
		m.updateSessionStatus(id, "error", fmt.Sprintf("save credentials: %v", err))
		return
	}

	m.updateSessionStatus(id, "completed", "")
	_ = stored // explicit reference so future logging additions can pick up fields
	_ = proxy.StoredCredentialFile{}
}

func portFromAddr(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "", fmt.Errorf("invalid addr %q", addr)
	}
	return port, nil
}

func portFromAddrOrEmpty(addr string) string {
	if p, err := portFromAddr(addr); err == nil {
		return p
	}
	return ""
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./...`
Expected: 报错——`m.findActiveLocked` / `updateSessionStatus` / `BootstrapSession.ExpiresAt` / `BootstrapSession.cancel` 未定义。这是预期的，将在 Task 5 补齐。

- [ ] **Step 3: 暂时不提交**

继续 Task 4 编写测试，Task 5 补齐缺失方法后再提交。

---

## Task 4: 编写 StartRemoteCallback 测试套件

**Files:**
- Create: `internal/api/bootstrap_remote_callback_test.go`

- [ ] **Step 1: 创建测试文件 — 基础设施 + happy path**

创建 `internal/api/bootstrap_remote_callback_test.go`：

```go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// freePort returns an OS-assigned free TCP port, then releases it for the
// caller to re-bind. This is racy but acceptable in tests.
func freePort(t *testing.T) string {
	t.Helper()
	addr := "127.0.0.1:0"
	srv, err := newListener(addr)
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := srv.Addr().(*netAddr).Port()
	srv.Close()
	return fmt.Sprintf("%d", port)
}

// We use the standard library directly to avoid importing internal helpers.
type netAddr = interface {
	Port() int
}

// Use net package via small wrappers
// (kept simple to avoid extra imports in this helper file).

func newListener(addr string) (interface {
	Addr() interface {
		Port() int
	}
	Close() error
}, error) {
	return openTCP(addr)
}
```

这个 helper 写起来太啰嗦了，浮浮酱用更直接的方式 —— 直接用 `net.Listen`。把上面 helper 删了，重新写：

```go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// freePort returns an OS-assigned free TCP port (releases the listener
// before returning). Acceptable race window for tests.
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

// patchUserLoginURL temporarily replaces the package-level userLoginURL with
// the test server URL so DeriveCredentialsRemotely points at our mock.
// It returns a restore function. NOTE: this requires userLoginURL to be a
// package var in internal/auth/remote_login.go (it is currently a const).
// Therefore, we instead spin up an HTTP-handler-based mock and rely on
// Step "swap userLoginURL to var" below.
//
// In this plan we will:
//   1. Change userLoginURL from `const` to `var` in remote_login.go (Task 5)
//   2. Use this helper to swap it in tests.
func patchUserLoginURL(t *testing.T, target string) func() {
	t.Helper()
	// import path is auth package; we reach in via test-only helper from auth.
	// Implemented after Task 5's userLoginURL becomes var.
	old := authUserLoginURL()
	setAuthUserLoginURL(target)
	return func() { setAuthUserLoginURL(old) }
}

// stubLoginServer returns an httptest.Server that mimics the remote
// /algo/api/v3/user/login endpoint. The returned closure can be set to
// either succeed (return cosy_key + encrypt_user_info) or fail (HTTP 403).
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
// Origin defaults to http://<listenAddr> when blank.
func postUserInfo(t *testing.T, listenAddr, origin string, body any) *http.Response {
	t.Helper()
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

// waitForStatus polls m.GetStatus(id) until status matches one of "want" or timeout.
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
	t.Fatalf("session %s did not reach %v within %v (last status: %+v)", id, want, timeout, m.GetStatus(id))
	return nil
}

// validUserInfoBody returns a JSON body matching what auto-inject script POSTs.
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
	restore := patchUserLoginURL(t, stub.URL)
	defer restore()

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

	// Wait for awaiting_callback (server is listening).
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

	// Verify credentials.json was written
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
	restore := patchUserLoginURL(t, stub.URL)
	defer restore()

	sess, err := mgr.StartRemoteCallback()
	if err != nil {
		t.Fatalf("StartRemoteCallback: %v", err)
	}
	waitForStatus(t, mgr, sess.ID, 2*time.Second, "awaiting_callback")

	resp := postUserInfo(t, listenAddr, "https://evil.com", validUserInfoBody())
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}

	current := mgr.GetStatus(sess.ID)
	if current.Status != "awaiting_callback" {
		t.Errorf("status changed unexpectedly: %s", current.Status)
	}

	// Cancel to clean up
	_ = mgr.Cancel(sess.ID)
}

func TestStartRemoteCallback_Timeout(t *testing.T) {
	// Override timeout via test hook
	old := remoteCallbackTimeout
	remoteCallbackTimeoutForTest = 200 * time.Millisecond
	defer func() { remoteCallbackTimeoutForTest = old }()

	mgr, _ := newTestManager(t)
	stub := stubLoginServer(t, true)
	defer stub.Close()
	restore := patchUserLoginURL(t, stub.URL)
	defer restore()

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
	restore := patchUserLoginURL(t, stub.URL)
	defer restore()

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
	restore := patchUserLoginURL(t, stub.URL)
	defer restore()

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

// Make compiler happy when not all helpers are used in every build mode.
var _ = sync.Mutex{}
var _ io.Reader = strings.NewReader("")
var _ context.Context = context.Background()
```

- [ ] **Step 2: 不运行测试**（Task 5 之前，编译会失败）

继续 Task 5 补齐 `Cancel`/`updateSessionStatus`/`findActiveLocked`/`ExpiresAt`/`cancel`/`remoteCallbackTimeoutForTest` + 在 auth 包暴露 `userLoginURL` test hook。

---

## Task 5: BootstrapManager 扩 ExpiresAt / Cancel / Start 分发 + auth 包暴露 test hook

**Files:**
- Modify: `internal/api/bootstrap_manager.go`
- Modify: `internal/auth/remote_login.go`
- Create: `internal/auth/remote_login_testhook.go`
- Create: `internal/api/auth_testhook.go`

- [ ] **Step 1: 修改 BootstrapSession 结构体添加 ExpiresAt + cancel**

打开 `internal/api/bootstrap_manager.go`，把 `BootstrapSession` 替换为：

```go
type BootstrapSession struct {
	ID        string             `json:"id"`
	Status    string             `json:"status"`
	Method    string             `json:"method"`
	AuthURL   string             `json:"auth_url,omitempty"`
	Error     string             `json:"error,omitempty"`
	StartedAt time.Time          `json:"started_at"`
	ExpiresAt time.Time          `json:"expires_at,omitempty"`
	cancel    context.CancelFunc `json:"-"`
}
```

确保 `internal/api/bootstrap_manager.go` 已 import `context`（既有）。

- [ ] **Step 2: 添加 findActiveLocked / updateSessionStatus / Cancel / Start**

在 `internal/api/bootstrap_manager.go` 文件末尾追加：

```go
// findActiveLocked returns the session currently in a non-terminal state, or nil.
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

// updateSessionStatus updates a session's status + error fields while holding m.mu.
func (m *BootstrapManager) updateSessionStatus(id, status, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.Status = status
		s.Error = errMsg
	}
}

// Start dispatches to the requested bootstrap method. Empty/auto runs the
// fallback chain: oauth (when client_id configured) → remote_callback → ws.
func (m *BootstrapManager) Start(method string) (*BootstrapSession, error) {
	switch method {
	case "", "auto":
		if m.clientID != "" {
			if s, err := m.StartOAuth(); err == nil {
				return s, nil
			}
		}
		if s, err := m.StartRemoteCallback(); err == nil {
			return s, nil
		}
		return m.StartWS()
	case "oauth":
		return m.StartOAuth()
	case "ws":
		return m.StartWS()
	case "remote_callback":
		return m.StartRemoteCallback()
	default:
		return nil, fmt.Errorf("invalid method: %s", method)
	}
}

// Cancel cancels an in-flight session by id. Only running/awaiting_callback/deriving
// sessions are cancellable.
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
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	// Set status synchronously so the caller can rely on the next GetStatus.
	m.updateSessionStatus(id, "cancelled", "")
	return nil
}
```

- [ ] **Step 3: 暴露测试用 timeout 钩子**

在 `internal/api/bootstrap_remote_callback.go` 文件末尾追加：

```go
// remoteCallbackTimeoutForTest can be set by tests to shorten the timeout.
// Production code uses remoteCallbackTimeout.
var remoteCallbackTimeoutForTest = remoteCallbackTimeout
```

然后在 `StartRemoteCallback` 函数体内，把 `context.WithTimeout(context.Background(), remoteCallbackTimeout)` 改为：

```go
ctx, cancel := context.WithTimeout(context.Background(), remoteCallbackTimeoutForTest)
```

并把构造 `sess.ExpiresAt` 的 `now.Add(remoteCallbackTimeout)` 改为 `now.Add(remoteCallbackTimeoutForTest)`。

- [ ] **Step 4: 把 auth.userLoginURL 改为 var 并加 test hook**

打开 `internal/auth/remote_login.go`，把：

```go
const (
	userLoginURL    = "https://lingma-api.tongyi.aliyun.com/algo/api/v3/user/login?Encode=1"
	userLoginAESKey = "QbgzpWzN7tfe43gf"
	...
)
```

改为：

```go
var (
	// userLoginURL is mutable for tests via SetUserLoginURLForTest.
	// Production value is the lingma-api.tongyi.aliyun.com endpoint.
	userLoginURL = "https://lingma-api.tongyi.aliyun.com/algo/api/v3/user/login?Encode=1"
)

const (
	userLoginAESKey = "QbgzpWzN7tfe43gf"
	...
)
```

把已有的其他 `const` 行（`OldSignatureKey` / `OldSignatureKeyAlt`）保留在第二个 const 块中。

- [ ] **Step 5: 创建 auth 包 test hook 文件**

创建 `internal/auth/remote_login_testhook.go`：

```go
package auth

// SetUserLoginURLForTest overrides userLoginURL for the duration of a test.
// Returns the previous value so the test can restore it.
//
// NOTE: not safe for parallel tests targeting different URLs.
func SetUserLoginURLForTest(target string) string {
	old := userLoginURL
	userLoginURL = target
	return old
}
```

- [ ] **Step 6: 创建 api 包 test hook 文件**

创建 `internal/api/auth_testhook.go`：

```go
package api

import "lingma2api/internal/auth"

// authUserLoginURL / setAuthUserLoginURL are thin wrappers exposed for tests
// in this package only.
func authUserLoginURL() string {
	return auth.SetUserLoginURLForTest(auth.UserLoginURLForTestRead())
}

func setAuthUserLoginURL(target string) {
	auth.SetUserLoginURLForTest(target)
}
```

哦不对，这样会有竞态。直接简化：

把 `internal/api/auth_testhook.go` 改为：

```go
package api

import "lingma2api/internal/auth"

func authUserLoginURL() string {
	// Read by swapping with itself, ensuring round-trip consistency.
	current := auth.SetUserLoginURLForTest("")
	auth.SetUserLoginURLForTest(current)
	return current
}

func setAuthUserLoginURL(target string) {
	auth.SetUserLoginURLForTest(target)
}
```

- [ ] **Step 7: 编译验证**

Run: `cd /home/zipper/Projects/lingma2api && go build ./...`
Expected: 编译通过。

- [ ] **Step 8: 运行所有 auth + api 单测**

Run: `go test ./internal/auth/... ./internal/api/... -v -count=1 -timeout 60s`
Expected: 既有测试 + Task 4 的 6 个新测试全部 PASS。

如果 `TestStartRemoteCallback_HappyPath` 失败提示 `bootstrap not configured` 之类，确认 `newTestManager` 中 `clientID=""`、`auth.SetUserLoginURLForTest` 在派生前已生效。

- [ ] **Step 9: 提交**

```bash
git add internal/api/bootstrap_remote_callback.go \
        internal/api/bootstrap_remote_callback_test.go \
        internal/api/bootstrap_manager.go \
        internal/api/auth_testhook.go \
        internal/auth/remote_login.go \
        internal/auth/remote_login_testhook.go
git commit -m "feat(api): StartRemoteCallback + Cancel + auto fallback dispatch

新增 BootstrapManager.StartRemoteCallback() 完成无 client_id /
无本地 Lingma 二进制的 bootstrap 流程。Start(method) 分发入口
支持 oauth/ws/remote_callback/auto 四种方法，auto 模式按
oauth (有 client_id) → remote_callback → ws 顺序回退。
Cancel(id) 主动终止 in-flight session。

附 6 个端到端单测覆盖 happy / origin / timeout / bad-userinfo /
derive-fail / cancel 场景。"
```

---

## Task 6: BootstrapHandler 接受 method 参数 + DELETE endpoint

**Files:**
- Modify: `internal/api/bootstrap_handler.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: 写测试**

在 `internal/api/bootstrap_remote_callback_test.go` 文件末尾追加：

```go
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
```

- [ ] **Step 2: 验证测试失败**

Run: `go test ./internal/api/ -run TestHandleAdminAccountBootstrap -v`
Expected: 部分测试 FAIL（method=auto 仍走旧 OAuth → WS 路径；DELETE 未实现）。

- [ ] **Step 3: 改写 handleAdminAccountBootstrap**

打开 `internal/api/bootstrap_handler.go`，把 `handleAdminAccountBootstrap` 替换为：

```go
func (server *Server) handleAdminAccountBootstrap(w http.ResponseWriter, r *http.Request) {
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	bootstrap := server.deps.Bootstrap
	if bootstrap == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "bootstrap not configured")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Method string `json:"method"`
		}
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeOpenAIError(w, http.StatusBadRequest, "invalid json: "+err.Error())
				return
			}
		}
		sess, err := bootstrap.Start(body.Method)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "in progress") {
				status = http.StatusConflict
			}
			writeOpenAIError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sess)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeOpenAIError(w, http.StatusBadRequest, "missing id parameter")
			return
		}
		if err := bootstrap.Cancel(id); err != nil {
			status := http.StatusBadRequest
			switch err.Error() {
			case "session not found":
				status = http.StatusNotFound
			default:
				if strings.HasPrefix(err.Error(), "session already") {
					status = http.StatusConflict
				}
			}
			writeOpenAIError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})

	default:
		writeMethodNotAllowed(w, "POST, DELETE")
	}
}
```

确认 `internal/api/bootstrap_handler.go` 的 import 包含 `encoding/json`、`net/http`、`strings`。

- [ ] **Step 4: 验证测试通过**

Run: `go test ./internal/api/ -run TestHandleAdminAccountBootstrap -v -count=1`
Expected: PASS。

- [ ] **Step 5: 验证全包测试无回归**

Run: `go test ./... -count=1`
Expected: 全 PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/api/bootstrap_handler.go internal/api/bootstrap_remote_callback_test.go
git commit -m "feat(api): bootstrap handler accepts method body + DELETE cancel"
```

---

## Task 7: BootstrapManager 并发 + auto fallback 测试

**Files:**
- Create: `internal/api/bootstrap_manager_test.go`

- [ ] **Step 1: 创建测试文件**

创建 `internal/api/bootstrap_manager_test.go`：

```go
package api

import (
	"strings"
	"testing"
	"time"
)

func TestBootstrapManager_StartAuto_FallsBackToRemoteCallback_NoClientID(t *testing.T) {
	mgr, _ := newTestManager(t) // clientID=""

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

	// Manually inject a completed session
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
```

- [ ] **Step 2: 运行**

Run: `go test ./internal/api/ -run TestBootstrapManager -v -count=1`
Expected: PASS。

- [ ] **Step 3: 提交**

```bash
git add internal/api/bootstrap_manager_test.go
git commit -m "test(api): bootstrap manager auto fallback + concurrency + cancel guards"
```

---

## Task 8: 前端类型定义扩展

**Files:**
- Modify: `frontend/src/types/index.ts`

- [ ] **Step 1: 修改 BootstrapResponse**

打开 `frontend/src/types/index.ts`，找到 `export interface BootstrapResponse`，整体替换为：

```typescript
export type BootstrapMethod = 'auto' | 'oauth' | 'ws' | 'remote_callback';

export type BootstrapStatus =
  | 'running'
  | 'awaiting_callback'
  | 'deriving'
  | 'completed'
  | 'error'
  | 'cancelled';

export interface BootstrapResponse {
  id: string;
  status: BootstrapStatus | string;
  method: BootstrapMethod | '';
  auth_url?: string;
  error?: string;
  started_at: string;
  expires_at?: string;
}
```

- [ ] **Step 2: 验证 TypeScript 编译**

Run: `cd /home/zipper/Projects/lingma2api/frontend && npx tsc --noEmit`
Expected: 无类型错误。

- [ ] **Step 3: 提交**

```bash
cd /home/zipper/Projects/lingma2api
git add frontend/src/types/index.ts
git commit -m "feat(frontend): BootstrapMethod/BootstrapStatus type unions"
```

---

## Task 9: 前端 client.ts 增加 startBootstrap(method) + cancelBootstrap

**Files:**
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: 修改 startBootstrap 签名 + 增加 cancelBootstrap**

打开 `frontend/src/api/client.ts`，找到现有 `export const startBootstrap`，替换两行（startBootstrap + 紧随其后的 getBootstrapStatus）为：

```typescript
import type { BootstrapMethod } from '../types';

export const startBootstrap = (method?: BootstrapMethod) =>
  request<BootstrapResponse>('/admin/account/bootstrap', {
    method: 'POST',
    body: method ? JSON.stringify({ method }) : undefined,
  });

export const getBootstrapStatus = (id: string) =>
  request<BootstrapResponse>(`/admin/account/bootstrap/status?id=${encodeURIComponent(id)}`);

export const cancelBootstrap = (id: string) =>
  request<{ status: 'cancelled' }>(`/admin/account/bootstrap?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
```

把文件顶部的 import 部分调整为：

```typescript
import type {
  LogListResult,
  RequestLog,
  DashboardData,
  AccountData,
  ModelMapping,
  BootstrapResponse,
  BootstrapMethod,
  PolicyRule,
  PolicyTestInput,
  PolicyTestResult,
} from '../types';
```

- [ ] **Step 2: 验证 TypeScript 编译**

Run: `cd frontend && npx tsc --noEmit`
Expected: 无错误。

- [ ] **Step 3: 提交**

```bash
cd /home/zipper/Projects/lingma2api
git add frontend/src/api/client.ts
git commit -m "feat(frontend): startBootstrap accepts method, add cancelBootstrap"
```

---

## Task 10: Account 页 UI — 倒计时 + 取消按钮 + 状态文案

**Files:**
- Modify: `frontend/src/pages/Account.tsx`

- [ ] **Step 1: 替换 import**

打开 `frontend/src/pages/Account.tsx`，把第一行 import 改为：

```typescript
import { useState, useEffect, useRef } from 'react';
import { LogIn, RefreshCw, X } from 'lucide-react';
import { getAccount, refreshAccount, startBootstrap, getBootstrapStatus, cancelBootstrap } from '../api/client';
import { StatCard } from '../components/StatCard';
import { Skeleton } from '../components/Skeleton';
import type { AccountData, BootstrapResponse } from '../types';
```

- [ ] **Step 2: 在 Account 函数顶部增加倒计时 state + helper**

紧跟 `const [loading, setLoading] = useState(true);` 一行之后插入：

```typescript
  const [remaining, setRemaining] = useState<string>('');
  const tickRef = useRef<ReturnType<typeof setInterval>>();

  const formatRemaining = (expiresAt?: string): string => {
    if (!expiresAt) return '';
    const ms = new Date(expiresAt).getTime() - Date.now();
    if (ms <= 0) return '已超时';
    const total = Math.floor(ms / 1000);
    const m = Math.floor(total / 60);
    const s = total % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };
```

- [ ] **Step 3: 增加倒计时 effect**

在已有 `useEffect(() => { return () => { ... } }, []);` 之后插入：

```typescript
  useEffect(() => {
    if (bootstrap?.status === 'running' || bootstrap?.status === 'awaiting_callback' || bootstrap?.status === 'deriving') {
      tickRef.current = setInterval(() => {
        setRemaining(formatRemaining(bootstrap?.expires_at));
      }, 1000);
      return () => {
        if (tickRef.current) clearInterval(tickRef.current);
      };
    }
    if (tickRef.current) {
      clearInterval(tickRef.current);
      tickRef.current = undefined;
    }
    setRemaining('');
    return undefined;
  }, [bootstrap?.status, bootstrap?.expires_at]);
```

- [ ] **Step 4: 增加 handleCancel**

在 `handleBootstrap` 函数之后插入：

```typescript
  const handleCancel = async () => {
    if (!bootstrap?.id) return;
    try {
      await cancelBootstrap(bootstrap.id);
      if (pollRef.current) clearInterval(pollRef.current);
      setBootstrap({ ...bootstrap, status: 'cancelled', error: '' });
    } catch (e) {
      setBootstrap({
        ...bootstrap,
        status: 'error',
        error: e instanceof Error ? e.message : String(e),
      });
    }
  };
```

- [ ] **Step 5: 修改状态显示卡片**

找到 `{bootstrap && (` 块，整体替换为：

```tsx
      {bootstrap && (
        <div className="card" style={{ marginBottom: 16, borderLeft: '3px solid var(--primary)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <h4 style={{ margin: 0 }}>
              {bootstrap.status === 'running' && '登录初始化中...'}
              {bootstrap.status === 'awaiting_callback' && '等待浏览器回调'}
              {bootstrap.status === 'deriving' && '派生凭据中...'}
              {bootstrap.status === 'completed' && '登录完成'}
              {bootstrap.status === 'error' && '登录失败'}
              {bootstrap.status === 'cancelled' && '已取消'}
            </h4>
            {(bootstrap.status === 'running' || bootstrap.status === 'awaiting_callback' || bootstrap.status === 'deriving') && (
              <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                {remaining && <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>剩余 {remaining}</span>}
                <button className="btn btn-sm" onClick={handleCancel}>
                  <X size={14} /> 取消
                </button>
              </div>
            )}
          </div>
          {bootstrap.auth_url && (bootstrap.status === 'running' || bootstrap.status === 'awaiting_callback') && (
            <div style={{ marginBottom: 8 }}>
              <p style={{ marginBottom: 8 }}>请在浏览器中打开以下链接完成阿里云登录：</p>
              <a href={bootstrap.auth_url} target="_blank" rel="noopener noreferrer"
                style={{ wordBreak: 'break-all', color: 'var(--primary)', fontWeight: 600 }}>
                {bootstrap.auth_url}
              </a>
              <p style={{ marginTop: 8, fontSize: 13, color: 'var(--text-secondary)' }}>
                登录完成后凭据将自动注入。本页面会在凭据写入后自动刷新。
              </p>
            </div>
          )}
          {bootstrap.status === 'deriving' && (
            <p style={{ color: 'var(--text-secondary)' }}>
              正在通过远程 user/login 派生 cosy_key 与 encrypt_user_info...
            </p>
          )}
          {bootstrap.status === 'completed' && (
            <p style={{ color: 'var(--success)' }}>凭据已成功更新。</p>
          )}
          {bootstrap.status === 'cancelled' && (
            <p style={{ color: 'var(--text-secondary)' }}>已取消登录流程，未写入任何凭据。</p>
          )}
          {bootstrap.status === 'error' && (
            <p style={{ color: 'var(--error)' }}>
              {bootstrap.error || '未知错误'}
              {bootstrap.error?.includes('timeout') && (
                <span> 5 分钟内未完成浏览器登录，请重试。</span>
              )}
              {bootstrap.error?.includes('all remote login strategies failed') && (
                <span> 远程派生 cosy_key 失败（可能被 WAF 拦截），请稍后重试或联系管理员。</span>
              )}
            </p>
          )}
        </div>
      )}
```

- [ ] **Step 6: TypeScript 编译验证**

Run: `cd frontend && npx tsc --noEmit`
Expected: 无错误。

- [ ] **Step 7: 前端构建验证**

Run: `cd frontend && npm run build`
Expected: 构建成功。

- [ ] **Step 8: 提交**

```bash
cd /home/zipper/Projects/lingma2api
git add frontend/src/pages/Account.tsx
git commit -m "feat(frontend): Account page countdown + cancel + remote_callback states"
```

---

## Task 11: 前端 E2E 测试（Playwright，仅在已配 Playwright 时跑）

**Files:**
- Create: `frontend/tests/account-bootstrap.spec.ts`

> **注意：** 当前仓库未必已经配 Playwright。本任务**先创建测试文件，但不要求 CI 跑**。如果 `frontend/package.json` 没有 `@playwright/test`，跳过 Step 3 即可。

- [ ] **Step 1: 创建测试文件**

创建 `frontend/tests/account-bootstrap.spec.ts`：

```typescript
import { test, expect } from '@playwright/test';

const BOOTSTRAP_RESPONSE = {
  id: 'sess-test',
  status: 'awaiting_callback',
  method: 'remote_callback',
  auth_url: 'https://account.alibabacloud.com/login/login.htm?fake=1',
  started_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
};

const STATUS_COMPLETED = {
  ...BOOTSTRAP_RESPONSE,
  status: 'completed',
};

const STATUS_ERROR = {
  ...BOOTSTRAP_RESPONSE,
  status: 'error',
  error: 'timeout: user did not complete login within 5m',
};

test.describe('Account bootstrap flow', () => {
  test('shows auth_url and cancel button after starting', async ({ page }) => {
    await page.route('**/admin/account', route =>
      route.fulfill({ json: { credential: {}, status: { loaded: false }, token_stats: {} } }));
    await page.route('**/admin/account/bootstrap', route => {
      if (route.request().method() === 'POST') {
        return route.fulfill({ json: BOOTSTRAP_RESPONSE });
      }
      return route.continue();
    });
    await page.route('**/admin/account/bootstrap/status*', route =>
      route.fulfill({ json: BOOTSTRAP_RESPONSE }));

    await page.goto('/');
    await page.click('a:has-text("账号")');
    await page.click('button:has-text("重新登录")');

    await expect(page.getByText(BOOTSTRAP_RESPONSE.auth_url)).toBeVisible();
    await expect(page.getByRole('button', { name: /取消/ })).toBeVisible();
  });

  test('cancel button calls DELETE endpoint', async ({ page }) => {
    let deleteCalled = false;
    await page.route('**/admin/account', route =>
      route.fulfill({ json: { credential: {}, status: {}, token_stats: {} } }));
    await page.route('**/admin/account/bootstrap', route => {
      if (route.request().method() === 'POST')
        return route.fulfill({ json: BOOTSTRAP_RESPONSE });
      if (route.request().method() === 'DELETE') {
        deleteCalled = true;
        return route.fulfill({ json: { status: 'cancelled' } });
      }
      return route.continue();
    });
    await page.route('**/admin/account/bootstrap/status*', route =>
      route.fulfill({ json: BOOTSTRAP_RESPONSE }));

    await page.goto('/');
    await page.click('a:has-text("账号")');
    await page.click('button:has-text("重新登录")');
    await page.click('button:has-text("取消")');

    await expect.poll(() => deleteCalled).toBe(true);
  });

  test('shows error message on timeout', async ({ page }) => {
    await page.route('**/admin/account', route =>
      route.fulfill({ json: { credential: {}, status: {}, token_stats: {} } }));
    await page.route('**/admin/account/bootstrap', route =>
      route.fulfill({ json: BOOTSTRAP_RESPONSE }));
    await page.route('**/admin/account/bootstrap/status*', route =>
      route.fulfill({ json: STATUS_ERROR }));

    await page.goto('/');
    await page.click('a:has-text("账号")');
    await page.click('button:has-text("重新登录")');

    await expect(page.getByText(/timeout/i)).toBeVisible();
    await expect(page.getByText(/5 分钟内未完成/)).toBeVisible();
  });
});
```

- [ ] **Step 2: 检查 Playwright 是否已配置**

Run: `cd frontend && cat package.json | grep -i playwright`
Expected: 如果有 `@playwright/test` 依赖则进入 Step 3，否则跳过 Step 3。

- [ ] **Step 3（可选）: 跑 E2E**

Run: `cd frontend && npx playwright test`
Expected: 3 用例 PASS（前提：后端和前端都已启动）。

- [ ] **Step 4: 提交**

```bash
cd /home/zipper/Projects/lingma2api
git add frontend/tests/account-bootstrap.spec.ts
git commit -m "test(frontend): Playwright E2E for account bootstrap remote_callback flow"
```

---

## Task 12: 端到端手工验证

**Files:** 无（手工流程）

- [ ] **Step 1: 准备净环境**

```bash
cd /home/zipper/Projects/lingma2api
# 备份现有凭据
[ -f auth/credentials.json ] && cp auth/credentials.json auth/credentials.json.bak
rm -f auth/credentials.json
# 确保 config.yaml 的 client_id 为空
grep "client_id:" config.yaml
# 期望输出: `client_id: ""`
```

- [ ] **Step 2: 启动服务**

Run: `./start.sh`
Expected: 后端监听 8080，前端 SPA 加载正常。

- [ ] **Step 3: 浏览器手工验证**

打开 http://127.0.0.1:8080/#/account

- 点击「重新登录 / 添加账号」
  - 期望：状态卡显示 `等待浏览器回调`，倒计时 `4:5x`，auth_url 链接显示
- 点击 auth_url 在新标签页中完成阿里云登录
  - 期望：登录后浏览器跳到 127.0.0.1:37510/auth/callback 显示「正在拓印凭据...」→「提交成功，可以关闭窗口。」
- 回到 Account 页等 1-3 秒
  - 期望：状态变成 `登录完成`，UserID/MachineID/CosyKey 等卡片刷新显示真实值

- [ ] **Step 4: 验证 credentials.json 已写入**

Run: `cat auth/credentials.json | jq .auth.cos_y_key | head -c 30 || cat auth/credentials.json | grep cosy_key`
Expected: 看到非空 cosy_key（172 字符左右）。

- [ ] **Step 5: 验证 Chat API 可用**

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"auto","stream":false,"messages":[{"role":"user","content":"你好"}]}'
```
Expected: 200 + JSON 响应含 `choices[0].message.content`。

- [ ] **Step 6: 验证取消行为**

回到 Account 页，再次点击「重新登录」，等到状态进入 `等待浏览器回调`，立即点击「取消」。
Expected: 状态变 `已取消`，37510 端口在 `lsof -i :37510` 中消失。

- [ ] **Step 7: 验证并发拒绝**

启动一次 bootstrap，进入 `等待浏览器回调` 状态，立即在另一个浏览器标签页（或 curl）再次发起：

```bash
curl -X POST http://127.0.0.1:8080/admin/account/bootstrap -H "Content-Type: application/json" -d '{"method":"remote_callback"}'
```
Expected: 409 Conflict，错误信息含 `another bootstrap in progress`。

- [ ] **Step 8: 恢复**

如果手工验证用的是测试账号，记得恢复主账号凭据：

```bash
[ -f auth/credentials.json.bak ] && mv auth/credentials.json.bak auth/credentials.json
```

- [ ] **Step 9: 提交手工验证记录（可选）**

如果手工验证发现需要微调代码，做完调整后再单独 commit。本任务本身**不产生新 commit**。

---

## Self-Review Checklist

写完计划浮浮酱已自查：

1. **Spec 覆盖**：
   - §1 背景缺口 → Task 12 手工验证证明缺口被填上
   - §2 总体架构 → Task 3 的 `runRemoteCallbackFlow` 实现
   - §3 数据流 + 自动注入 HTML → Task 1 + Task 3
   - §4 文件清单 13 项 → Task 1-11 各对应一项
   - §5 接口契约（POST/GET/DELETE） → Task 6 实现
   - §6 安全边界（127.0.0.1 + Origin + 超时 + 并发） → Task 2 + Task 5 + Task 7 测试
   - §7 错误处理表 14 项 → Task 4 + Task 6 + Task 7 测试覆盖关键分支
   - §8 测试策略 → Task 4（6 个端到端）+ Task 7（4 个 manager 单元）+ Task 11（3 个 E2E）+ Task 12（手工 7 步）
   - §9 构建序列 10 步 → Task 1-10 一一对应（少了"自审" 那 1 步因为本身就是元层）
   - §10 兼容承诺 → Task 6 保留旧 POST 行为；Task 8 status 字段降级为 `string` 兼容
   - §11 不在范围内 5 项 → 都未出现在 task 中
   - §12 决策记录 → 已注入到对应 Task 的实现细节里

2. **占位符扫描**：无 TBD/TODO/handle edge cases；所有代码块完整可粘贴

3. **类型一致性**：
   - Go：`StartRemoteCallback` / `runRemoteCallbackFlow` / `findActiveLocked` / `updateSessionStatus` / `Cancel` / `Start` 在 Task 3 + Task 5 间签名一致
   - Frontend：`BootstrapMethod` / `BootstrapStatus` 在 Task 8 定义后被 Task 9 + Task 10 一致使用
   - `cancelBootstrap` 返回 `{ status: 'cancelled' }` 在 Task 9 与 Task 10 调用处一致

无问题。
