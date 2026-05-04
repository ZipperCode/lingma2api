# Remote Callback Bootstrap 设计文档

> **日期：** 2026-05-05
> **目标：** 在 lingma2api 用户管理中新增「脱离本地二进制」的远程 API 登录方式，让用户在没有 `client_id` 配置、没有本地 Lingma 二进制的环境下，也能通过浏览器一键完成阿里云 OAuth 登录并自动派生凭据写入 `auth/credentials.json`。
> **依据：** `docs/topics/callback-37510-simulation.md` Phase 1（37510 OAuth 回调拦截）+ Phase 2（远程 user/login 派生 cosy_key）。
> **范围：** 后端 Admin API + 37510 回调服务器 + 前端 Account 页 UI；不改 Chat API、不改 OAuth/WS 既有路径。

---

## 1. 背景与缺口

### 1.1 当前实现

`internal/api/bootstrap_manager.go` 提供两条 bootstrap 路径，用户在前端 Account 页点击"重新登录 / 添加账号"按钮时按 OAuth → WS 顺序尝试：

| 方法 | 前置条件 | 限制 |
|---|---|---|
| `StartOAuth` | `config.yaml` 的 `lingma.client_id` 非空 | 没有公开 client_id 的用户无法使用 |
| `StartWS` | 本机已运行 Lingma 二进制（`~/.lingma/bin/...`），`auth/credentials.json` 已有 OAuth tokens | 新机器、Linux/server 场景全部失效 |

CLI 层的 `cmd/lingma-auth-bootstrap` 已通过 `runBootstrapWithoutClientID` 实现"无 client_id"流程，但**未集成进 Admin API / 前端按钮**。

### 1.2 缺口

新机器、纯 Linux 服务器、CI 环境用户在 Account 页点击"重新登录"会立刻失败：
- `StartOAuth` 因 `client_id` 未配置直接 `400`
- `StartWS` 因 `credentials.json` 不存在或没有 OAuth tokens `400`

也即"完全脱离本地二进制 + 无需提前配 client_id"的场景下，前端**没有任何登录入口**。

### 1.3 目标

新增第三条 bootstrap 方法 `StartRemoteCallback()`，前置条件为零（既不需要 `client_id` 也不需要本地 Lingma），实现：

1. 后端构造 Lingma login URL（沿用 `auth.BuildLingmaLoginEntryURL`）并返回给前端
2. 后端在 `127.0.0.1:37510` 起一次性 HTTP 监听
3. 用户浏览器完成阿里云登录后被 302 到 `127.0.0.1:37510/auth/callback`
4. 该回调返回的 HTML 自动注入 `<script>`，把 `window.user_info` POST 到 `127.0.0.1:37510/submit-userinfo`
5. 后端解析 user_info → 调远程 `/algo/api/v3/user/login` 派生 `cosy_key + encrypt_user_info` → 写入 `auth/credentials.json`
6. 前端轮询拿到 `completed` 后自动刷新

---

## 2. 总体架构

```
┌──────────────────────────────────────────────────────────────────────┐
│  前端 Account.tsx                                                      │
│  [重新登录] 按钮 → POST /admin/account/bootstrap                       │
│      (可选 body: {"method": "auto"|"remote_callback"|"oauth"|"ws"})   │
│  ← 返回 {id, auth_url, method, status, expires_at}                    │
│  ← 轮询 GET /admin/account/bootstrap/status?id=...                    │
│  ← 取消 DELETE /admin/account/bootstrap?id=...                        │
└──────────────────────────────┬───────────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────────┐
│  BootstrapManager (internal/api/bootstrap_manager.go)                 │
│  Start(method) 分发：                                                  │
│    • oauth              → StartOAuth()        (现有，client_id 必需)  │
│    • ws                 → StartWS()           (现有，本机 Lingma 必需)│
│    • remote_callback    → StartRemoteCallback()  ★ 新增               │
│    • auto (默认)        → oauth (有 client_id) → remote_callback → ws │
│  Cancel(id) → 触发 ctx.Done() 关 37510 → status=cancelled              │
└──────────────────────────────┬───────────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────────┐
│  StartRemoteCallback (internal/api/bootstrap_remote_callback.go)      │
│  1. auth.BuildLingmaLoginEntryURL(MachineID, Port) → login_url        │
│  2. auth.WrapLingmaLoginURLForBrowser(login_url)   → browser_url      │
│  3. 启动 127.0.0.1:37510 回调服务器（5 分钟超时）                       │
│     ├─ GET  /auth/callback   → 注入 <script> 自动 POST user_info     │
│     ├─ POST /submit-userinfo → Origin 校验 → 触发 derive              │
│     └─ 返回 capture                                                    │
│  4. auth.ExtractFromCallbackPage() → access_token + refresh_token+uid │
│  5. auth.DeriveCredentialsRemotely() → cosy_key + encrypt_user_info   │
│  6. auth.SaveCredentialFile(cfg.authFile, stored)                     │
│  7. 更新 session.status=completed                                      │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 3. 数据流

### 3.1 时序

```
Frontend                     Backend                       Browser/Aliyun
────────                     ───────                       ──────────────

