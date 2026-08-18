#!/usr/bin/env bash
# Assemble target-native Wails output and Python runtime into release assets.
# Usage: scripts/ci-assemble.sh <darwin-arm64|darwin-amd64|windows-amd64> [version]

set -euo pipefail

PLATFORM="${1:?用法: $0 <darwin-arm64|darwin-amd64|windows-amd64> [version]}"
VERSION="${2:-${VERSION:-dev}}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Override is intended for isolated packaging tests and downstream reuse.
GO_DIR="${ALCHEMY_GO_DIR:-$ROOT/backend/go}"
BIN_DIR="$GO_DIR/build/bin"
DIST_DIR="$GO_DIR/build/dist"
RUNTIME_SRC="$GO_DIR/dist-runtime"

say()  { printf '\033[36m[assemble]\033[0m %s\n' "$*"; }
fail() { printf '\033[31m[assemble] ❌ %s\033[0m\n' "$*" >&2; exit 1; }

mkdir -p "$DIST_DIR"
[ -d "$RUNTIME_SRC/engine/app" ] || fail "运行时缺少 engine/app: $RUNTIME_SRC"

case "$PLATFORM" in
  darwin-arm64|darwin-amd64)
    APP="$BIN_DIR/AlchemyFurnace.app"
    [ -x "$APP/Contents/MacOS/AlchemyFurnace" ] || fail "未找到 Wails 应用: $APP"
    [ -x "$RUNTIME_SRC/bin/python3" ] || fail "运行时缺少 bin/python3: $RUNTIME_SRC"

    case "$PLATFORM" in
      darwin-arm64) ASSET_ARCH="arm64" ;;
      darwin-amd64) ASSET_ARCH="x64" ;;
    esac

    RES="$APP/Contents/Resources"
    RUNTIME_DST="$RES/python-runtime"
    rm -rf "$RUNTIME_DST"
    mkdir -p "$RUNTIME_DST"
    say "装入 Python 运行时 → $RUNTIME_DST"
    cp -R "$RUNTIME_SRC/." "$RUNTIME_DST/"

    # Wails build happens before runtime assembly. Re-sign afterwards or the
    # bundle's original signature becomes invalid as soon as Resources changes.
    PLIST="$APP/Contents/Info.plist"
    BUNDLE_VERSION="${VERSION#v}"
    BUNDLE_VERSION="${BUNDLE_VERSION%%-*}"
    if [[ ! "$BUNDLE_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      BUNDLE_VERSION="0.0.0"
    fi
    /usr/libexec/PlistBuddy -c "Set :CFBundleVersion $BUNDLE_VERSION" "$PLIST"
    /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $BUNDLE_VERSION" "$PLIST"

    SIGN_IDENTITY="${MACOS_SIGN_IDENTITY:--}"
    say "签名应用(identity: $SIGN_IDENTITY)"
    codesign --force --deep --sign "$SIGN_IDENTITY" "$APP"
    codesign --verify --deep --strict --verbose=2 "$APP"

    DMG="$DIST_DIR/AlchemyFurnace-mac-${ASSET_ARCH}.dmg"
    ZIP="$DIST_DIR/AlchemyFurnace-mac-${ASSET_ARCH}.zip"
    STAGE="$(mktemp -d)"
    trap 'rm -rf "$STAGE"' EXIT
    cp -R "$APP" "$STAGE/"
    ln -s /Applications "$STAGE/Applications"
    say "生成 $(basename "$DMG")"
    rm -f "$DMG"
    hdiutil create -volname "AlchemyFurnace" -srcfolder "$STAGE" \
      -ov -format UDZO "$DMG" >/dev/null

    say "生成 $(basename "$ZIP")"
    rm -f "$ZIP"
    ditto -c -k --sequesterRsrc --keepParent "$APP" "$ZIP"
    ;;

  windows-amd64)
    EXE="$BIN_DIR/AlchemyFurnace.exe"
    [ -f "$EXE" ] || fail "未找到 Wails 可执行文件: $EXE"
    [ -f "$RUNTIME_SRC/python.exe" ] || fail "运行时缺少 python.exe: $RUNTIME_SRC"
    command -v makensis >/dev/null 2>&1 || fail "未找到 makensis(NSIS)"

    PACKAGE_DIR="$GO_DIR/build/package/windows"
    rm -rf "$PACKAGE_DIR"
    mkdir -p "$PACKAGE_DIR/runtime"
    cp "$EXE" "$PACKAGE_DIR/AlchemyFurnace.exe"
    cp -R "$RUNTIME_SRC/." "$PACKAGE_DIR/runtime/"

    INSTALLER="$DIST_DIR/AlchemyFurnace-Setup.exe"
    rm -f "$INSTALLER"
    say "生成 $(basename "$INSTALLER")(包含 runtime/)"
    (
      cd "$GO_DIR"
      makensis \
        -DAPP_SOURCE="build/package/windows" \
        -DOUTPUT_FILE="build/dist/AlchemyFurnace-Setup.exe" \
        build/windows/installer.nsi
    )
    [ -s "$INSTALLER" ] || fail "NSIS 未生成安装器: $INSTALLER"
    ;;

  *) fail "未知平台: $PLATFORM" ;;
esac

say "✅ $PLATFORM ${VERSION} 产物:"
find "$DIST_DIR" -maxdepth 1 -type f -name 'AlchemyFurnace-*' -print
