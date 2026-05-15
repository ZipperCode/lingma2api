# lingma2api

`lingma2api` 是一个最小 OpenAI 兼容代理，对外暴露 `/v1/models`、`/v1/chat/completions` 与 `/v1/messages`，对内复用 Lingma 远端 HTTP/SSE 契约。

## 一键启动

仓库根目录提供跨平台启动脚本，前提：本机已安装 `go`、`node`、`npm`。

### 生产模式（构建并运行）

会构建前端、把 `frontend-dist/` 嵌入 Go 二进制后启动单进程服务。

```powershell
# Windows (PowerShell)
.\start.ps1
```

```bash
# Linux / macOS
chmod +x ./start.sh
./start.sh
```

启动后访问：

- 控制台：`http://<server>:8080`
- OpenAI：`http://<server>:8080/v1`
- Anthropic：`http://<server>:8080/v1/messages`

默认配置会监听 `0.0.0.0:8080`，适合服务器部署。

### 开发模式（前端热更新 + 后端 go run）

并行启动 Vite dev server（`:3000`，热更新）与 Go 后端（`:8080`），Vite 已配置 `/v1` 与 `/admin` 代理到后端。

```powershell
.\dev.ps1
```

```bash
chmod +x ./dev.sh
./dev.sh
```

## Docker 部署

无需安装 Go / Node 环境，通过 Docker 一键启动服务。

### 前置条件

- 已安装 [Docker](https://docs.docker.com/get-docker/) 和 [Docker Compose](https://docs.docker.com/compose/install/)
- 已准备配置文件 `config.yaml`（项目自带默认配置）
- 已准备认证文件 `auth/credentials.json`（参考 `auth/credentials.example.json` 生成）

### 快速启动

```bash
# 1. 构建镜像
docker build -t lingma2api .

# 2. 准备数据目录
mkdir -p data

# 3. 启动服务
docker compose up -d

# 4. 查看日志
docker compose logs -f
```

启动后访问：

- 控制台：`http://localhost:8080`
- OpenAI：`http://localhost:8080/v1`
- Anthropic：`http://localhost:8080/v1/messages`

### 停止服务

```bash
docker compose down
```

### 更新镜像

```bash
# 拉取最新代码后重新构建并启动
git pull
docker compose up -d --build
```

### 卷映射说明

| 宿主机路径 | 容器路径 | 说明 |
|---|---|---|
| `./config.yaml` | `/app/config/config.yaml` | 配置文件（只读） |
| `./auth/` | `/app/auth/` | 认证文件（持久化） |
| `./data/` | `/app/data/` | SQLite 数据库（持久化） |

- `auth/credentials.json` 通过管理页面的"浏览器登录"功能生成
- `data/` 目录持久化存储 SQLite 数据库，容器重启后数据不丢失
- 修改 `config.yaml` 后需重启容器生效：`docker compose restart`

### 首次登录

1. 启动服务后打开 `http://localhost:8080`
2. 进入"账号管理"页面，点击"浏览器登录"
3. 按页面提示完成阿里云登录流程
4. 认证文件将持久化保存在 `auth/credentials.json` 中

## 当前能力

- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/messages`
- `stream=true` 与 `stream=false`
- `GET /admin/status`
- `GET /admin/account`
- `POST /admin/account/bootstrap`
- `POST /admin/account/bootstrap/submit`
- `GET /admin/account/bootstrap/status`
- `POST /admin/account/test`

## 运行态认证边界

运行态只读取项目内的认证文件：

- `credential.auth_file`（默认 `./auth/credentials.json`）

服务启动时不再自动：

- 读取 `~/.lingma/*`
- 连接本地 Lingma WebSocket `127.0.0.1:37010`
- 在服务器本机监听 `127.0.0.1:37510` 等待浏览器自动回调

也就是说，服务器本身不需要浏览器、不需要本地 Lingma 客户端，也不要求存在 `~/.lingma`。

## 服务器版登录流程

推荐使用管理页完成账号接入：

1. 启动服务并打开“账号管理”页
2. 点击“浏览器登录”生成登录链接
3. 在你自己的浏览器中打开该链接并完成阿里云登录
4. 登录完成后，浏览器会跳到 `http://127.0.0.1:37510/...`
5. 即使页面打不开，也直接复制地址栏里的完整回调 URL
6. 把该 URL 粘贴回管理页输入框并提交
7. 服务端解析 `auth` / `token`，生成并保存 `credentials.json`
8. 使用“测试连接”或请求 `/v1/models` 验证

### 关于 `127.0.0.1:37510`

配置里的：

```yaml
lingma:
  oauth_callback_addr: "127.0.0.1:37510"
```

表示“用户浏览器登录完成后要跳转到的本地回调地址”，用于生成 Lingma 登录链接；它不是服务器实际监听的端口。

## 认证文件

请参考：

- `auth/credentials.example.json`

当前推荐路径由 `config.yaml` 中的 `credential.auth_file` 指定，默认值为：

```text
./auth/credentials.json
```

文件中至少要包含：

- `auth.cosy_key`
- `auth.encrypt_user_info`
- `auth.user_id`
- `auth.machine_id`

## 启动

```bash
go run . -config ./config.yaml
```

## 请求示例

```bash
curl -s http://127.0.0.1:8080/v1/models
```

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "auto",
    "stream": true,
    "messages": [
      {"role": "user", "content": "Hello"}
    ]
  }'
```

## 管理接口

如果 `server.admin_token` 非空，则管理接口需要携带以下任一认证头：

- `Authorization: Bearer <admin_token>`
- `X-Admin-Token: <admin_token>`

## 离线辅助工具

以下工具仍保留在仓库中，适合本地研究、迁移或调试使用，但不属于服务器主运行态：

- `cmd/lingma-auth-bootstrap`
- `cmd/lingma-import-cache`
- `cmd/ws-refresh-test`
- `import-auth.sh` / `import-auth.ps1`

主 README 不再把这些工具作为生产接入主路径。

## 限制

- 当前远端传输依赖本机可执行的 `curl`
- 当前实现仅覆盖最小 OpenAI Chat Completions / Anthropic Messages 子集
- `POST /admin/account/refresh` 现在主要用于重新读取磁盘凭据，不再表示本地 WebSocket 续期