POST /admin/account/bootstrap
        ──────────────────>
                         StartRemoteCallback()
                           - 生成 machine_id
                           - 构造 lingma login_url + browser_url
                           - go runRemoteCallbackFlow(id)
                           - 127.0.0.1:37510 监听
        <──────────────── {id, method, auth_url, status:running, expires_at}

弹出 auth_url (target=_blank)
                                                      Open auth_url
                                                          ↓
                                                      Aliyun login
                                                          ↓
                                                      Lingma 回调页注入
                                                      window.user_info / login_url
                                                          ↓
                                                      302 → 37510/auth/callback

轮询 GET /admin/account/bootstrap/status?id=...
        ──────────────────>
        <──────────────── {status:awaiting_callback}

                         GET  /auth/callback
                           ← 返回注入脚本 HTML
                                                      <script> 执行：
                                                      fetch POST /submit-userinfo
                                                      body={userInfo, loginUrl}

                         POST /submit-userinfo
                           - Origin 白名单校验
                           - capture body
                         ExtractFromCallbackPage()
                           - 拿 access_token / refresh_token / uid
                         DeriveCredentialsRemotely()
                           - 调 lingma-api.tongyi /api/v3/user/login
                           - 拿 cosy_key + encrypt_user_info
                         SaveCredentialFile()
                         session.status = completed

轮询 GET status
        ──────────────────>
        <──────────────── {status:completed}

GET /admin/account
        ──────────────────>
        <──────────────── 含 cosy_key 的最新凭据
```

### 3.2 自动注入 HTML

存放在 `internal/auth/callback_html.go` 的常量 `CallbackAutoInjectHTML`：

```html
<!DOCTYPE html>
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
</body></html>
```

### 3.3 Session 状态机

```
running ──> awaiting_callback ──> deriving ──> completed
   │              │                   │
   │              │                   └──> error (derive_failed)
   │              └──> error (timeout / parse_failed / extract_failed)
   └──> error (config_missing / port_in_use)
        ↑
        └── cancelled (Cancel() 触发)
```

---

## 4. 文件清单

| 文件 | 动作 | 职责 |
|---|---|---|
| `internal/api/bootstrap_remote_callback.go` | **新建** | `StartRemoteCallback()` + `runRemoteCallbackFlow()` |
| `internal/api/bootstrap_remote_callback_test.go` | **新建** | 6 个端到端单测（happy / origin / timeout / bad-userinfo / derive-fail / cancel） |
| `internal/api/bootstrap_manager.go` | 修改 | `Start(method)` 分发；`Cancel(id)`；`ExpiresAt` 字段；并发 session 拒绝 |
| `internal/api/bootstrap_manager_test.go` | 修改/新建 | auto fallback + 并发 + cancel 测试 |
| `internal/api/bootstrap_handler.go` | 修改 | 接 `body.method`；新增 `DELETE /admin/account/bootstrap` |
| `internal/auth/bootstrap.go` | 修改 | `WaitForCallbackWithOptions(opts)`；Origin 白名单；自动注入 HTML 分支 |
| `internal/auth/bootstrap_test.go` | 修改 | Origin 校验 + AutoInject HTML 断言 |
| `internal/auth/callback_html.go` | 修改 | 新增 `CallbackAutoInjectHTML` 常量 |
| `internal/auth/callback_html_test.go` | 修改 | 断言注入脚本含 `/submit-userinfo` |
| `frontend/src/types/index.ts` | 修改 | `BootstrapMethod` / `BootstrapStatus` 枚举；`expires_at` 字段 |
| `frontend/src/api/client.ts` | 修改 | `startBootstrap(method?)` / `cancelBootstrap(id)` |
| `frontend/src/pages/Account.tsx` | 修改 | 倒计时显示 / 取消按钮 / 文案 |
| `frontend/src/tests/account-bootstrap.spec.ts` | **新建** | Playwright E2E 3 用例 |
| `docs/superpowers/specs/2026-05-05-remote-callback-bootstrap-design.md` | **新建** | 本 spec |

**不改动：** `internal/auth/remote_login.go` / `internal/auth/userinfo_parse.go` / `internal/auth/credential_derive.go` / `cmd/lingma-auth-bootstrap/main.go` / `internal/auth/ws_refresh.go`。

---

## 5. 接口契约

### 5.1 后端 Admin API

```
POST /admin/account/bootstrap
  body (可选): {"method": "auto"|"oauth"|"ws"|"remote_callback"}
  缺省 = "auto"
  返回:
    200 BootstrapSession
    400 invalid method / 方法所需前置条件未满足
    409 another bootstrap in progress (id=xxx)

