#!/usr/bin/env bash
# Nib one-liner uninstaller.
#
#   curl -fsSL https://raw.githubusercontent.com/daniel-alexander4/nib/main/scripts/uninstall.sh | bash
#
# Removes whatever install.sh (or the .deb/.rpm/AUR package) put on this machine:
# binary/AppImage, desktop entry, icon. Package-manager entries are removed with
# the matching tool when present (dpkg/rpm/pacman); the raw-binary files are
# always cleaned up. Never touches your vault, documents, or ~/nib peer inbox.
set -euo pipefail

removed=0
remove_file() { # $1 = path
  if [ -e "$1" ]; then rm -f "$1"; echo "removed $1"; removed=1; fi
}

# 1) Native packages first (need root/sudo; ignore failures — file cleanup below covers it).
if command -v dpkg >/dev/null 2>&1 && dpkg -s nib >/dev/null 2>&1; then
  if [ "$(id -u)" = "0" ]; then dpkg -r nib || true; else sudo dpkg -r nib || true; fi
  removed=1
fi
if command -v rpm >/dev/null 2>&1 && rpm -q nib >/dev/null 2>&1; then
  if [ "$(id -u)" = "0" ]; then rpm -e nib || true; else sudo rpm -e nib || true; fi
  removed=1
fi
for mgr in yay paru pacman; do
  if command -v "$mgr" >/dev/null 2>&1 && pacman -Q nib nib-bin nib-git 2>/dev/null | grep -q .; then
    echo "removing AUR package(s) via $mgr… (run '$mgr -R nib nib-bin nib-git' if this prompts)"
    if [ "$(id -u)" = "0" ]; then "$mgr" -R --noconfirm nib nib-bin nib-git 2>/dev/null || true
    else sudo "$mgr" -R nib nib-bin nib-git 2>/dev/null || "$mgr" -R nib nib-bin nib-git 2>/dev/null || true; fi
    removed=1
    break
  fi
done

# 2) Raw files: system-wide + user-local locations install.sh uses.
for bin in /usr/local/bin/nib /usr/bin/nib "$HOME/.local/bin/nib" "$HOME/.local/bin/nib.AppImage" /usr/local/bin/nib.AppImage; do
  remove_file "$bin"
done
for desk in /usr/share/applications/nib.desktop "$HOME/.local/share/applications/nib.desktop"; do
  remove_file "$desk"
done
for icon in /usr/share/icons/hicolor/scalable/apps/nib.svg "$HOME/.local/share/icons/hicolor/scalable/apps/nib.svg"; do
  remove_file "$icon"
done

command -v update-desktop-database >/dev/null 2>&1 && { update-desktop-database /usr/share/applications 2>/dev/null || true; update-desktop-database "$HOME/.local/share/applications" 2>/dev/null || true; }

if [ "$removed" = "0" ]; then echo "nib does not appear to be installed."; else echo "done — your vault, documents, and ~/nib were left alone."; fi
