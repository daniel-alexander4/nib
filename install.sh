#!/usr/bin/env bash
# Build Nib for this machine, package it as a .deb, and install it — giving a
# menu item under Office.
#
# It used to say the menu item "kills any running instance and starts anew". That was true
# until P07 deleted internal/singleton, which did the killing; the .desktop entry is a
# plain `Exec=nib %f`, and a second launch now hands its document to the running instance
# (cmd/nib/main.go says so in terms). A false sentence in a script users run.
#
# Usage: ./install.sh [version]   (version defaults to the VERSION file)
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${1:-$(cat VERSION 2>/dev/null || echo 0.0.0)}"
ARCH="$(dpkg --print-architecture)" # amd64 or arm64
DEB="dist/nib_${VERSION}_${ARCH}.deb"
mkdir -p dist

echo "building nib $VERSION ($ARCH)…"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o dist/nib ./cmd/nib

if ! command -v nfpm >/dev/null 2>&1; then
  echo "installing nfpm…"
  # Pinned, not @latest: an unpinned build tool fetched mid-install could drift
  # or be compromised between runs. Bump deliberately when upgrading nfpm.
  go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.46.3
  export PATH="$PATH:$(go env GOPATH)/bin"
fi

echo "packaging $DEB…"
ARCH="$ARCH" VERSION="$VERSION" \
  nfpm package --config build/nfpm.yaml --packager deb --target "$DEB"

echo "installing $DEB (sudo)…"
# dpkg -i (not apt install ./file): no _apt sandbox warning, and it allows
# reinstalling the same version or downgrading. Nib has no dependencies.
sudo dpkg -i "$DEB"

# Refresh the desktop/icon caches so the menu item appears without a re-login.
sudo update-desktop-database 2>/dev/null || true
sudo gtk-update-icon-cache -f /usr/share/icons/hicolor 2>/dev/null || true

rm -f dist/nib
echo
echo "Installed. Look for Nib in your applications menu under Office."
