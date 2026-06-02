#!/usr/bin/env bash
# Cross-compile Nib for all platforms and build Linux .deb packages.
# Usage: ./build.sh [version] [--publish]   (version defaults to the VERSION file)
#
# Produces a single cgo-free static binary per OS/arch in dist/, plus a .deb for
# linux amd64/arm64 when nfpm is installed (go install
# github.com/goreleaser/nfpm/v2/cmd/nfpm@latest).
#
# With --publish it also pushes the current branch and uploads the public
# binaries to a GitHub release tagged v<version> (needs the gh CLI). build.sh
# only ever builds the public `nib` binaries — never the embed-keys nib-sig
# variant — so what it publishes is always safe to share.
set -euo pipefail
cd "$(dirname "$0")"

PUBLISH=0
VERSION=""
for arg in "$@"; do
  case "$arg" in
    --publish|-p) PUBLISH=1 ;;
    *)            VERSION="$arg" ;;
  esac
done
VERSION="${VERSION:-$(cat VERSION 2>/dev/null || echo dev)}"
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

# Optional: publish the public binaries to a GitHub release (only with --publish).
if [ "$PUBLISH" = "1" ]; then
  command -v gh >/dev/null 2>&1 || { echo "gh (GitHub CLI) not found — cannot publish" >&2; exit 1; }
  tag="v$VERSION"
  branch="$(git rev-parse --abbrev-ref HEAD)"
  echo "publishing $tag to GitHub from $branch…"
  git push origin "$branch" # the tag points at this branch's tip on the remote
  shopt -s nullglob
  assets=("$DIST/nib-$VERSION"-* "$DIST/nib_${VERSION}_"*.deb)
  shopt -u nullglob
  if gh release view "$tag" >/dev/null 2>&1; then
    gh release upload "$tag" "${assets[@]}" --clobber
  else
    gh release create "$tag" "${assets[@]}" \
      --target "$branch" --title "Nib $VERSION" --notes "Nib $VERSION"
  fi
  echo "published $tag"
fi
