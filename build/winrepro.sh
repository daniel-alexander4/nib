#!/usr/bin/env bash
# Check Nib's Windows-specific behaviour by running the real Windows binary.
#
# Usage: ./build/winrepro.sh [--keep]
#
# Nib is cross-compiled for Windows but developed and tested on Linux, so the
# places where `path/filepath` answers differently — no single filesystem root,
# `\` as the separator, drive letters, and errnos that don't match POSIX — are
# exactly the places a Linux-only test suite cannot reach. Reading the toolchain
# source is not enough: the v1.101.0 work found a classification bug that passed
# on Linux (listing a file fails ENOTDIR there, ENOENT on Windows) and only this
# harness caught it.
#
# So: build nib.exe, run it headless under wine in a throwaway prefix, and drive
# the same HTTP calls the browser UI makes. Requires wine; skips cleanly without
# it, the same way the poppler/Ghostscript/veraPDF tests do.
#
# --keep leaves the wine prefix and the server running for poking at by hand.
set -uo pipefail
cd "$(dirname "$0")/.."

command -v wine >/dev/null 2>&1 || { echo "wine not installed; skipping the Windows checks"; exit 0; }

KEEP=0
[ "${1:-}" = "--keep" ] && KEEP=1

PORT="${NIB_WINE_PORT:-8765}"
BASE="http://127.0.0.1:$PORT"
WORK="$(mktemp -d)"
export WINEPREFIX="$WORK/wine"
export WINEDEBUG="-all"
EXE="$WORK/nib.exe"
USERNAME_="$(id -un)"
FAILED=0

cleanup() {
  # The pattern is bracketed so it can't match this script's own command line.
  pkill -f "$(printf 'ni[b]%s.exe' '')" >/dev/null 2>&1
  [ "$KEEP" = "1" ] || rm -rf "$WORK"
}
[ "$KEEP" = "1" ] || trap cleanup EXIT

# check NAME HAYSTACK NEEDLE — assert NEEDLE appears in HAYSTACK.
check() {
  if printf '%s' "$2" | grep -qF -- "$3"; then
    echo "  ok   $1"
  else
    echo "  FAIL $1"
    echo "         want substring: $3"
    echo "         got:            $2"
    FAILED=1
  fi
}

# checknot NAME HAYSTACK NEEDLE — assert NEEDLE does NOT appear.
checknot() {
  if printf '%s' "$2" | grep -qF -- "$3"; then
    echo "  FAIL $1"
    echo "         unwanted substring present: $3"
    echo "         got: $2"
    FAILED=1
  else
    echo "  ok   $1"
  fi
}

listdir() { curl -s -G "$BASE/api/listdir" --data-urlencode "path=$1"; }

# The literal text a native Windows path takes inside a JSON body, where every
# separator is doubled. Assertions match against this rather than a bare suffix:
# checking only for "\nibprobe" would pass on an UNexpanded "~\nibprobe", which is
# precisely the bug the tilde check exists to catch.
jpath() { printf 'C:\\\\users\\\\%s\\\\%s' "$USERNAME_" "$1"; }

echo "building nib.exe"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$EXE" ./cmd/nib || exit 1

echo "initializing wine prefix (first run takes a moment)"
wineboot -u >/dev/null 2>&1

# Seed a folder directly under the Windows home. Wine symlinks the shell folders
# (Documents, Desktop) out to the real Unix home, so a fresh subdirectory is the
# only one guaranteed to hold just our fixtures.
WHOME="$WINEPREFIX/drive_c/users/$USERNAME_"
mkdir -p "$WHOME/nibprobe"
cp .playwright-mcp/shots/docA.pdf "$WHOME/nibprobe/report.pdf" 2>/dev/null \
  || printf '%%PDF-1.7\n' > "$WHOME/nibprobe/report.pdf"
printf 'not a pdf\n' > "$WHOME/nibprobe/notes.txt"

echo "starting nib.exe headless on $BASE"
NIB_ADDR="127.0.0.1:$PORT" NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 wine "$EXE" > "$WORK/nib.log" 2>&1 &
for _ in $(seq 1 40); do
  curl -s --max-time 2 "$BASE/api/status" >/dev/null 2>&1 && break
  sleep 1
done
curl -s --max-time 2 "$BASE/api/status" >/dev/null 2>&1 || {
  echo "nib.exe never came up; log follows"; cat "$WORK/nib.log"; exit 1
}

