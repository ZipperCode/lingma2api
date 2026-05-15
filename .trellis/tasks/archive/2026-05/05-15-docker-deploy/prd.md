# 添加 Docker 部署支持

## Goal

为 lingma2api 提供 Docker 容器化部署方案，使项目可以在不安装 Go/Node 环境的机器上通过 Docker 一键运行。

## What I already know

- Go 1.24 项目，使用 Go embed 将前端文件嵌入二进制
- 前端是 React + Vite，需先 `npm run build` 输出到 `frontend-dist/`
- `frontend-dist/` 在 `.gitignore` 中（构建产物，不提交）
- SQLite 数据库 (`./lingma2api.db`)
- 配置文件 (`config.yaml`) 和认证文件 (`auth/credentials.json`) 在运行时读取
- 当前启动方式：`start.sh` 先构建前端，再 `go build`，再执行二进制
- 二进制默认监听 `0.0.0.0:8080`
- Go 标准库 `net/http` 无外部 Web 框架
- 当前无 Dockerfile、docker-compose.yml、.dockerignore

## Assumptions (temporary)

- 用户希望通过 Docker 在服务器上简化部署
- 需要处理 SQLite 数据库持久化（不能存储在容器内）
- 需要处理配置文件、认证文件挂载
- 确认：运行时不需要 `curl`（使用 Go `net/http`）

## Requirements

### Dockerfile (multi-stage)
- **Build stage**: `golang:1.24-alpine` — 构建前端 (npm run build) + 编译 Go 二进制
- **Runtime stage**: `alpine:3.20` — 最小运行环境，包含 ca-certificates、时区数据
- 二进制内嵌前端 (`embed.FS`)，无外部静态文件依赖
- 健康检查 (`HEALTHCHECK`)
- 非 root 用户运行

### docker-compose.yml
- 服务定义：`lingma2api`
- 持久化卷映射：
  - `./data/` → 存储 `lingma2api.db`（SQLite）
  - `./config.yaml` → 配置文件（只读）
  - `./auth/` → 认证文件（持久化）
- 端口映射：`8080:8080`
- 自动重启策略

### .dockerignore
- 排除 node_modules/、.git/、本地构建产物等

### 辅助文件
- `docker-compose.yml` 在项目根目录
- `Dockerfile` 在项目根目录
- `.dockerignore` 在项目根目录

## Acceptance Criteria

- [ ] `docker build -t lingma2api .` 构建成功
- [ ] `docker compose up -d` 启动后服务可访问 `http://localhost:8080`
- [ ] 容器重启后 SQLite 数据不丢失
- [ ] 配置文件映射修改后重启容器生效
- [ ] 非 root 用户运行

## Definition of Done

- Dockerfile 构建通过
- docker-compose.yml 启动验证通过
- 数据持久化验证

## Out of Scope (explicit)

- 不涉及 CI/CD 集成
- 不涉及镜像推送（不修改 GitHub Actions）
- 不涉及 K8s 部署
- 不涉及多架构构建 Matrix（暂不加 `docker buildx` workflow）

## Technical Notes

### 项目结构关键路径
- `main.go` — 入口，embed frontend-dist
- `config.yaml` — 运行时配置
- `auth/credentials.json` — 认证凭证
- `./lingma2api.db` — SQLite 数据文件

### 技术确认
- 运行时不需要 `curl`（`curl_transport.go` 存在但 `main.go` 使用 `NativeTransport`，即 Go `net/http`）
- SQLite 使用 `modernc.org/sqlite`（纯 Go 实现，无 CGO 依赖）
- 前端通过 `embed.FS` 嵌入二进制，运行时不需前端文件
- Go 二进制默认监听 `0.0.0.0:8080`