GET /admin/account/bootstrap/status?id=...
  返回:
    200 BootstrapSession (含 expires_at)
    404 session not found

DELETE /admin/account/bootstrap?id=...   ★ 新增
  返回:
    200 {"status": "cancelled"}
    404 session not found
    409 session already <status>
```

### 5.2 BootstrapSession 字段

```go
type BootstrapSession struct {
    ID        string    `json:"id"`
    Status    string    `json:"status"`     // running|awaiting_callback|deriving|completed|error|cancelled
    Method    string    `json:"method"`     // oauth|ws|remote_callback
    AuthURL   string    `json:"auth_url,omitempty"`
    Error     string    `json:"error,omitempty"`
    StartedAt time.Time `json:"started_at"`
    ExpiresAt time.Time `json:"expires_at,omitempty"` // ★ 新增
    cancel    context.CancelFunc                       // 非序列化
}
```

### 5.3 BootstrapManager 新方法

```go
// Start 统一分发入口；method 为空或 "auto" 时按
// oauth (有 client_id) → remote_callback → ws (有本机 Lingma) 顺序尝试。
// 每次尝试在前置条件不满足时立即换下一个，不进入 running 态。
func (m *BootstrapManager) Start(method string) (*BootstrapSession, error)

// StartRemoteCallback 构造 Lingma login URL，起 37510 监听，异步派生凭据。
// 前置条件：无（不依赖 client_id 或本机 Lingma 二进制）。
func (m *BootstrapManager) StartRemoteCallback() (*BootstrapSession, error)

// Cancel 主动取消 session；仅 running/awaiting_callback/deriving 态可取消。
func (m *BootstrapManager) Cancel(id string) error
```

### 5.4 auth 包扩展

```go
// internal/auth/bootstrap.go
type WaitForCallbackOptions struct {
    AllowedOrigins []string // /submit-userinfo Origin 白名单
    AutoInjectHTML bool     // /auth/callback 返回注入脚本
}

// 兼容旧 API
func WaitForCallback(ctx context.Context, listenAddr, callbackPath string) (CallbackCapture, error)

// 新 API
func WaitForCallbackWithOptions(ctx context.Context, listenAddr, callbackPath string, opts WaitForCallbackOptions) (CallbackCapture, error)
```

```go
// internal/auth/callback_html.go
const CallbackAutoInjectHTML = `<!DOCTYPE html>...`  // 见 §3.2
```

### 5.5 前端类型

```typescript
// frontend/src/types/index.ts
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
  status: BootstrapStatus;
  method: BootstrapMethod | '';
  auth_url?: string;
  error?: string;
  started_at: string;
  expires_at?: string;
}

// frontend/src/api/client.ts
export function startBootstrap(method?: BootstrapMethod): Promise<BootstrapResponse>;
export function getBootstrapStatus(id: string): Promise<BootstrapResponse>;
export function cancelBootstrap(id: string): Promise<{ status: 'cancelled' }>;
```

---

## 6. 安全边界

| 环节 | 策略 |
|---|---|
| 37510 监听地址 | 强制绑定 `127.0.0.1:37510`，不监听 `0.0.0.0` |
| `/auth/callback` GET | 任意 Origin 均返回 HTML，但脚本 fetch 目标写死 `http://127.0.0.1:37510/submit-userinfo` |
| `/submit-userinfo` POST | 白名单 Origin：`http://127.0.0.1:37510`、空 / `null`；其他一律 403 |
| 并发 session | 同时只允许一个 `running`/`awaiting_callback`/`deriving` session；新 `Start()` → 409 |
| Session 超时 | 默认 5 分钟（`ExpiresAt = StartedAt + 5min`），超时自动 `error: timeout` |
| Session 回收 | 完成 / 错误 / 取消后 10 分钟内仍可查询，之后从 `sessions` map 删除 |

