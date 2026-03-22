#!/bin/bash
# Build coral-go Windows portable ZIP (cross-compiled from Linux/macOS)
#
# Usage: ./installers/build-windows.sh [version]
# Output: installers/dist/coral-windows-amd64.zip

set -euo pipefail

VERSION="${1:-dev}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GO_DIR="$PROJECT_DIR/coral-go"
DIST_DIR="$SCRIPT_DIR/dist"
BUILD_DIR="$DIST_DIR/coral-windows"

echo "==> Building coral-go for Windows (amd64) v${VERSION}"

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

cd "$GO_DIR"

echo "==> Compiling coral.exe"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BUILD_DIR/coral.exe" ./cmd/coral/

echo "==> Compiling launch-coral.exe"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BUILD_DIR/launch-coral.exe" ./cmd/launch-coral/

echo "==> Compiling coral-board.exe"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BUILD_DIR/coral-board.exe" ./cmd/coral-board/

echo "==> Creating ZIP"
cd "$DIST_DIR"
rm -f "coral-windows-amd64-${VERSION}.zip"
zip -j "coral-windows-amd64-${VERSION}.zip" coral-windows/coral.exe coral-windows/launch-coral.exe coral-windows/coral-board.exe

echo "==> Cleaning up build dir"
rm -rf "$BUILD_DIR"

echo ""
echo "Done! Installer at: $DIST_DIR/coral-windows-amd64-${VERSION}.zip"
echo ""
echo "To test on Windows:"
echo "  1. Unzip to a folder (e.g. C:\\coral\\)"
echo "  2. Run: coral.exe --backend pty --host 127.0.0.1 --port 8420"
echo "  3. Open browser to http://127.0.0.1:8420"
