#!/usr/bin/env bash
# Nib one-liner installer.
#
#   curl -fsSL https://raw.githubusercontent.com/daniel-alexander4/nib/main/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/daniel-alexander4/nib/main/scripts/install.sh | bash -s -- --appimage
#   curl -fsSL https://raw.githubusercontent.com/daniel-alexander4/nib/main/scripts/install.sh | bash -s -- --version 2.50.4 --binary
#
# Installs the latest Nib release (or --version X.Y.Z) for your OS/arch:
#   - Debian/Ubuntu (dpkg present): .deb via dpkg (needs sudo/root)
#   - Fedora/RHEL/openSUSE (rpm present): .rpm via rpm (needs sudo/root)
#   - Arch Linux: tries the AUR (yay/paru/pacman) when available, else falls back to the raw binary
#   - --appimage: installs the portable .AppImage to ~/.local/bin (or /usr/local/bin as root)
#   - otherwise: raw binary + desktop entry + icon
#
# Every download is verified against the release SHA256SUMS before anything is installed.
# Env overrides: NIB_REPO=owner/repo (default daniel-alexander4/nib), NIB_VERSION=X.Y.Z, NIB_BINDIR=dir.
set -euo pipefail

REPO="${NIB_REPO:-daniel-alexander4/nib}"
VERSION="${NIB_VERSION:-latest}"
MODE="auto"          # auto | deb | rpm | appimage | binary
BINDIR="${NIB_BINDIR:-}"
VERIFY=1

usage() {
  cat <<'EOF'
Usage: install.sh [--version X.Y.Z] [--deb|--rpm|--appimage|--binary] [--bindir DIR] [--no-verify] [--help]

  --version X.Y.Z   install this version instead of the latest release
  --deb             install the .deb package (Debian/Ubuntu)
  --rpm             install the .rpm package (Fedora/RHEL/openSUSE)
  --appimage        install the portable .AppImage
  --binary          install the raw binary + desktop entry
  --bindir DIR      where to put the binary/AppImage (default: /usr/local/bin as root, ~/.local/bin otherwise)
  --no-verify       skip SHA256SUMS verification (not recommended)
  --help            show this help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:?}"; shift 2 ;;
    --deb) MODE="deb"; shift ;;
    --rpm) MODE="rpm"; shift ;;
    --appimage) MODE="appimage"; shift ;;
    --binary) MODE="binary"; shift ;;
    --bindir) BINDIR="${2:?}"; shift 2 ;;
    --no-verify) VERIFY=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "install.sh: unknown flag $1 (see --help)" >&2; exit 1 ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "install.sh: need '$1' on PATH" >&2; exit 1; }; }
need curl; need sha256sum || need shasum

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
machine="$(uname -m)"
case "$machine" in
  x86_64|amd64) arch="amd64"; aiarch="x86_64" ;;
  aarch64|arm64) arch="arm64"; aiarch="aarch64" ;;
  *) echo "install.sh: unsupported arch $machine" >&2; exit 1 ;;
esac
case "$os" in
  linux) platform="linux" ;;
  darwin) platform="darwin" ;;
  *) echo "install.sh: unsupported OS $os (Windows: run the nib-*-windows-*.exe from the releases page)" >&2; exit 1 ;;
esac