---

## 7. 错误处理

| 场景 | 触发位置 | session.status | error 文案 | HTTP |
|---|---|---|---|---|
| `method` 不在枚举 | bootstrap_handler | — | `invalid method: xxx` | 400 |
| `oauth` 选但 `client_id` 缺 | StartOAuth | — | `client_id not configured` | 400 |
| `ws` 选但本机 Lingma 不可用 | StartWS | — | `no existing credentials...` | 400 |
| 已有 running session 试启新一个 | Start | — | `another bootstrap in progress (id=xxx)` | 409 |
| 37510 端口占用 | StartRemoteCallback | — | `listen 127.0.0.1:37510: address in use` | 500 |
| 用户 5 分钟未完成 | runRemoteCallbackFlow | error | `timeout: user did not complete login within 5m` | 200 (轮询) |
| Origin 不在白名单 | /submit-userinfo | awaiting_callback 保持 | 不结束 session，仅拒该次 POST | 403 |
| `window.user_info` 解析失败 | runRemoteCallbackFlow | error | `parse user_info failed: <msg>` | 200 |
| `machine_id` 无法提取 | runRemoteCallbackFlow | error | `could not extract machine_id from login_url` | 200 |
| `DeriveCredentialsRemotely` 被 WAF 拦 | runRemoteCallbackFlow | error | `derive credentials: all remote login strategies failed: ...` | 200 |
| 写 `credentials.json` 失败 | runRemoteCallbackFlow | error | `save credentials: <err>` | 200 |
| `Cancel(id)` 找不到 | Cancel | — | `session not found` | 404 |
| `Cancel(id)` 已结束 | Cancel | — | `session already <status>` | 409 |

**关键决策：**
1. Origin 错误**不 kill session**，session 自己的超时兜底。
2. Derive 失败 session 直接进入 `error`，**不在中途自动 fallback** 到 ws/oauth；fallback 只发生在 `Start("auto")` 的"尝试前"。
3. Cancel 不清理已写入的 `credentials.json`。

---

## 8. 测试策略

### 8.1 Go 单元测试（`internal/api/bootstrap_remote_callback_test.go`）

```go
func TestStartRemoteCallback_HappyPath(t *testing.T)
  // mock Lingma login URL → 已知 machine_id
  // 模拟浏览器 GET /auth/callback → 验证响应含注入脚本
  // POST /submit-userinfo 带合法 user_info
  // 用 httptest.Server 替换 userLoginURL 返回 cosy_key + encrypt_user_info
  // 断言 credentials.json 落地，session.status == completed

func TestStartRemoteCallback_BadOrigin(t *testing.T)
  // POST /submit-userinfo 带 Origin: https://evil.com → 403
  // session 仍 awaiting_callback

func TestStartRemoteCallback_Timeout(t *testing.T)
  // 超时设 200ms，不发 POST
  // 等 300ms 断言 session.status == error，error 含 "timeout"

func TestStartRemoteCallback_BadUserInfo(t *testing.T)
  // POST /submit-userinfo 带 userInfo = "{}"（缺 securityOauthToken）
  // 断言 session.status == error，error 含 "missing securityOauthToken"

func TestStartRemoteCallback_DeriveFailed(t *testing.T)
  // 合法 user_info，但 mock userLoginURL 返回 403
  // 断言 session.status == error，error 含 "all remote login strategies failed"

func TestStartRemoteCallback_Cancel(t *testing.T)
  // Start → 100ms 后 Cancel(id)
  // 断言 session.status == cancelled
```

### 8.2 Go 单元测试扩展

`internal/api/bootstrap_manager_test.go`：
- `TestBootstrapManager_StartAuto_FallsBackToRemoteCallback`
- `TestBootstrapManager_ConcurrentSessionRejected`
- `TestBootstrapManager_Cancel_NotFound`
- `TestBootstrapManager_Cancel_AlreadyCompleted`

`internal/auth/bootstrap_test.go`：
- `TestWaitForCallback_AutoInjectHTML` —— 响应含 `<script>` + `/submit-userinfo`
- `TestWaitForCallback_OriginWhitelist` —— evil.com 403，127.0.0.1 200

`internal/auth/callback_html_test.go`：
- 断言 `CallbackAutoInjectHTML` 含关键脚本片段

### 8.3 前端 E2E（Playwright，`frontend/src/tests/account-bootstrap.spec.ts`）

