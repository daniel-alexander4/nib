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
# ── What this harness CANNOT discharge ───────────────────────────────────────
# **Link-local discovery.** wine models neither multicast group membership nor
# interface enumeration, and the two Windows divergences Nib's discovery code has
# to survive are precisely there:
#
#   - x/net's SetControlMessage is unimplemented on Windows (control_windows.go is
#     a TODO returning errNotImplemented), so a received control message is nil
#     with a NIL ERROR and any filter written on the arrival interface silently
#     accepts everything;
#   - an IPv4 group join resolves the interface to an ADDRESS rather than an index
#     (setIPv4MreqToInterface), so an interface whose IPv4 lease has not arrived is
#     joinable on Linux and refused on Windows.
#
# A green run of THIS script says nothing about either. P03's exit criterion says
# so in as many words — "a green winrepro may not discharge this bullet" — and the
# thing that does is `nib discover` on a real Windows machine, which prints the
# interface selection, whether announcements left the host, and whether the
# platform supplied an arrival interface at all.
#
# Nothing here should ever grow a discovery check: it would be a green nobody
# could act on.

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

SERVER_PID=""

cleanup() {
  # This run's processes only. The bracketed `pkill -f ni[b].exe` this replaces did
  # protect against matching the script's own command line, and that is all it
  # protected against: it still killed EVERY nib.exe on the machine, so a developer
  # poking at their own `wine nib.exe` lost it, and two harness runs killed each
  # other.
  #
  # Both halves are needed. $! is the `wine` wrapper, not the Windows process —
  # that one is a wineserver-managed child — so the pid alone is not guaranteed to
  # take it down. The pkill is scoped to $WORK, a mktemp path unique to this run,
  # which is what makes it safe where matching on the exe name was not.
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" >/dev/null 2>&1
  pkill -f "$WORK" >/dev/null 2>&1
  [ "$KEEP" = "1" ] || rm -rf "$WORK"
}
# Trapped unconditionally, with --keep handled INSIDE cleanup — the same shape
# uirepro.sh uses. Installing the trap only when KEEP != 1 (as this did) meant the
# work dir and the wine prefix survived an interrupt on an ordinary run, and it
# trapped only EXIT where uirepro traps EXIT INT TERM.
trap cleanup EXIT INT TERM

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

# Run the whole harness against a HOSTILE MIME registry. Go's mime package seeds
# itself from HKEY_CLASSES_ROOT on Windows, so a stray per-machine entry decides
# the Content-Type of the assets Nib ships inside its own binary. A real user hit
# exactly this: ".mjs" served as text/plain, Chromium refused the UI's modules
# under strict MIME checking, and the app window came up with no dialog and no
# button that worked. Go's own workaround covers ".js" and nothing else.
#
# Setting these before nib starts (mime reads the registry once, at init) means
# every check below runs under the condition that broke it.
wine reg add 'HKCR\.mjs' /v "Content Type" /t REG_SZ /d "text/plain" /f >/dev/null 2>&1
wine reg add 'HKCR\.js'  /v "Content Type" /t REG_SZ /d "text/plain" /f >/dev/null 2>&1

# Seed a folder directly under the Windows home. Wine symlinks the shell folders
# (Documents, Desktop) out to the real Unix home, so a fresh subdirectory is the
# only one guaranteed to hold just our fixtures.
WHOME="$WINEPREFIX/drive_c/users/$USERNAME_"
mkdir -p "$WHOME/nibprobe"
# Fixtures are GENERATED, not copied. This used to be
#   cp .playwright-mcp/shots/docA.pdf … || printf '%%PDF-1.7\n' > …
# whose source is a gitignored scratch artifact — so on every machine but the one
# that captured it the fallback fired and the "PDF" was a nine-byte header. Every
# check below passed against it, because LooksLikePDF only wants the header and a
# headless run renders nothing; the harness was silent about whether Nib can open a
# real document on Windows while reading as though it had said so. See build/genpdf.go.
go run build/genpdf.go "$WHOME/nibprobe/report.pdf" "report page 1" "report page 2" || exit 1
# A SECOND document, for the hand-off checks: D16 makes an already-open path focus
# rather than duplicate, so a second launch carrying report.pdf could not move the
# document count no matter how well the hand-off worked.
go run build/genpdf.go "$WHOME/nibprobe/second.pdf" "second page 1" || exit 1
printf 'not a pdf\n' > "$WHOME/nibprobe/notes.txt"

