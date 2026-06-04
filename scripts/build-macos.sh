#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/build-macos.sh [--skip-tests] [--output-dir dist/macos]

Builds an unsigned portable macOS app bundle:
  dist/macos/LylatLink.app
  dist/lylatlink-macos-<arch>.zip

The bundle includes AppIcon.icns and libopus.0.dylib, so users do not need
Homebrew to run the packaged app.
EOF
}

SKIP_TESTS=0
OUTPUT_DIR="dist/macos"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-tests)
      SKIP_TESTS=1
      shift
      ;;
    --output-dir)
      if [[ $# -lt 2 ]]; then
        echo "--output-dir requires a value" >&2
        exit 2
      fi
      OUTPUT_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ "$(go env GOOS)" != "darwin" ]]; then
  echo "build-macos.sh must run on macOS" >&2
  exit 2
fi

for tool in go pkg-config otool install_name_tool ditto codesign; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "required tool not found: $tool" >&2
    exit 2
  fi
done

if ! pkg-config --exists opus; then
  echo "pkg-config cannot find opus. Install with: brew install opus pkg-config" >&2
  exit 2
fi

if [[ ! -f assets/icon.icns ]]; then
  echo "missing assets/icon.icns" >&2
  exit 2
fi

if [[ "$SKIP_TESTS" != "1" ]]; then
  go test ./...
fi

ARCH="$(go env GOARCH)"
OUT="$ROOT/$OUTPUT_DIR"
APP="$OUT/LylatLink.app"
CONTENTS="$APP/Contents"
MACOS="$CONTENTS/MacOS"
RESOURCES="$CONTENTS/Resources"
FRAMEWORKS="$CONTENTS/Frameworks"
BIN="$MACOS/LylatLink"
HELPER_APP="$OUT/Slippi Dolphin with LylatLink.app"
HELPER_CONTENTS="$HELPER_APP/Contents"
HELPER_MACOS="$HELPER_CONTENTS/MacOS"
HELPER_RESOURCES="$HELPER_CONTENTS/Resources"
HELPER_BIN="$HELPER_MACOS/Slippi Dolphin with LylatLink"
ZIP="$ROOT/dist/lylatlink-macos-$ARCH.zip"
ZIP_ROOT="$OUT/LylatLink-macos-$ARCH"

rm -rf "$APP" "$HELPER_APP" "$ZIP" "$ZIP_ROOT"
mkdir -p "$MACOS" "$RESOURCES" "$FRAMEWORKS" "$HELPER_MACOS" "$HELPER_RESOURCES" "$ROOT/dist"

cp packaging/macos/Info.plist "$CONTENTS/Info.plist"
cp assets/icon.icns "$RESOURCES/AppIcon.icns"
cp packaging/macos/LauncherInfo.plist "$HELPER_CONTENTS/Info.plist"
cp assets/icon.icns "$HELPER_RESOURCES/AppIcon.icns"

CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o "$BIN" ./cmd/lylatlink
chmod +x "$BIN"
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o "$HELPER_BIN" ./cmd/lylatlink-dolphin
chmod +x "$HELPER_BIN"

OPUS_DYLIB="$(otool -L "$BIN" | awk '/libopus.*\.dylib/ {print $1; exit}')"
if [[ -z "$OPUS_DYLIB" ]]; then
  echo "built binary does not appear to link libopus; skipping dylib bundling"
else
  if [[ "$OPUS_DYLIB" = @* ]]; then
    echo "binary already uses relocatable opus path: $OPUS_DYLIB"
  else
    OPUS_NAME="$(basename "$OPUS_DYLIB")"
    cp "$OPUS_DYLIB" "$FRAMEWORKS/$OPUS_NAME"
    chmod 0644 "$FRAMEWORKS/$OPUS_NAME"
    install_name_tool -change "$OPUS_DYLIB" "@executable_path/../Frameworks/$OPUS_NAME" "$BIN"
  fi
fi

codesign --force --deep --sign - --entitlements packaging/macos/entitlements.plist "$APP"
codesign --verify --deep --strict "$APP"
codesign --force --deep --sign - "$HELPER_APP"
codesign --verify --deep --strict "$HELPER_APP"

mkdir -p "$ZIP_ROOT"
ditto "$APP" "$ZIP_ROOT/LylatLink.app"
ditto "$HELPER_APP" "$ZIP_ROOT/Slippi Dolphin with LylatLink.app"

ditto -c -k --sequesterRsrc --keepParent "$ZIP_ROOT" "$ZIP"

echo "Built:"
echo "  $APP"
echo "  $HELPER_APP"
echo "  $ZIP_ROOT"
echo "  $ZIP"
