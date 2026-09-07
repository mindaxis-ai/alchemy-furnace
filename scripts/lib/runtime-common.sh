#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════
# 炼丹炉 · 跨平台脚本公共库
#
# 只导出函数,被 scripts/*.sh source 使用,不执行任何副作用。
# 统一解决两类跨平台根因:
#   1. 输出目录一律规范化为绝对路径,避免 cd 后相对路径失效;
#   2. 运行 embedded Python 时固定 UTF-8 环境,避免 Windows 编码问题。
# ═══════════════════════════════════════════════════════════

# 打印错误到 stderr 并以 1 退出(调用方可在 source 后自行覆盖)
fail() {
  printf '❌ %s\n' "$*" >&2
  exit 1
}

# runtime_resolve_output_path <path>
# 把(可能相对的)输出路径规范化为绝对路径:
#   - 基于调用时的 cwd 解析相对路径,切换子目录后依然有效;
#   - 折叠 . 与 ..,不要求目录已存在(输出目录可能尚未创建);
#   - 拒绝宽目录:空值、/、.、..、仓库根、仓库根/backend、仓库根/backend/go。
# 成功时把绝对路径打印到 stdout。
runtime_resolve_output_path() {
  local raw="${1:-}"
  if [[ -z "$raw" ]]; then
    fail "拒绝空输出路径"
  fi
  # 宽路径字面值直接拒绝
  if [[ "$raw" == "/" || "$raw" == "." || "$raw" == ".." ]]; then
    fail "拒绝过宽的输出路径: $raw"
  fi

  local abs
  if [[ "$raw" == /* ]]; then
    abs="$raw"
  elif [[ "$raw" =~ ^[A-Za-z]:[\\/](.*)$ ]]; then
    # Windows 盘符绝对路径(Git Bash 下 $GITHUB_WORKSPACE 形态 D:\a\...)
    # 转 MSYS 风格 /d/a/...——否则会被当相对路径拼上 $PWD 产生双前缀
    local letter="${raw:0:1}" rest="${BASH_REMATCH[1]}"
    if [[ -z "$rest" ]]; then
      fail "拒绝盘符根目录作为输出路径: $raw"
    fi
    letter="$(printf '%s' "$letter" | tr 'A-Z' 'a-z')"
    rest="$(printf '%s' "$rest" | tr '\\' '/')"
    abs="/$letter/$rest"
  else
    abs="$PWD/$raw"
  fi

  # 折叠 . 与 ..(不用 cd/realpath,因为目录可能尚未存在)
  local -a segs=() out=()
  local seg
  IFS='/' read -r -a segs <<< "$abs"
  for seg in "${segs[@]}"; do
    if [[ -z "$seg" || "$seg" == "." ]]; then
      continue
    elif [[ "$seg" == ".." ]]; then
      if [[ ${#out[@]} -gt 0 ]]; then
        out=("${out[@]:0:${#out[@]}-1}")
      fi
    else
      out+=("$seg")
    fi
  done

  local canonical="/"
  local s
  for s in "${out[@]}"; do
    canonical="${canonical}${s}/"
  done
  if [[ "$canonical" != "/" ]]; then
    canonical="${canonical%/}"
  fi
  if [[ "$canonical" == "/" ]]; then
    fail "拒绝根目录作为输出路径: $raw"
  fi

  # 仓库根/backend/backend/go 属于"过宽"输出目录(打包脚本会往里写整棵 runtime)
  local ROOT
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  case "$canonical" in
    "$ROOT"|"$ROOT/backend"|"$ROOT/backend/go")
      fail "拒绝过宽的输出路径: $raw"
      ;;
  esac

  printf '%s\n' "$canonical"
}

# runtime_python <python-executable> <arguments...>
# 以固定 UTF-8 环境运行 python,透传其余参数:
#   PYTHONUTF8=1、PYTHONIOENCODING=utf-8、
#   PIP_DISABLE_PIP_VERSION_CHECK=1、PYTHONDONTWRITEBYTECODE=1
# 环境只在函数内部生效,不污染调用方;退出码透传。
runtime_python() {
  local py="${1:?用法: runtime_python <python-executable> [参数...]}"
  shift
  (
    export PYTHONUTF8=1
    export PYTHONIOENCODING=utf-8
    export PIP_DISABLE_PIP_VERSION_CHECK=1
    export PYTHONDONTWRITEBYTECODE=1
    "$py" "$@"
  )
}

# runtime_sync_engine <backend/python/app> <runtime-root>
# 已安装依赖的 runtime 可离线复用，但业务源码必须每次打包都刷新。
# 通过同目录暂存后替换 engine/app，避免拷贝中断留下半套源码。
runtime_sync_engine() {
  local source_app="${1:?用法: runtime_sync_engine <source-app> <runtime-root>}"
  local runtime_root="${2:?用法: runtime_sync_engine <source-app> <runtime-root>}"
  [[ -f "$source_app/main.py" ]] || fail "Python 引擎源码不完整: $source_app/main.py"
  [[ -d "$runtime_root" ]] || fail "Python runtime 不存在: $runtime_root"

  local engine_root="$runtime_root/engine"
  local target="$engine_root/app"
  mkdir -p "$engine_root"
  local staged
  staged="$(mktemp -d "$engine_root/.app.tmp.XXXXXX")"
  if ! cp -R "$source_app"/. "$staged"/; then
    rm -rf "$staged"
    fail "同步 Python 引擎源码失败"
  fi
  find "$staged" -type d -name '__pycache__' -prune -exec rm -rf {} + 2>/dev/null || true
  rm -rf "$target"
  mv "$staged" "$target"
}

# assert_version_embedded <binary> <version-tag>
# 断言二进制内包含版本字节(ldflags -X 注入的产物)。
# tag 形如 v0.1.1-rc.3,带 v 与不带 v 两种形态都算数
# (GetVersion 注入完整 tag;User-Agent 用 AlchemyFurnace-Desktop/<version>)。
# 用于 CI 打包前对三平台二进制统一把关——macOS verifier 只看 Info.plist,
# 查不出二进制丢失版本字节(rc.3 教训),此断言补上该缺口。
assert_version_embedded() {
  local binary="${1:?用法: assert_version_embedded <binary> <version-tag>}"
  local tag="${2:?用法: assert_version_embedded <binary> <version-tag>}"
  [ -f "$binary" ] || fail "二进制不存在: $binary"
  if ! grep -aqF "$tag" "$binary" && ! grep -aqF "${tag#v}" "$binary"; then
    fail "二进制未包含版本字节(期望 ${tag#v}): $binary"
  fi
}

# retry <attempts> <delay-seconds> <command...>
# 命令失败时重试,最多 attempts 次,每次间隔 delay 秒(支持小数)。
# 环境性偶发失败的防御(rc.4 darwin 打包 hdiutil create Resource busy 实测),
# 真·失败(配置/权限/资源耗尽)在尝试上限后原样返回失败,不掩盖。
retry() {
  local attempts="${1:?用法: retry <attempts> <delay-seconds> <command...>}"
  local delay="${2:?用法: retry <attempts> <delay-seconds> <command...>}"
  shift 2
  local n=0
  until "$@"; do
    n=$((n + 1))
    if [[ "$n" -ge "$attempts" ]]; then
      return 1
    fi
    sleep "$delay"
  done
  return 0
}