# First run: create a key, which creates and unlocks the vault. Every route the
# dialogs use sits behind requireUnlocked.
curl -s -X POST "$BASE/api/ssh/enroll" -H 'Content-Type: application/json' -d '{"mode":"create"}' >/dev/null
CSRF="$(curl -s "$BASE/api/status" | sed -n 's/.*"csrf":"\([^"]*\)".*/\1/p')"
[ -n "$CSRF" ] || { echo "vault did not unlock; log follows"; cat "$WORK/nib.log"; exit 1; }

echo
echo "drive enumeration — the parent walk dead-ends at a drive root on Windows"
ROOT_LIST="$(listdir 'C:\')"
check "C:\\ reports no parent"            "$ROOT_LIST" '"parent":""'
# Matched inside the roots array specifically — a bare 'C:\\' also appears in the
# "path" field, so the looser form passes even when no drives are offered at all.
check "C:\\ offers the drives instead"    "$ROOT_LIST" '"roots":["C:\\"'

echo
echo "empty vs unreadable — a folder that can't be listed must say why"
check "absent folder -> missing"  "$(listdir 'C:\users\'"$USERNAME_"'\nope')"           '"reason":"missing"'
check "a file -> notdir"          "$(listdir 'C:\users\'"$USERNAME_"'\nibprobe\notes.txt')" '"reason":"notdir"'
checknot "a real folder is quiet" "$(listdir 'C:\users\'"$USERNAME_"'\nibprobe')"       '"reason"'
# The save dialog asks for ~/nib before it exists by sending no path at all;
# that one case is expected and must stay silent.
checknot "default folder is quiet" "$(curl -s "$BASE/api/listdir")" '"reason"'

echo
echo "tilde — a Windows user types the separator the OS uses"
BACK="$(listdir '~\nibprobe')"
FWD="$(listdir '~/nibprobe')"
check "~\\ expands to the home dir"  "$BACK" "\"path\":\"$(jpath nibprobe)\""
check "~/ expands identically"       "$FWD"  "\"path\":\"$(jpath nibprobe)\""
check "~\\ finds the PDF"            "$BACK" 'report.pdf'

echo
echo "server-built child paths — the browser never joins a path itself"
check "children carry a native path" "$BACK" "$(jpath 'nibprobe\\report.pdf')"

echo
echo "open by path, including the mixed separators a browsed path used to produce"
OPENED="$(curl -s -X POST "$BASE/api/open" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d '{"path":"C:\\users\\'"$USERNAME_"'/nibprobe/report.pdf"}')"
check "mixed separators still open" "$OPENED" '"name":"report.pdf"'

echo
echo "recent — the display name is the server's job"
check "recent carries a basename" "$(curl -s "$BASE/api/recent")" '"name":"report.pdf"'

echo
echo "save-as containment — a typed name must not escape the chosen folder"
printf 'payload' > "$WORK/payload.bin"
# The body is asserted, not just the status: a build that doesn't understand the
# dir+name fields at all also answers 400, so a status-only check would pass
# against code that has no containment guard whatsoever.
ESCAPE_BODY="$(curl -s -X POST "$BASE/api/write" -H "X-CSRF-Token: $CSRF" \
  -F 'dir=C:\users\'"$USERNAME_"'\nibprobe' -F 'name=../../escaped.pdf' -F "data=@$WORK/payload.bin")"
check "../ name is refused as unsafe" "$ESCAPE_BODY" 'unsafe file name'
if [ -e "$WINEPREFIX/drive_c/users/escaped.pdf" ]; then
  echo "  FAIL the name escaped the chosen folder"; FAILED=1
else
  echo "  ok   nothing written outside the folder"
fi
OK_CODE="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/write" -H "X-CSRF-Token: $CSRF" \
  -F 'dir=C:\users\'"$USERNAME_"'\nibprobe' -F 'name=saved.pdf' -F "data=@$WORK/payload.bin")"
check "an ordinary save still works" "$OK_CODE" '200'

echo
if [ "$FAILED" = "0" ]; then
  echo "all Windows checks passed"
else
  echo "SOME WINDOWS CHECKS FAILED (see FAIL lines above)"
fi
[ "$KEEP" = "1" ] && echo "kept: prefix $WINEPREFIX, server on $BASE, log $WORK/nib.log"
exit "$FAILED"
