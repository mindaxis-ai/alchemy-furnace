#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════
# 炼丹炉 · 桌面安装包装配(CI + 本地共用)
#
# 流程(以 darwin-arm64 为例):
#   1. 已 wails build → build/bin/AlchemyFurnace.app
#   2. 拷 python-runtime 到 .app/Contents/Resources/python-runtime
#   3. hdiutil 打 .dmg + zip 打 .zip
#   4. 产物落到 build/dist/
#
# windows-amd64:
#   1. 已 wails build -nsis → build/bin/AlchemyFurnace-Setup.exe
#      (若 wails 不出 nsis,改用 makensis 手工打,本脚本先做 cp runtime + zip)
#   2. 拷 python-runtime 到 build/bin/runtime/
#   3. zip 整体成 AlchemyFurnace-windows-amd64.zip
#   4. 产物落到 build/dist/
# ═══════════════════════════════════════════════════════════

set -euo pipefail

PLATFORM="${1:?用法: $0 <darwin-arm64|darwin-amd64|windows-amd64>}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_DIR="$ROOT/backend/go"
BIN_DIR="$GO_DIR/build/bin"
DIST_DIR="$GO_DIR/build/dist"
RUNTIME_SRC="$GO_DIR/dist-runtime"

mkdir -p "$DIST_DIR"

say()  { printf '\033[36m[assemble]\033[0m %s\n' "$*"; }
fail() { printf '\033[31m[assemble] ❌ %s\033[0m\n' "$*" >&2; exit 1; }

[ -d "$RUNTIME_SRC" ] || fail "未找到 dist-runtime/(先跑 build-python-runtime.sh)"

VERSION="${VERSION:-dev}"

case "$PLATFORM" in
  darwin-arm64|darwin-amd64)
    APP="$BIN_DIR/AlchemyFurnace.app"
    [ -d "$APP" ] || fail "未找到 $APP(先跑 wails build)"
    RES="$APP/Contents/Resources"
    mkdir -p "$RES"
    say "装 python-runtime → $RES/python-runtime"
    cp -R "$RUNTIME_SRC" "$RES/python-runtime"

    # .dmg(可拖拽 .app 到 Applications 的磁盘镜像)
    DMG_NAME="AlchemyFurnace-${PLATFORM}-${VERSION}.dmg"
    STAGE="$(mktemp -d)"
    cp -R "$APP" "$STAGE/"
    ln -s /Applications "$STAGE/Applications"
    say "打 dmg: $DMG_NAME"
    hdiutil create -volname "AlchemyFurnace" -srcfolder "$STAGE" \
      -ov -format UDZO "$DIST_DIR/$DMG_NAME" >/dev/null
    rm -rf "$STAGE"

    # .zip(直接解压的备选分发)
    ZIP_NAME="AlchemyFurnace-${PLATFORM}-${VERSION}.zip"
    say "打 zip: $ZIP_NAME"
    (cd "$BIN_DIR" && zip -ry "$DIST_DIR/$ZIP_NAME" AlchemyFurnace.app >/dev/null)
    ;;

  windows-amd64)
    # wails build -nsis 输出 Setup.exe;若无 nsis,改打包 AlchemyFurnace.exe + runtime
    SETUP="$BIN_DIR/AlchemyFurnace-Setup.exe"
    if [ -f "$SETUP" ]; then
      say "使用 NSIS Setup.exe"
      cp "$SETUP" "$DIST_DIR/AlchemyFurnace-Setup-${VERSION}.exe"
    else
      EXE="$BIN_DIR/AlchemyFurnace.exe"
      [ -f "$EXE" ] || fail "未找到 $EXE(先跑 wails build)"
      say "无 NSIS,改 zip 打包 exe + runtime"
      RUNTIME_DST="$BIN_DIR/runtime"
      cp -R "$RUNTIME_SRC" "$RUNTIME_DST"
      ZIP_NAME="AlchemyFurnace-${PLATFORM}-${VERSION}.zip"
      (cd "$BIN_DIR" && zip -r "$DIST_DIR/$ZIP_NAME" AlchemyFurnace.exe runtime >/dev/null)
    fi
    ;;

  *) fail "未知平台: $PLATFORM" ;;
esac

say "✅ 产物:"
ls -lh "$DIST_DIR" | tail -n +2 | awk '{print "  " $NF " (" $5 ")"}'
