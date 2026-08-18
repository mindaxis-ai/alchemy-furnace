#!/usr/bin/env bash
# ═══════════════════════════════════════════
# 炼丹炉 · 一键全局启动（本地开发）
#
# 启动顺序：PostgreSQL 检查 → Python 引擎 → Go 网关 → Next.js 前端
# 停止方式：Ctrl+C 一次全部停止（含各服务的孙进程）
#
# 用法：./scripts/dev.sh   或   make dev
# ═══════════════════════════════════════════

set -o pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ─── 颜色与日志前缀 ───
C_RESET=$'\033[0m'
C_SCRIPT=$'\033[32m'   # 绿：脚本自身
C_PY=$'\033[33m'       # 黄：Python 引擎
C_GO=$'\033[36m'       # 青：Go 网关
C_FE=$'\033[35m'       # 紫：前端

say()  { printf '%s[炼丹炉]%s %s\n' "$C_SCRIPT" "$C_RESET" "$*"; }
fail() { printf '%s[炼丹炉] ❌ %s%s\n' "$C_SCRIPT" "$*" "$C_RESET" >&2; exit 1; }

# ─── 0. 工具链检查 ───
# pnpm 需要 Node ≥ 18.12（本机默认可能是 v16），自动从 nvm 目录兜底
node_ok() { command -v node >/dev/null 2>&1 && [ "$(node -v 2>/dev/null | sed 's/^v//; s/\..*//')" -ge 18 ]; }
if ! node_ok; then
  for v in v22.12.0 v20.18.1 v18.20.5; do
    if [ -x "$HOME/.nvm/versions/node/$v/bin/node" ]; then
      export PATH="$HOME/.nvm/versions/node/$v/bin:$PATH"
      say "系统 Node 版本过低，已切换到 nvm $v"
      break
    fi
  done
fi
node_ok || fail "需要 Node.js ≥ 18.12（可用 nvm 安装 v22）"
command -v pnpm >/dev/null 2>&1 || fail "未找到 pnpm，请先：npm install -g pnpm"
command -v go   >/dev/null 2>&1 || fail "未找到 go，请先安装 Go 工具链"

# ─── 1. 加载根目录 .env ───
if [ -f "$ROOT/.env" ]; then
  set -a; . "$ROOT/.env"; set +a
  say "已加载 .env"
else
  say "⚠️  未找到 .env，将使用默认配置（可先 make init）"
fi

FE_PORT="${PORT:-3000}"
GO_PORT="${GO_PORT:-8080}"
PY_PORT="${PYTHON_PORT:-8000}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"

# ─── 2. PostgreSQL：本地已有则用，否则尝试 Docker 兜底 ───
if nc -z "$DB_HOST" "$DB_PORT" 2>/dev/null; then
  say "PostgreSQL 已在 $DB_HOST:$DB_PORT 运行"
elif docker info >/dev/null 2>&1; then
  say "本地未检测到 PostgreSQL，使用 Docker 启动 postgres 容器..."
  docker compose -f "$ROOT/docker-compose.yml" up -d postgres || fail "docker compose 启动 postgres 失败"
  i=0
  until docker compose -f "$ROOT/docker-compose.yml" exec -T postgres \
      pg_isready -U "${DB_USER:-alchemy}" -d "${DB_NAME:-alchemy_db}" >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -ge 30 ] && fail "等待 PostgreSQL 就绪超时"
    sleep 1
  done
  say "PostgreSQL 容器已就绪"
else
  fail "未检测到 PostgreSQL（$DB_HOST:$DB_PORT 无监听，Docker 也未运行）。请先启动本地 postgres 或 Docker Desktop"
fi

# ─── 3. 端口占用检查：先报清楚再退出，避免半启动状态 ───
busy=""
for spec in "$FE_PORT:前端" "$GO_PORT:Go API" "$PY_PORT:Python 引擎"; do
  port="${spec%%:*}"; name="${spec#*:}"
  if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    printf '%s[炼丹炉] ❌ 端口 %s 已被占用（%s）：%s\n' "$C_SCRIPT" "$port" "$name" "$C_RESET" >&2
    lsof -nP -iTCP:"$port" -sTCP:LISTEN | tail -n +2 >&2
    busy=1
  fi
done
[ -n "$busy" ] && exit 1

# ─── 4. 依赖就绪 ───
if [ ! -d "$ROOT/frontend/node_modules" ]; then
  say "首次运行，安装前端依赖（pnpm install）..."
  (cd "$ROOT/frontend" && pnpm install) || fail "pnpm install 失败"
fi
[ -x "$ROOT/backend/python/.venv/bin/uvicorn" ] || \
  fail "未找到 backend/python/.venv，请先：cd backend/python && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt"

# 前端 next/font/google 构建时需访问外网：若本机代理在线且用户未显式设置，则仅给前端进程带上代理
FE_PROXY_ENV=()
if [ -z "$HTTPS_PROXY" ] && nc -z 127.0.0.1 7890 2>/dev/null; then
  FE_PROXY_ENV=(env "HTTPS_PROXY=http://127.0.0.1:7890" "HTTP_PROXY=http://127.0.0.1:7890")
  say "检测到本机代理 127.0.0.1:7890，已为前端字体加载开启代理"
fi

# ─── 5. 启动全部服务 ───
# 任一服务退出 → 记录并通知主进程 → 全部停止，避免留下半运行的环境
DIED_LOG="$(mktemp -t alchemy-dev)"
PIDS=""
EXIT_CODE=0

launch() {
  # launch <名字> <颜色> <工作目录> <命令...>
  local name="$1" color="$2" dir="$3"; shift 3
  {
    ( cd "$dir" && exec "$@" )
    echo "$name exited ($?)" >> "$DIED_LOG"
    kill -TERM $$ 2>/dev/null
  } > >(awk -v tag="$color[$name]$C_RESET" '{ printf "%s %s\n", tag, $0; fflush() }') 2>&1 &
  PIDS="$PIDS $!"
}

cleanup() {
  trap - INT TERM
  [ -s "$DIED_LOG" ] && EXIT_CODE=1
  printf '\n'
  say "🛑 正在停止所有服务..."
  # shellcheck disable=SC2086
  [ -n "$PIDS" ] && kill $PIDS 2>/dev/null
  sleep 1
  # 兜底：按端口清理可能残留的孙进程（go run 的编译产物、next 的 worker 等）
  for port in "$FE_PORT" "$GO_PORT" "$PY_PORT"; do
    p="$(lsof -ti tcp:"$port" 2>/dev/null)"
    # shellcheck disable=SC2086
    [ -n "$p" ] && kill $p 2>/dev/null
  done
  rm -f "$DIED_LOG" 2>/dev/null
  say "✅ 已全部停止"
}
trap 'cleanup; exit $EXIT_CODE' INT TERM

say "🔥 炼丹炉点火中（Go 首次编译约需几秒）..."
launch "python"   "$C_PY" "$ROOT/backend/python"  .venv/bin/uvicorn app.main:app --reload --port "$PY_PORT"
launch "go"       "$C_GO" "$ROOT/backend/go"      go run cmd/main/main.go serve
launch "frontend" "$C_FE" "$ROOT/frontend"        "${FE_PROXY_ENV[@]}" ./node_modules/.bin/next dev -p "$FE_PORT"

printf '\n'
say "🔥 炼丹炉已点燃（开发模式）"
say "   前端:        http://localhost:$FE_PORT"
say "   Go API:     http://localhost:$GO_PORT"
say "   Python 引擎: http://localhost:$PY_PORT"
say "   停止: Ctrl+C"
printf '\n'

wait
cleanup
