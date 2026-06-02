#!/usr/bin/env bash
# Cross-compile Nib for all platforms and build Linux .deb packages.
# Usage: ./build.sh [version]   (version defaults to "dev")
#
# Produces a single cgo-free static binary per OS/arch in dist/, plus a .deb for
# linux amd64/arm64 when nfpm is installed (go install
# github.com/goreleaser/nfpm/v2/cmd/nfpm@latest).
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${1:-$(cat VERSION 2>/dev/null || echo dev)}"
DIST="dist"
rm -rf "$DIST"
mkdir -p "$DIST"

targets=(
  "linux/amd64" "linux/arm64"
  "darwin/amd64" "darwin/arm64"
  "windows/amd64" "windows/arm64"
)

for t in "${targets[@]}"; do
  os="${t%/*}"; arch="${t#*/}"
  ext=""; [ "$os" = "windows" ] && ext=".exe"
  out="$DIST/nib-$VERSION-$os-$arch$ext"
  echo "building $out"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" -o "$out" ./cmd/nib
done

if command -v nfpm >/dev/null 2>&1; then
  for arch in amd64 arm64; do
    echo "packaging nib_${VERSION}_${arch}.deb"
    cp "$DIST/nib-$VERSION-linux-$arch" "$DIST/nib" # nfpm.yaml references dist/nib
    ARCH="$arch" VERSION="$VERSION" \
      nfpm package --config build/nfpm.yaml --packager deb --target "$DIST/nib_${VERSION}_${arch}.deb"
  done
  rm -f "$DIST/nib"
else
  echo "nfpm not found — skipping .deb. Install: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest"
fi

echo "done — artifacts in $DIST/"
