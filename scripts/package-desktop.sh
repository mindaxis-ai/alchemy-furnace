#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════
# 炼丹炉 · 本地桌面打包(CI 等价:frontend + runtime + wails + assemble)
#
# 用法: scripts/package-desktop.sh <darwin-arm64|darwin-amd64|windows-amd64> [version]
# 产物: backend/go/build/dist/ 下与 GitHub Release 相同的稳定文件名
#
# 注意:
#   - 前端 build 需 Google Fonts: 1panel/nvm proxy 兜底(7890)
#   - python-build-standalone 需访问 GitHub: 同上代理或用户自备
#   - wails 工具需单独安装: go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0
# ═══════════════════════════════════════════════════════════

set -euo pipefail

PLATFORM="${1:?用法: $0 <darwin-arm64|darwin-amd64|windows-amd64> [version]}"
VERSION="${2:-dev}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ─── 颜色 ───
say()  { printf '\033[36m[package]\033[0m %s\n' "$*"; }
fail() { printf '\033[31m[package] ❌ %s\033[0m\n' "$*" >&2; exit 1; }

# ─── 工具链自检 ───
# Next.js 16 requires modern Node; automatically fall back to a known nvm install.
node_ok() { command -v node >/dev/null 2>&1 && [ "$(node -v 2>/dev/null | sed 's/^v//; s/\..*//')" -ge 20 ]; }
if ! node_ok; then
  for v in v22.12.0 v20.18.1 v18.20.5; do
    if [ -x "$HOME/.nvm/versions/node/$v/bin/node" ]; then
      export PATH="$HOME/.nvm/versions/node/$v/bin:$PATH"
      say "Node 切换到 nvm $v"
      break
    fi
  done
fi
node_ok || fail "需要 Node.js ≥ 20(nvm install v22)"
command -v pnpm >/dev/null 2>&1 || fail "未找到 pnpm: npm install -g pnpm"
command -v go >/dev/null 2>&1 || fail "未找到 go"
# go install 默认把 CLI 放在 GOPATH/bin；nvm 切换后 PATH 可能未包含它。
GO_TOOL_BIN="$(go env GOPATH)/bin"
export PATH="$GO_TOOL_BIN:$PATH"
command -v wails >/dev/null 2>&1 || fail "未找到 wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0"

# 前端字体代理(若有本机代理 + 未设 HTTPS_PROXY)
if [ -z "${HTTPS_PROXY:-}" ] && nc -z 127.0.0.1 7890 2>/dev/null; then
  export HTTPS_PROXY="http://127.0.0.1:7890" HTTP_PROXY="http://127.0.0.1:7890"
  say "前端代理 → 127.0.0.1:7890(Google Fonts 兜底)"
fi

# ─── 1. 前端构建 ───
say "前端构建"
( cd "$ROOT/frontend" && pnpm install --frozen-lockfile && pnpm build )

# webui 嵌入准备
rm -rf "$ROOT/backend/go/internal/webui/out"
cp -R "$ROOT/frontend/out" "$ROOT/backend/go/internal/webui/out"

# ─── 2. Python 运行时 ───
say "Python 运行时 → $PLATFORM"
"$ROOT/scripts/build-python-runtime.sh" "$PLATFORM" "$ROOT/backend/go/dist-runtime"

# ─── 3. Wails 编译(注入 ldflags) ───
cd "$ROOT/backend/go"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
REPO="${GITHUB_REPOSITORY:-yusanwen-code/alchemy-furnace}"

WAILS_PLATFORM=""
case "$PLATFORM" in
  darwin-arm64) WAILS_PLATFORM="darwin/arm64" ;;
  darwin-amd64) WAILS_PLATFORM="darwin/amd64" ;;
  windows-amd64) WAILS_PLATFORM="windows/amd64" ;;
  *) fail "未知平台: $PLATFORM" ;;
esac

say "wails build → $WAILS_PLATFORM (v$VERSION)"
rm -rf build/bin
mkdir -p build/bin
wails build -platform "$WAILS_PLATFORM" \
  -ldflags "-X github.com/alchemy-furnace/server/internal/buildinfo.Version=${VERSION} -X github.com/alchemy-furnace/server/internal/buildinfo.Commit=${COMMIT} -X github.com/alchemy-furnace/server/internal/buildinfo.BuildDate=${BUILD_DATE} -X github.com/alchemy-furnace/server/internal/buildinfo.UpdateRepo=${REPO}" \
  -skipbindings

# ─── 4. 装配 + 打 dmg/zip ───
say "装配 + 打包"
"$ROOT/scripts/ci-assemble.sh" "$PLATFORM" "$VERSION"

# ─── 5. 完成 ───
say "✅ 完成: $ROOT/backend/go/build/dist/"
ls -lh "$ROOT/backend/go/build/dist/" | tail -n +2
