#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════
# 炼丹炉 · runtime-common.sh 回归测试(无框架 bash,红→绿)
#
# 覆盖根因:
#   1. 相对输出路径在切换子目录后失效 → 必须规范化为绝对路径
#   2. 宽目录(空值、/、.、..、仓库根、backend、backend/go)必须被拒绝
#   3. Windows 编码问题 → runtime_python 必须固定 UTF-8 环境
#   4. 含空格路径未引用 → runtime_python 必须正确引用可执行文件与参数
#
# 用法: bash scripts/tests/runtime-common-test.sh
# ═══════════════════════════════════════════════════════════

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LIB="$SCRIPT_DIR/../lib/runtime-common.sh"

if [[ ! -f "$LIB" ]]; then
  printf 'FAIL: 缺少 %s(red: runtime-common.sh 尚未实现)\n' "$LIB"
  exit 1
fi
# shellcheck source=../lib/runtime-common.sh
source "$LIB"

PASS=0
FAILED=0

check() { # check <描述> <实际> <期望>
  local desc="$1" actual="$2" expected="$3"
  if [[ "$actual" == "$expected" ]]; then
    PASS=$((PASS + 1))
    printf 'ok   - %s\n' "$desc"
  else
    FAILED=$((FAILED + 1))
    printf 'FAIL - %s\n      期望: %s\n      实际: %s\n' "$desc" "$expected" "$actual"
  fi
}

contains() { # contains <描述> <内容> <片段>
  local desc="$1" hay="$2" needle="$3"
  if [[ "$hay" == *"$needle"* ]]; then
    PASS=$((PASS + 1))
    printf 'ok   - %s\n' "$desc"
  else
    FAILED=$((FAILED + 1))
    printf 'FAIL - %s(缺少片段: %s)\n      实际输出: %s\n' "$desc" "$needle" "$hay"
  fi
}

expect_reject() { # expect_reject <描述> <路径...>
  local desc="$1"
  shift
  if ( runtime_resolve_output_path "$@" >/dev/null 2>&1 ); then
    FAILED=$((FAILED + 1))
    printf 'FAIL - %s(应拒绝却被接受: %s)\n' "$desc" "$*"
  else
    PASS=$((PASS + 1))
    printf 'ok   - %s\n' "$desc"
  fi
}

# 保证环境断言确定性:先清掉可能从外部带入的变量
unset PYTHONUTF8 PYTHONIOENCODING PIP_DISABLE_PIP_VERSION_CHECK PYTHONDONTWRITEBYTECODE 2>/dev/null || true

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# ─── 1. 相对路径规范化为绝对路径 ───
cd "$TEST_ROOT"
REL_OUT="$(runtime_resolve_output_path "backend/go/dist-runtime")"
check "相对路径 backend/go/dist-runtime 规范化为绝对路径" "$REL_OUT" "$TEST_ROOT/backend/go/dist-runtime"

