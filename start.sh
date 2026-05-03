#!/usr/bin/env bash
# lingma2api 一键生产部署脚本（Linux / macOS）
#
# 用法:
#   ./start.sh                        # 默认 config.yaml
#   ./start.sh -c ./config.yaml       # 自定义配置
#   ./start.sh --skip-frontend        # 跳过前端构建

set -euo pipefail

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$ROOT"

CONFIG="./config.yaml"
SKIP_FRONTEND=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        -c|--config)
            CONFIG="$2"; shift 2;;
        --skip-frontend)
            SKIP_FRONTEND=1; shift;;
        -h|--help)
            sed -n '2,7p' "$0"; exit 0;;
        *)
            echo "[lingma2api] 未知参数: $1" >&2; exit 1;;
    esac
done

GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'; RED='\033[0;31m'; NC='\033[0m'
step() { printf "${CYAN}[lingma2api]${NC} %s\n" "$*"; }
ok()   { printf "${GREEN}[lingma2api]${NC} %s\n" "$*"; }
warn() { printf "${YELLOW}[lingma2api]${NC} %s\n" "$*"; }
err()  { printf "${RED}[lingma2api]${NC} %s\n" "$*" >&2; }

# 1. 工具链检查
step "[1/4] 检查工具链..."
missing=()
command -v go  >/dev/null 2>&1 || missing+=("go")
command -v npm >/dev/null 2>&1 || missing+=("npm")
if [[ ${#missing[@]} -gt 0 ]]; then
    err "缺少工具: ${missing[*]}，请先安装"; exit 1
fi
ok "  go: $(go version | sed 's/^go version //')"
ok "  node: $(node --version)"

# 2. 配置检查（仅警告）
[[ -f "$CONFIG" ]] || warn "未找到配置文件 $CONFIG（继续，二进制启动时会报错）"
[[ -f "./auth/credentials.json" ]] || warn "未找到 ./auth/credentials.json，启动后请通过 OAuth bootstrap 获取（参考 README）"

# 3. 构建前端
if [[ $SKIP_FRONTEND -eq 0 ]]; then
    step "[2/4] 构建前端..."
    pushd ./frontend >/dev/null
    if [[ ! -d node_modules ]]; then
        step "  安装 npm 依赖（首次运行较慢）..."
        npm install
    fi
    npm run build
    popd >/dev/null
    ok "  前端构建完成 -> frontend-dist/"
else
    warn "[2/4] 跳过前端构建（--skip-frontend）"
fi

# 4. 构建后端
step "[3/4] 构建后端二进制..."
BINARY="./lingma2api"
go build -o "$BINARY" .
ok "  二进制 -> $BINARY"

# 5. 启动
step "[4/4] 启动服务..."
ok "  控制台:    http://127.0.0.1:8080"
ok "  OpenAI:    http://127.0.0.1:8080/v1"
ok "  Anthropic: http://127.0.0.1:8080/v1/messages"
echo ""
echo "按 Ctrl+C 停止服务"
echo ""

exec "$BINARY" -config "$CONFIG"