```typescript
test('Account: 重新登录 shows auth_url and cancel button', async ({ page }) => {
  // mock /admin/account/bootstrap → remote_callback + auth_url
  // mock status → running, running, completed
  // 断言 auth_url 渲染、取消按钮存在、completed 后页面刷新
});

test('Account: cancel button calls DELETE endpoint', async ({ page }) => {
  // 点击取消 → 断言 DELETE 请求被发起
});

test('Account: error state shows error message', async ({ page }) => {
  // mock status → error + "timeout: user did not complete login within 5m"
  // 断言错误文案渲染
});
```

### 8.4 手工验证清单

| # | 步骤 | 期望 |
|---|---|---|
| 1 | `config.yaml` 清空 `client_id`，删除本机 Lingma 二进制，删除 `auth/credentials.json` | 准备净环境 |
| 2 | `./start.sh` → 打开 http://127.0.0.1:8080 | 控制台正常 |
| 3 | Account 页 → 点"重新登录 / 添加账号" | 弹 auth_url，method=remote_callback |
| 4 | 浏览器打开 auth_url，完成阿里云登录 | 注入脚本执行 → credentials.json 自动写入 |
| 5 | 重新打开 Account 页 | 看到 user_id / machine_id / cosy_key |
| 6 | 重复步骤 3，但中途点取消 | session 置 cancelled，37510 端口释放 |
| 7 | 同时启动两次 bootstrap | 第二次 409 |

---

## 9. 构建序列

| # | 步骤 | 验证 |
|---|---|---|
| 1 | 扩展 `internal/auth/callback_html.go`：新增 `CallbackAutoInjectHTML` 常量 + 单测 | `go test ./internal/auth/...` 绿 |
| 2 | 扩展 `internal/auth/bootstrap.go`：`WaitForCallbackWithOptions` + Origin + AutoInject | `go test ./internal/auth/...` 绿 |
| 3 | 新建 `internal/api/bootstrap_remote_callback.go` | 编译通过 |
| 4 | 新建 `internal/api/bootstrap_remote_callback_test.go`：6 场景 | 6 个测试全绿 |
| 5 | 修改 `bootstrap_manager.go`：分发 + Cancel + ExpiresAt | 扩展测试全绿 |
| 6 | 修改 `bootstrap_handler.go`：method 参数 + DELETE | `go test ./internal/api/...` 绿 |
| 7 | 修改前端 `types/index.ts` + `api/client.ts` | `npm run build` 通过 |
| 8 | 修改前端 `pages/Account.tsx`：倒计时 + 取消按钮 | Vite dev 手工看一眼 |
| 9 | 新建 Playwright 测试：3 用例 | `npx playwright test` 绿 |
| 10 | 手工验证清单第 1-7 项 | `curl /admin/account` 看到 cosy_key |

---

## 10. 向后兼容承诺

- 既有 `POST /admin/account/bootstrap`（无 body）行为：等价于 `method:"auto"`
- `auto` 顺序：有 `client_id` → `oauth`；否则 → `remote_callback`；最后 → `ws`
- `StartOAuth` / `StartWS` 全保留，测试不变
- `cmd/lingma-auth-bootstrap` CLI 不变
- 前端 Account 页"轮询 → 完成后 load()"主流程不变，新增 `auth_url` 总是可用 + 倒计时 + 取消按钮

---

## 11. 不在范围内（明确不做）

- 不实现 Token 自动刷新的远程化（`refresh.go` / `WSRefresher` 保持现状）
- 不实现多账号并发登录（仍是单账号架构，并发 session 直接 409）
- 不实现 cancel 后清理已写入的 `credentials.json`
- 不实现"用户手工粘贴 user_info JSON"输入框（自动注入脚本已覆盖主路径）
- 不实现端口可配置化（37510 沿用 `cfg.Lingma.OAuthListenAddr` 默认值）

---

## 12. 关键决策记录

1. **新增方法 + 智能 fallback**（用户选项）：保留 `StartOAuth` / `StartWS`，新增 `StartRemoteCallback`，`auto` 模式按顺序尝试。
2. **纯 Remote 派生**（用户选项）：失败直接返回错误，不在中途自动 fallback。
3. **自动注入脚本**（用户选项）：在 `/auth/callback` HTML 里注入 `<script>` 自提交，用户零操作。
4. **127.0.0.1 绑定 + Origin 白名单 + Go 端到端 + 前端 E2E + Session 超时/取消**（用户多选）全部纳入 MVP。
5. **`user_info` 不含 cosy_key**：必须经过 `DeriveCredentialsRemotely` 调 `/api/v3/user/login` 派生。如果 WAF 拦截，session error，不自动回退。
