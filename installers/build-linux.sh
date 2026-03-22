#!/bin/bash
# Build coral-go Linux portable tarball
#
# Usage: ./installers/build-linux.sh [version]
# Output: installers/dist/coral-linux-amd64-<version>.tar.gz

set -euo pipefail

VERSION="${1:-dev}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GO_DIR="$PROJECT_DIR/coral-go"
DIST_DIR="$SCRIPT_DIR/dist"
BUILD_DIR="$DIST_DIR/coral-linux"

echo "==> Building coral-go for Linux (amd64) v${VERSION}"

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

cd "$GO_DIR"

echo "==> Compiling coral"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BUILD_DIR/coral" ./cmd/coral/

echo "==> Compiling launch-coral"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BUILD_DIR/launch-coral" ./cmd/launch-coral/

echo "==> Compiling coral-board"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BUILD_DIR/coral-board" ./cmd/coral-board/

echo "==> Creating tarball"
cd "$DIST_DIR"
TARBALL="coral-linux-amd64-${VERSION}.tar.gz"
rm -f "$TARBALL"
tar czf "$TARBALL" -C coral-linux .

echo "==> Cleaning up build dir"
rm -rf "$BUILD_DIR"

echo ""
echo "Done! Installer at: $DIST_DIR/$TARBALL"
echo ""
echo "To install:"
echo "  tar xzf $TARBALL -C /usr/local/bin/"
echo "  coral --host 127.0.0.1 --port 8420"
