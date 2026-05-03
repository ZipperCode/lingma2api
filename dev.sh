#!/usr/bin/env bash
# lingma2api 开发模式启动脚本（Linux / macOS）
#
# 并行启动:
#   - Vite dev server (http://127.0.0.1:3000) 后台
#   - Go 后端 (http://127.0.0.1:8080)         前台
# Vite 已配置代理，访问 http://127.0.0.1:3000 即可获得热更新体验。
#
# 用法:
#   ./dev.sh                  # 默认 config.yaml
#   ./dev.sh -c ./config.yaml # 自定义配置

set -euo pipefail

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$ROOT"

CONFIG="./config.yaml"
while [[ $# -gt 0 ]]; do
    case "$1" in
        -c|--config) CONFIG="$2"; shift 2;;
        -h|--help)   sed -n '2,12p' "$0"; exit 0;;
        *)           echo "[lingma2api/dev] 未知参数: $1" >&2; exit 1;;
    esac
done

GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'; RED='\033[0;31m'; NC='\033[0m'
step() { printf "${CYAN}[lingma2api/dev]${NC} %s\n" "$*"; }
ok()   { printf "${GREEN}[lingma2api/dev]${NC} %s\n" "$*"; }
warn() { printf "${YELLOW}[lingma2api/dev]${NC} %s\n" "$*"; }
err()  { printf "${RED}[lingma2api/dev]${NC} %s\n" "$*" >&2; }

# 1. 工具链检查
step "[1/3] 检查工具链..."
missing=()
command -v go  >/dev/null 2>&1 || missing+=("go")
command -v npm >/dev/null 2>&1 || missing+=("npm")
if [[ ${#missing[@]} -gt 0 ]]; then
    err "缺少工具: ${missing[*]}，请先安装"; exit 1
fi

# 2. 准备前端依赖
if [[ ! -d ./frontend/node_modules ]]; then
    step "[2/3] 首次运行，安装前端依赖..."
    (cd ./frontend && npm install)
else
    step "[2/3] 前端依赖已就绪"
fi

# 3. 启动 Vite dev server（后台）
step "[3/3] 启动 Vite dev server (后台) ..."
(cd ./frontend && npm run dev) &
VITE_PID=$!
ok "  Vite PID: $VITE_PID"

cleanup() {
    step "清理 Vite dev server (PID $VITE_PID) ..."
    # 杀整个进程组（npm -> node 等子进程）
    if kill -0 "$VITE_PID" 2>/dev/null; then
        # 优先尝试进程组
        if kill -- -"$VITE_PID" 2>/dev/null; then :;
        else
            kill "$VITE_PID" 2>/dev/null || true
        fi
    fi
    pkill -P "$VITE_PID" 2>/dev/null || true
    ok "已停止"
}
trap cleanup EXIT INT TERM

sleep 1
echo ""
ok "  前端 (热更新): http://127.0.0.1:3000"
ok "  后端:          http://127.0.0.1:8080"
ok "  推荐访问:      http://127.0.0.1:3000  (Vite 自动代理 API)"
echo ""
echo "按 Ctrl+C 停止"
echo ""

go run . -config "$CONFIG"
