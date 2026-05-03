#!/usr/bin/env bash
# 从本机已登录的 Lingma 程序提取认证信息，导入到项目的 auth/credentials.json
#
# 包装 cmd/lingma-import-cache 工具：
#   1. 检查 Lingma 缓存目录是否存在（默认 ~/.lingma）
#   2. 调用 go run ./cmd/lingma-import-cache 读取缓存并派生凭据
#   3. 写出到项目的 auth/credentials.json
#
# 仅作一次性迁移使用。运行态服务不会再读取 ~/.lingma。
#
# 用法:
#   ./import-auth.sh                                      # 默认参数
#   ./import-auth.sh -d ~/.lingma -o ./auth/credentials.json
#   ./import-auth.sh --force                              # 已存在时直接覆盖

set -euo pipefail

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$ROOT"

LINGMA_DIR="${HOME}/.lingma"
OUTPUT="./auth/credentials.json"
FORCE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        -d|--lingma-dir) LINGMA_DIR="$2"; shift 2;;
        -o|--output)     OUTPUT="$2"; shift 2;;
        -f|--force)      FORCE=1; shift;;
        -h|--help)       sed -n '2,15p' "$0"; exit 0;;
        *)               echo "[lingma2api/import] 未知参数: $1" >&2; exit 1;;
    esac
done

GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'; RED='\033[0;31m'; NC='\033[0m'
step() { printf "${CYAN}[lingma2api/import]${NC} %s\n" "$*"; }
ok()   { printf "${GREEN}[lingma2api/import]${NC} %s\n" "$*"; }
warn() { printf "${YELLOW}[lingma2api/import]${NC} %s\n" "$*"; }
err()  { printf "${RED}[lingma2api/import]${NC} %s\n" "$*" >&2; }

# 1. 检查
step "[1/3] 检查环境..."
command -v go >/dev/null 2>&1 || { err "未找到 go，请先安装"; exit 1; }
if [[ ! -d "$LINGMA_DIR" ]]; then
    err "Lingma 目录不存在: $LINGMA_DIR"
    err "请确认本机已安装并登录过 Lingma 客户端"
    exit 1
fi
[[ -d "$LINGMA_DIR/cache" ]] || warn "未找到 $LINGMA_DIR/cache（导入可能失败，请先在 Lingma 客户端完成一次登录）"
ok "  Lingma 目录: $LINGMA_DIR"

# 2. 输出处理
OUTPUT_ABS="$(cd "$(dirname "$OUTPUT")" 2>/dev/null && pwd)/$(basename "$OUTPUT")" 2>/dev/null || OUTPUT_ABS="$ROOT/${OUTPUT#./}"
OUTPUT_DIR="$(dirname "$OUTPUT_ABS")"
if [[ ! -d "$OUTPUT_DIR" ]]; then
    mkdir -p "$OUTPUT_DIR"
    ok "  创建输出目录: $OUTPUT_DIR"
fi

if [[ -f "$OUTPUT_ABS" && $FORCE -eq 0 ]]; then
    warn "  目标文件已存在: $OUTPUT_ABS"
    read -r -p "覆盖? [y/N] " reply
    case "$reply" in
        [yY]*) ;;
        *) warn "已取消（使用 --force 跳过本提示）"; exit 0;;
    esac
fi

# 3. 调用 lingma-import-cache
step "[2/3] 从 Lingma 缓存提取并派生凭据..."
go run ./cmd/lingma-import-cache --lingma-dir "$LINGMA_DIR" --output "$OUTPUT_ABS"

# 4. 校验
step "[3/3] 校验输出..."
if [[ ! -f "$OUTPUT_ABS" ]]; then
    err "导入命令成功但目标文件未生成: $OUTPUT_ABS"
    exit 1
fi
BYTES=$(wc -c < "$OUTPUT_ABS" | tr -d ' ')
ok "  写入: $OUTPUT_ABS ($BYTES bytes)"

# 5. 提醒同步 config.yaml
CFG="$ROOT/config.yaml"
if [[ -f "$CFG" ]]; then
    if ! grep -qF "$OUTPUT_ABS" "$CFG" && ! grep -qE 'auth_file:\s*"\./auth/credentials\.json"' "$CFG"; then
        warn "提示: config.yaml 的 credential.auth_file 可能未指向 $OUTPUT_ABS"
        warn "  打开 config.yaml 检查 credential.auth_file"
    fi
fi

ok "完成喵～现在可以运行 ./start.sh 启动服务"