# ─── 2. 切换子目录后依然有效(返回的是绝对路径,不依赖 cwd)───
cd "$TEST_ROOT/backend/go"
if [[ "$REL_OUT" == /* && "$REL_OUT" == "$TEST_ROOT/backend/go/dist-runtime" ]]; then
  PASS=$((PASS + 1))
  printf 'ok   - 切换子目录后绝对路径依然有效\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - 切换子目录后路径失效: %s\n' "$REL_OUT"
fi

# ─── 3. 宽目录拒绝 ───
cd "$TEST_ROOT"
expect_reject "拒绝空值" ""
expect_reject "拒绝根目录 /" "/"
expect_reject "拒绝 ." "."
expect_reject "拒绝 .." ".."
expect_reject "拒绝仓库根(绝对路径)" "$TEST_ROOT"
expect_reject "拒绝仓库根(相对形式 ./)" "./"
expect_reject "拒绝 backend 目录" "$TEST_ROOT/backend"
expect_reject "拒绝 backend(相对形式)" "backend"
expect_reject "拒绝 backend/go 目录" "$TEST_ROOT/backend/go"
expect_reject "拒绝 backend/go(相对形式)" "backend/go"

# ─── 3b. Windows 盘符绝对路径(Git Bash 下 GITHUB_WORKSPACE 形态)───
# 根因: runtime_resolve_output_path 只认 / 开头为绝对路径,把 D:\a\... 当相对路径
# 拼上 $PWD,产生双前缀(CI 实测 tar 报 Cannot open)。修复后应转 MSYS 风格 /d/a/...
cd "$TEST_ROOT"
check "Windows 盘符路径转 MSYS 绝对路径" \
  "$(runtime_resolve_output_path 'D:\a\alchemy-furnace\alchemy-furnace/backend/go/dist-runtime')" \
  "/d/a/alchemy-furnace/alchemy-furnace/backend/go/dist-runtime"
check "Windows 正斜杠盘符路径" "$(runtime_resolve_output_path 'D:/a/b/c')" "/d/a/b/c"
check "Windows 小写盘符路径" "$(runtime_resolve_output_path 'c:\Users\x')" "/c/Users/x"
check "Windows 路径折叠 .." "$(runtime_resolve_output_path 'D:\a\b\..\c')" "/d/a/c"
expect_reject "拒绝盘符根目录" 'D:\'

# ─── 4. fake python:UTF-8 环境 + 含空格路径引用 + 参数透传 ───
mkdir -p "$TMP/fake bin"
FAKE_PY="$TMP/fake bin/fake python.sh"
cat > "$FAKE_PY" <<'EOF'
#!/usr/bin/env bash
printf 'PYTHONUTF8=%s\n' "${PYTHONUTF8:-unset}"
printf 'PYTHONIOENCODING=%s\n' "${PYTHONIOENCODING:-unset}"
printf 'PIP_DISABLE_PIP_VERSION_CHECK=%s\n' "${PIP_DISABLE_PIP_VERSION_CHECK:-unset}"
printf 'PYTHONDONTWRITEBYTECODE=%s\n' "${PYTHONDONTWRITEBYTECODE:-unset}"
printf 'ARG1=%s\n' "${1:-}"
EOF
chmod +x "$FAKE_PY"

FAKE_OUT="$(runtime_python "$FAKE_PY" "hello world")"
contains "PYTHONUTF8=1" "$FAKE_OUT" "PYTHONUTF8=1"
contains "PYTHONIOENCODING=utf-8" "$FAKE_OUT" "PYTHONIOENCODING=utf-8"
contains "PIP_DISABLE_PIP_VERSION_CHECK=1" "$FAKE_OUT" "PIP_DISABLE_PIP_VERSION_CHECK=1"
contains "PYTHONDONTWRITEBYTECODE=1" "$FAKE_OUT" "PYTHONDONTWRITEBYTECODE=1"
contains "含空格路径的 fake python 被正确执行并引用" "$FAKE_OUT" "PYTHONUTF8=1"
contains "参数透传(含空格内容)" "$FAKE_OUT" "ARG1=hello world"

# ─── 5. 退出码透传 ───
FAKE_EXIT3="$TMP/fake exit3.sh"
printf '#!/usr/bin/env bash\nexit 3\n' > "$FAKE_EXIT3"
chmod +x "$FAKE_EXIT3"
runtime_python "$FAKE_EXIT3" >/dev/null 2>&1
RC=$?
check "runtime_python 透传退出码" "$RC" "3"

# ─── 6. 环境变量不污染调用方 ───
if [[ -z "${PYTHONUTF8:-}" && -z "${PYTHONIOENCODING:-}" && -z "${PIP_DISABLE_PIP_VERSION_CHECK:-}" && -z "${PYTHONDONTWRITEBYTECODE:-}" ]]; then
  PASS=$((PASS + 1))
  printf 'ok   - runtime_python 不污染调用方环境\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - runtime_python 污染了调用方环境\n'
fi

# ─── 7. 复用 runtime 时刷新 engine/app，避免桌面包携带旧 Python 业务代码 ───
mkdir -p "$TMP/source-app" "$TMP/runtime/engine/app/__pycache__"
printf 'new-engine\n' > "$TMP/source-app/main.py"
printf 'new-only\n' > "$TMP/source-app/profile.py"
printf 'stale\n' > "$TMP/runtime/engine/app/stale.py"
printf 'bytecode\n' > "$TMP/runtime/engine/app/__pycache__/main.pyc"
runtime_sync_engine "$TMP/source-app" "$TMP/runtime"
check "runtime 同步最新 main.py" "$(cat "$TMP/runtime/engine/app/main.py")" "new-engine"
if [[ -f "$TMP/runtime/engine/app/profile.py" && ! -e "$TMP/runtime/engine/app/stale.py" && ! -e "$TMP/runtime/engine/app/__pycache__" ]]; then
  PASS=$((PASS + 1))
  printf 'ok   - runtime engine 原子替换并清除旧文件/字节码\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - runtime engine 未完整刷新\n'
fi

# ─── 7. 版本字节断言(ldflags -X 注入防回归)───
# 根因: CI 上 wails 双 build 导致 -ldflags 失效,产物丢失版本字节,
# Windows verifier 拦截,但 macOS verifier 只看 Info.plist 查不出——
# 此断言在 ci-assemble 里对三平台二进制统一把关。
printf 'PK\x03\x04 fake v0.1.1-rc.3 build' > "$TMP/bin-with-version"
printf 'PK\x03\x04 fake build' > "$TMP/bin-no-version"
printf 'UA AlchemyFurnace-Desktop/0.1.1-rc.3' > "$TMP/bin-bare-version"

if assert_version_embedded "$TMP/bin-with-version" "v0.1.1-rc.3" 2>/dev/null; then
  PASS=$((PASS + 1))
  printf 'ok   - 断言:含版本字节的二进制通过\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - 断言:含版本字节的二进制通过\n'
fi

if ! ( assert_version_embedded "$TMP/bin-no-version" "v0.1.1-rc.3" >/dev/null 2>&1 ); then
  PASS=$((PASS + 1))
  printf 'ok   - 断言:缺版本字节的二进制被拒绝\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - 断言:缺版本字节的二进制被拒绝\n'
fi

if assert_version_embedded "$TMP/bin-bare-version" "v0.1.1-rc.3" 2>/dev/null; then
  PASS=$((PASS + 1))
  printf 'ok   - 断言:裸版本字节(不带 v,UA 形态)通过\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - 断言:裸版本字节(不带 v,UA 形态)通过\n'
fi

if ! ( assert_version_embedded "$TMP/missing.bin" "v0.1.1-rc.3" >/dev/null 2>&1 ); then
  PASS=$((PASS + 1))
  printf 'ok   - 断言:二进制缺失被拒绝\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - 断言:二进制缺失被拒绝\n'
fi

# ─── 8. retry 重试(环境性偶发失败防御,rc.4 hdiutil Resource busy 教训)───
mkdir -p "$TMP/retrybin"
cat > "$TMP/retrybin/flaky.sh" <<'EOF'
#!/usr/bin/env bash
# flaky.sh <count-file> <needed> — 记录调用次数,达到 needed 才成功
f="$1"; needed="$2"
c=0
if [[ -f "$f" ]]; then c="$(cat "$f")"; fi
c=$((c + 1))
printf '%s' "$c" > "$f"
[[ "$c" -ge "$needed" ]]
EOF
chmod +x "$TMP/retrybin/flaky.sh"

if retry 3 0 "$TMP/retrybin/flaky.sh" "$TMP/retry-c1" 1; then
  PASS=$((PASS + 1))
  printf 'ok   - retry:首次即成功\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - retry:首次即成功\n'
fi

if retry 3 0 "$TMP/retrybin/flaky.sh" "$TMP/retry-c2" 3; then
  PASS=$((PASS + 1))
  printf 'ok   - retry:失败 2 次后第 3 次成功\n'
else
  FAILED=$((FAILED + 1))
  printf 'FAIL - retry:失败 2 次后第 3 次成功\n'
fi

if retry 2 0 "$TMP/retrybin/flaky.sh" "$TMP/retry-c3" 99; then
  FAILED=$((FAILED + 1))
  printf 'FAIL - retry:应达到次数上限后失败\n'
else
  PASS=$((PASS + 1))
  printf 'ok   - retry:达到次数上限后失败\n'
fi

C3_COUNT="$(cat "$TMP/retry-c3" 2>/dev/null || echo 0)"
check "retry:失败场景只尝试了上限次数" "$C3_COUNT" "2"

printf '\n%d passed, %d failed\n' "$PASS" "$FAILED"
if [[ "$FAILED" -gt 0 ]]; then
  exit 1
fi
exit 0