echo "starting nib.exe headless on $BASE"
NIB_ADDR="127.0.0.1:$PORT" NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 wine "$EXE" > "$WORK/nib.log" 2>&1 &
SERVER_PID=$!
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
echo "asset MIME types — the UI is an ES module; a non-JS type stops it executing"
ctype() { curl -s -o /dev/null -D - "$BASE/$1" | tr -d '\r' | sed -n 's/^[Cc]ontent-[Tt]ype: //p'; }
check "app.js is served as JavaScript"          "$(ctype 'app.js')"                          'text/javascript'
check ".mjs is served as JavaScript"            "$(ctype 'vendor/pdfjs/pdf.min.mjs')"        'text/javascript'
check "the other .mjs modules too"              "$(ctype 'vendor/pixelmatch/pixelmatch.mjs')" 'text/javascript'
check "the tesseract core (.js) too"            "$(ctype 'vendor/tesseract/tesseract-core-simd.wasm.js')" 'text/javascript'
check "stylesheet is served as CSS"             "$(ctype 'style.css')"                       'text/css'

echo
echo "key paths — a relative one is anchored to however the app was started"
# filepath.IsAbs answers differently per OS, so the Windows shapes can only be
# checked here: C:\... is absolute, a bare name and a drive-relative \Users\...
# are not. A key stored at a non-absolute path is not found by the next launch.
# Driven through /api/ssh/keys rather than /api/ssh/enroll: the vault is already
# set up by this point, so enroll would answer 409 before ever reaching the guard.
# This is the second entry point anyway — the one a fix to the first-run wizard
# alone would have missed.
keyadd() { curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/ssh/keys" \
  -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d "{\"mode\":\"create\",\"keyPath\":\"$1\"}"; }
check "a bare relative name is refused"     "$(keyadd 'id_ed25519')"          '400'
check "a drive-relative path is refused"    "$(keyadd '\\\\Users\\\\me\\\\k')" '400'
# Where a bare relative name would ACTUALLY land, which is not where this used to
# look. The script cd's to the repo root, so `wine "$EXE"` inherits that as its
# working directory and a Windows-side relative path resolves against it — not
# against $WORK, which only ever holds nib.exe, wine/, nib.log and payload.bin.
# Scanning $WORK therefore could not fail whatever the guard under test did, and a
# regression would have dropped a private key into the developer's working tree
# while this printed ok. Both plausible landing sites are checked, and the wine
# drive_c home as well, so the assertion covers where the file goes rather than
# where the exe happens to sit.
checknot "and no key was written to the working tree" "$(ls . 2>/dev/null)"        'id_ed25519'
checknot "nor into the wine home"                     "$(ls -R "$WHOME" 2>/dev/null)" 'id_ed25519'

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
echo "second launch — hands the document to the running instance instead of killing it"
# P07's Windows criterion, and until this section the harness could not speak to it:
# winrepro started exactly ONE nib.exe, so "a second launch hands off" was asserted by
# Go tests and by a Linux browser drive, on the one platform where double-click is the
# ORDINARY way in (`nib register`) and where the mechanism it replaces never worked at
# all — `internal/singleton` walked /proc, and its Windows build was `return 0`.
#
# The instrument is the FIRST instance's document list. A hand-off that worked moves it;
# a replace-and-kill would empty it; a second instance that simply served alongside would
# not touch it at all. Those three outcomes are distinguishable here and nowhere else in
# this file.
docs() { curl -s --max-time 5 "$BASE/api/docs"; }
ndocs() { printf '%s' "$1" | grep -o '"id":"' | wc -l | tr -d ' '; }

BEFORE="$(docs)"
# The stimulus: the first instance must be holding report.pdf already (the open check
# above put it there). Without this, "the count went up" could be counting from a route
# that answered nothing, and "report.pdf survived" could be true of an empty list.
check "the running instance holds the opened document" "$BEFORE" '"name":"report.pdf"'
N_BEFORE="$(ndocs "$BEFORE")"
[ "$N_BEFORE" -ge 1 ] || { echo "  FAIL /api/docs reported $N_BEFORE documents; the hand-off checks below cannot mean anything"; FAILED=1; }

# No NIB_ADDR on the second launch, deliberately: if the hand-off fails, this process
# falls through to binding a port, and pinning it to the first instance's would turn a
# hand-off failure into a bind failure — the wrong error, and one that looks like this.
timeout 90 env NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 \
  wine "$EXE" 'C:\users\'"$USERNAME_"'\nibprobe\second.pdf' > "$WORK/second.log" 2>&1
SECOND_RC=$?
if [ "$SECOND_RC" = "0" ]; then
  echo "  ok   the second launch exited instead of serving"
else
  echo "  FAIL the second launch exited $SECOND_RC (124 = it never exited, so it became a second server)"
  sed -n '1,20p' "$WORK/second.log"
  FAILED=1
fi
checknot "and it never bound a port of its own" "$(cat "$WORK/second.log")" 'serving at'
# A refusal or a queue also ends in a clean exit, so the exit status alone does not say
# the document arrived. The launch logs both, and neither may appear here: the instance
# is unlocked and has room.
checknot "and the hand-off was neither refused nor queued" "$(cat "$WORK/second.log")" 'refused this document'
checknot "nor deferred to an unlock"                       "$(cat "$WORK/second.log")" 'Nib is locked'

AFTER="$(docs)"
N_AFTER="$(ndocs "$AFTER")"
if [ "$N_AFTER" -gt "$N_BEFORE" ]; then
  echo "  ok   the first instance's document count moved ($N_BEFORE -> $N_AFTER)"
else
  echo "  FAIL the first instance still holds $N_AFTER documents; the hand-off did not arrive"
  FAILED=1
fi
check "the handed-off document is open in the FIRST instance" "$AFTER" '"name":"second.pdf"'
# The retirement itself. Note what this one does and does not falsify: it fails against
# replace-and-kill, which would have taken the first instance down and left a survivor
# knowing nothing of report.pdf — and it passes against a launch that never handed off at
# all, because that leaves the first instance equally untouched. The count check above is
# the row that catches THAT, which is why both are here.
check "and the document that was already open survived" "$AFTER" '"name":"report.pdf"'

# D16 — an already-open path is focused, not opened twice. Checked here rather than only
# in Go because the comparison is between PATHS, and a path is the thing this platform
# spells differently.
#
# **Its stimulus is asserted before its result**, and that is not ceremony: the first
# draft compared counts alone, so when the hand-off was disabled entirely — the red-proof
# for this whole section — the count did not move and this row reported ok. A check that
# is green when nothing happened is measuring nothing.
timeout 90 env NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 \
  wine "$EXE" 'C:\users\'"$USERNAME_"'/nibprobe/report.pdf' > "$WORK/third.log" 2>&1
THIRD_RC=$?
N_THIRD="$(ndocs "$(docs)")"
if [ "$THIRD_RC" != "0" ] || grep -qF 'serving at' "$WORK/third.log"; then
  echo "  FAIL the already-open launch never reached the running instance (exit $THIRD_RC), so its count check would pass over a hand-off that did not happen"
  FAILED=1
elif grep -qF 'refused this document' "$WORK/third.log"; then
  # `focused` and `refused` are indistinguishable by count — both leave it where it was —
  # so without this the check would report ok for a hand-off the running instance turned
  # down. The launch logs a refusal, and that is the only place the difference surfaces.
  echo "  FAIL the running instance REFUSED the already-open path; the count did not move because nothing was opened, not because it was focused"
  sed -n '1,10p' "$WORK/third.log"
  FAILED=1
elif [ "$N_THIRD" = "$N_AFTER" ]; then
  echo "  ok   handing over an already-open path focused it rather than duplicating it"
else
  echo "  FAIL the count went $N_AFTER -> $N_THIRD; an already-open path opened a second copy"
  FAILED=1
fi

echo
if [ "$FAILED" = "0" ]; then
  echo "all Windows checks passed"
else
  echo "SOME WINDOWS CHECKS FAILED (see FAIL lines above)"
fi
[ "$KEEP" = "1" ] && echo "kept: prefix $WINEPREFIX, server on $BASE, log $WORK/nib.log"
exit "$FAILED"
