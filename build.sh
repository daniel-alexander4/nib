#!/usr/bin/env bash
# Cross-compile Nib for all platforms and build Linux .deb/.rpm/AppImage packages.
# Usage: ./build.sh [version] [--publish] [--skip-appimage]   (version defaults to the VERSION file)
#
# Produces a single cgo-free static binary per OS/arch in dist/, plus a .deb,
# a .rpm, and an .AppImage for linux amd64/arm64 when nfpm (deb/rpm) and
# appimagetool (AppImage) are installed:
#   go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.46.3 — the version install.sh pins.
#
# With --publish it also pushes the current branch and uploads the public
# binaries to a GitHub release tagged v<version> (needs the gh CLI). build.sh
# only ever builds the public `nib` binaries — never the embed-keys nib-sig
# variant — so what it publishes is always safe to share.
set -euo pipefail
cd "$(dirname "$0")"

PUBLISH=0
SKIP_APPIMAGE=0
VERSION=""
for arg in "$@"; do
  case "$arg" in
    --publish|-p) PUBLISH=1 ;;
    --skip-appimage) SKIP_APPIMAGE=1 ;;
    *)            VERSION="$arg" ;;
  esac
done
VERSION="${VERSION:-$(cat VERSION 2>/dev/null || echo dev)}"
# Refuse a dirty tree, a version HEAD does not carry, a machine without nfpm or a signed-in gh,
# or a tag that names another commit — BEFORE anything is built or pushed.
if [ "$PUBLISH" = "1" ]; then
  ./build/publish-preflight.sh "$VERSION"
fi
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
    echo "packaging nib-${VERSION}-1.${arch}.rpm"
    ARCH="$arch" VERSION="$VERSION" \
      nfpm package --config build/nfpm.yaml --packager rpm --target "$DIST/"
  done
  rm -f "$DIST/nib"
else
  echo "nfpm not found — skipping .deb/.rpm. Install: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.46.3"
fi

# AppImage: one portable bundle per linux arch. Layout follows the AppImage
# spec (AppRun + <name>.desktop + icon + usr/bin/nib); appimagetool packs it.
# The binary is static/pure-Go, so no shared-library bundling is needed.
if [ "$SKIP_APPIMAGE" = "0" ]; then
  if command -v appimagetool >/dev/null 2>&1; then
    for arch in amd64 arm64; do
      case "$arch" in
        amd64) aiarch="x86_64" ;;
        arm64) aiarch="aarch64" ;;
      esac
      echo "packaging nib-${VERSION}-linux-${arch}.AppImage"
      appdir="$DIST/AppDir-$arch"
      rm -rf "$appdir"
      mkdir -p "$appdir/usr/bin"
      cp "$DIST/nib-$VERSION-linux-$arch" "$appdir/usr/bin/nib"
      chmod 0755 "$appdir/usr/bin/nib"
      cp build/nib.desktop "$appdir/nib.desktop"
      cp build/nib.svg "$appdir/nib.svg"
      # AppRun execs the bundled binary, passing through any file arguments.
      printf '#!/bin/sh\nexec "$(dirname "$0")/usr/bin/nib" "$@"\n' > "$appdir/AppRun"
      chmod 0755 "$appdir/AppRun"
      ( cd "$DIST" && ARCH="$aiarch" VERSION="$VERSION" \
        appimagetool "AppDir-$arch" "nib-${VERSION}-linux-${arch}.AppImage" )
      rm -rf "$appdir"
    done
  else
    echo "appimagetool not found — skipping .AppImage (https://appimage.github.io/appimagetool/)"
  fi
else
  echo "skipping .AppImage (--skip-appimage)"
fi

# SHA256SUMS: one line per binary and package, so a user (or a program that pins a minimum version) can check
# the file it downloaded is the one this build made. The licence and notices are not signed by it.
# ("nib-$VERSION"-* already covers the raw binaries, the .rpm, and the .AppImage; the second
# pattern adds the .deb, whose name puts the version after an underscore.)
( cd "$DIST" && shopt -s nullglob && files=("nib-$VERSION"-* "nib_${VERSION}_"*.deb) \
  && if command -v sha256sum >/dev/null 2>&1; then sha256sum "${files[@]}"; else shasum -a 256 "${files[@]}"; fi > SHA256SUMS )

echo "done — artifacts in $DIST/"

# Optional: publish the public binaries to a GitHub release (only with --publish).
if [ "$PUBLISH" = "1" ]; then
  command -v gh >/dev/null 2>&1 || { echo "gh (GitHub CLI) not found — cannot publish" >&2; exit 1; }
  tag="v$VERSION"
  branch="$(git rev-parse --abbrev-ref HEAD)"
  echo "publishing $tag to GitHub from $branch…"
  git push origin "$branch" # the tag points at this branch's tip on the remote
  shopt -s nullglob
  # Ship the AGPL licence + third-party attribution alongside the binaries so the
  # bare (non-.deb/.rpm) downloads carry their required notices too.
  assets=("$DIST/nib-$VERSION"-* "$DIST/nib_${VERSION}_"*.deb "$DIST/nib-${VERSION}"*.rpm "$DIST/SHA256SUMS" LICENSE THIRD-PARTY-NOTICES.md)
  shopt -u nullglob
  # Upload, publish a release found as a draft, and report what releases/latest answers (/pending 868).
  ./build/publish-release.sh "$VERSION" "$branch" "${assets[@]}"
fi