# Resolve the version: query GitHub for the latest release tag (vX.Y.Z -> X.Y.Z).
# NOTE: grep -m1 closes the pipe early, which makes curl exit 23 under
# `set -o pipefail` — so the API body is fetched first, then parsed.
if [ "$VERSION" = "latest" ]; then
  echo "resolving latest release for $REPO…" >&2
  api_body="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest")"
  tag="$(printf '%s' "$api_body" | grep -m1 '"tag_name"' | sed 's/.*"v\?\([^"]*\)".*/\1/')"
  [ -n "${tag:-}" ] || { echo "install.sh: could not resolve latest release" >&2; exit 1; }
  VERSION="$tag"
fi
VERSION="${VERSION#v}"
echo "installing nib $VERSION ($platform/$arch)…" >&2

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"

base="https://github.com/$REPO/releases/download/v$VERSION"
sums="SHA256SUMS"
curl -fsSL -o "$sums" "$base/$sums"

fetch_verify() { # $1 = filename
  curl -fsSL -o "$1" "$base/$1"
  if [ "$VERIFY" = "1" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      grep -F " $1" "$sums" | sha256sum -c -
    else
      grep -F " $1" "$sums" | shasum -a 256 -c -
    fi
  fi
}

is_root() { [ "$(id -u)" = "0" ]; }
have() { command -v "$1" >/dev/null 2>&1; }

# Default bindir: system-wide as root, user-local otherwise.
if [ -z "$BINDIR" ]; then
  if is_root; then BINDIR="/usr/local/bin"; else BINDIR="$HOME/.local/bin"; fi
fi
datadir=""; icondir=""
if is_root; then
  datadir="/usr/share/applications"; icondir="/usr/share/icons/hicolor/scalable/apps"
else
  datadir="$HOME/.local/share/applications"; icondir="$HOME/.local/share/icons/hicolor/scalable/apps"
fi

install_desktop_files() { # $1 = Exec path
  mkdir -p "$datadir" "$icondir"
  curl -fsSL -o "$datadir/nib.desktop" "https://raw.githubusercontent.com/$REPO/main/build/nib.desktop"
  curl -fsSL -o "$icondir/nib.svg" "https://raw.githubusercontent.com/$REPO/main/build/nib.svg"
  sed -i "s|^Exec=nib|Exec=$1|" "$datadir/nib.desktop"
  command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$datadir" 2>/dev/null || true
  command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -f "$(dirname "$icondir")/.." 2>/dev/null || true
}

install_binary_asset() { # $1 = asset filename
  fetch_verify "$1"
  chmod +x "$1"
  if "$1" version >/dev/null 2>&1 || ./"$1" version >/dev/null 2>&1; then :; else echo "install.sh: downloaded binary failed to run" >&2; exit 1; fi
  mkdir -p "$BINDIR"
  cp "$1" "$BINDIR/nib"
  chmod +x "$BINDIR/nib"
  install_desktop_files "$BINDIR/nib"
  echo "installed $( "$BINDIR/nib" version ) to $BINDIR/nib"
}

# Auto mode: prefer a native package when the toolchain is there, else binary.
if [ "$MODE" = "auto" ]; then
  if [ "$platform" = "linux" ] && have dpkg && (is_root || have sudo); then MODE="deb"
  elif [ "$platform" = "linux" ] && have rpm && (is_root || have sudo); then MODE="rpm"
  elif [ "$platform" = "linux" ] && (have yay || have paru || have pacman); then
    echo "Arch detected: prefer the AUR — 'yay -S nib' (source), 'yay -S nib-bin' (binary), 'yay -S nib-git' (dev)." >&2
    echo "Falling back to the raw binary below; pass --appimage for the portable bundle." >&2
    MODE="binary"
  else MODE="binary"
  fi
fi

case "$MODE" in
  deb)
    [ "$platform" = "linux" ] || { echo "install.sh: --deb is Linux-only" >&2; exit 1; }
    asset="nib_${VERSION}_${arch}.deb"
    fetch_verify "$asset"
    if is_root; then dpkg -i "$asset"; else sudo dpkg -i "$asset"; fi
    ;;
  rpm)
    [ "$platform" = "linux" ] || { echo "install.sh: --rpm is Linux-only" >&2; exit 1; }
    # nfpm names it nib-<version>-1.<rpmarch>.rpm (x86_64/aarch64 — not amd64/arm64).
    case "$arch" in
      amd64) rpmarch="x86_64" ;;
      arm64) rpmarch="aarch64" ;;
    esac
    asset="$(grep -o "nib-${VERSION}[^ ]*\\.${rpmarch}\\.rpm" "$sums" | head -n 1 || true)"
    [ -n "${asset:-}" ] || asset="nib-${VERSION}-1.${rpmarch}.rpm"
    fetch_verify "$asset"
    if is_root; then rpm -Uvh "$asset"; else sudo rpm -Uvh "$asset"; fi
    ;;
  appimage)
    [ "$platform" = "linux" ] || { echo "install.sh: --appimage is Linux-only" >&2; exit 1; }
    asset="nib-${VERSION}-linux-${arch}.AppImage"
    fetch_verify "$asset"
    chmod +x "$asset"
    mkdir -p "$BINDIR"
    cp "$asset" "$BINDIR/nib.AppImage"
    install_desktop_files "$BINDIR/nib.AppImage"
    echo "installed $BINDIR/nib.AppImage (chmod +x; runs anywhere, no root needed)"
    ;;
  binary)
    if [ "$platform" = "darwin" ]; then asset="nib-$VERSION-darwin-$arch"
    else asset="nib-$VERSION-linux-$arch"; fi
    install_binary_asset "$asset"
    ;;
  *) echo "install.sh: unknown mode $MODE" >&2; exit 1 ;;
esac

echo "done — run 'nib version' to verify."
