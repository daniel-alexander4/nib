#!/usr/bin/env bash
# Tier 3 of Nib's three-tier test harness: drive the REAL nib binary in a REAL
# browser.
#
# Usage: ./build/uirepro.sh [--keep]
#
# Nib serves a loopback web UI and opens it in an installed Chromium-family
# browser with --app= (internal/browser). So the engine a user actually runs is
# Chromium, and this tier drives that same engine — a UI harness that avoided a
# browser would be testing something nobody runs.
#
# ── What this tier reaches that tier 2 cannot ────────────────────────────────
# Everything test/jsdom/boot.mjs names as its ceiling:
#   * layout — real clientWidth/getBoundingClientRect, so the fit-widest-page
#     path and anything measuring a page div is observable here;
#   * canvas — thumbnails, the redaction bake, the compare pixel map;
#   * media queries — the device-pixel-ratio heal;
#   * the real pdf.js, parsing a real PDF, not a stub;
#   * real files on disk, and the real Go server answering.
#
# ── Where it still stops ─────────────────────────────────────────────────────
# Chromium only. That is deliberate rather than lazy: nib ships a Chromium
# app-mode window, so Chromium is what users get. The fallback path can still
# land someone in Firefox (internal/browser falls back to xdg-open), and that gap
# is tracked as its own VERIFY item on the pending list — this harness does not
# close it and should not be read as closing it.
#
# Requires node, playwright-core (npm install), and a Chromium-family browser.
# Skips cleanly without any of them — separately, because a missing driver and a
# missing browser are different fixes and one message covering both would send
# the reader at the wrong one. Same convention as build/winrepro.sh and the
# poppler/Ghostscript/veraPDF tests.
#
# --keep leaves the work dir and the server running for poking at by hand.
set -uo pipefail
cd "$(dirname "$0")/.."

command -v node >/dev/null 2>&1 || { echo "node not installed; skipping the browser UI tests"; exit 0; }
[ -d node_modules/playwright-core ] || { echo "playwright-core not installed (run: npm install); skipping the browser UI tests"; exit 0; }

# The candidate order is nib's own, from internal/browser.chromiumCandidates() —
# testing a browser users never get would be worse than not testing at all. It is
# duplicated here rather than derived, because nothing exposes that list outside
# Go; NIB_UI_BROWSER overrides it.
BROWSER="${NIB_UI_BROWSER:-}"
if [ -z "$BROWSER" ]; then
  for c in google-chrome google-chrome-stable chromium chromium-browser microsoft-edge brave-browser; do
    if p="$(command -v "$c" 2>/dev/null)"; then BROWSER="$p"; break; fi
  done
fi
[ -n "$BROWSER" ] || {
  echo "no Chromium-family browser found (looked for google-chrome, google-chrome-stable, chromium, chromium-browser, microsoft-edge, brave-browser); skipping the browser UI tests"
  exit 0
}

KEEP=0
[ "${1:-}" = "--keep" ] && KEEP=1

PORT="${NIB_UI_PORT:-18531}"
BASE="http://127.0.0.1:$PORT"
WORK="$(mktemp -d)"
SERVER_PID=""

cleanup() {
  # --keep means BOTH halves survive: the work dir and the running server. Killing
  # the server anyway would make the flag a half-truth — the doc comment above
  # promises a server you can still poke at, and a leftover data dir with nothing
  # serving it is not that.
  if [ "$KEEP" = "1" ]; then
    echo "--keep: nib still running at $BASE (pid $SERVER_PID), work dir $WORK"
    return
  fi
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" >/dev/null 2>&1
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

# Refuse a port someone else already holds, BEFORE building or enrolling anything.
# Without this the failure surfaces four steps later and blames the wrong thing: a
# leftover nib from an earlier --keep run keeps the port, our server fails to bind,
# curl reaches the OLD process, its vault is already enrolled, and the run dies on
# "FAIL: could not enroll a key". Every word of that points at key enrolment and
# none of it is what is wrong — and worse, the tier then reports on a binary it did
# not build.
if curl -fsS -o /dev/null --max-time 2 "$BASE/api/status" 2>/dev/null; then
  echo "FAIL: something is already serving $BASE — a leftover --keep run?" >&2
  echo "      stop it (or set NIB_UI_PORT to a free port) and re-run." >&2
  exit 1
fi

echo "building nib…"
go build -o "$WORK/nib" ./cmd/nib || { echo "FAIL: could not build nib" >&2; exit 1; }

# A throwaway HOME **and XDG_CONFIG_HOME** so the run creates its own vault and
# cannot touch the developer's real one.
#
# Setting HOME alone did not do that. The vault resolves through
# vault.DefaultDir() -> os.UserConfigDir(), which on Linux prefers
# $XDG_CONFIG_HOME and only falls back to $HOME/.config — so on any machine that
# exports it (dotfile managers, Nix, plenty of ~/.profile setups) this harness
# enrolled a key into the developer's REAL config dir, and cleanup() then removed
# the key it had sealed to. It was safe here only because the variable happens to
# be unset: environment luck, not isolation.
#
# The comment this replaces cited the Go server tests as the precedent, and that
# was the wrong precedent — those pass an explicit t.TempDir() as configDir
# (helpers_test.go) and never rely on HOME for the vault at all. HOME is still set
# because it isolates ~/.ssh, which is the other half.
mkdir -p "$WORK/home" "$WORK/config"
HOME="$WORK/home" XDG_CONFIG_HOME="$WORK/config" \
  # NIB_NO_UPDATE_CHECK=1 makes this tier HERMETIC. Without it the app calls
  # /api/update/check at boot, the server reaches out to github.com, and where that is
  # unreachable — offline, behind a proxy, on a plane — it answers 502 and the browser
  # logs "Failed to load resource: … 502". smoke.test.mjs's "boots without console errors"
  # then fails for a reason that has nothing to do with the code under test. Observed
  # exactly that way. build/winrepro.sh has always set this; the two harnesses had drifted
  # on the same concern, which is the sort of difference nobody notices until one of them
  # fails somewhere the other does not.
  NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 NIB_ADDR="127.0.0.1:$PORT" "$WORK/nib" >"$WORK/nib.log" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 60); do
  curl -fsS -o /dev/null "$BASE/api/status" 2>/dev/null && break
  sleep 0.25
done
curl -fsS -o /dev/null "$BASE/api/status" 2>/dev/null || {
  echo "FAIL: nib did not come up on $BASE" >&2; cat "$WORK/nib.log" >&2; exit 1
}

# Every document route is behind requireUnlocked, so without a vault the harness
# would only ever see the auth overlay.
curl -fsS -X POST "$BASE/api/ssh/enroll" -H 'Content-Type: application/json' \
  -d "{\"mode\":\"create\",\"keyPath\":\"$WORK/home/.ssh/id_ed25519\"}" >/dev/null || {
  echo "FAIL: could not enroll a key" >&2; exit 1
}

export NIB_UI_BASE="$BASE" NIB_UI_BROWSER="$BROWSER" NIB_UI_WORK="$WORK"
# --test-concurrency=1: the files run SERIALLY, because they share one nib process.
#
# node --test runs files in parallel by default, and this tier hands every file the same
# server — one registry, one active document. That was already unsound and was masked by
# Open REPLACING: whatever a sibling file did, the next open reset the registry to one
# document. P06.S01 makes Open add, and the masking goes with it — observed immediately,
# as a 30s waitForFunction timeout in tabs.test.mjs while lifecycle.test.mjs's Close
# (which is close-ALL server-side until P06.S02) emptied the registry underneath it, and
# as opens refused by the eight-document cap once files stopped clearing up after each
# other. Serial is the honest mode for a shared mutable server; per-file servers would be
# the alternative and cost a build and an enrol each.
out="$(node --test --test-concurrency=1 test/ui/ 2>&1)"
code=$?
echo "$out"

# Re-print the failing test names at the END, where a tailed run can still see them.
#
# node --test puts each failure inline, hundreds of lines above the summary, so
# `./build/uirepro.sh | tail` shows "# fail 1" and NOT which test — and a flake observed
# that way is a flake you cannot name. That happened on 2026-08-19 and cost the sighting:
# five clean runs afterwards could neither confirm nor rule out what the one failure was.
# The summary is the cheap fix, and it costs nothing on a green run.
if [ "$code" -ne 0 ]; then
  echo
  echo "── failed ──────────────────────────────────────────────────────────"
  printf '%s\n' "$out" | grep -E "^not ok " || echo "(no 'not ok' lines — the runner itself failed; read the log above)"
  echo "────────────────────────────────────────────────────────────────────"
fi

# Same trap tier 2 found: a runner that discovers no tests also exits 0, and would
# read as a passing suite forever.
n="$(printf '%s\n' "$out" | sed -n 's/^# tests \([0-9][0-9]*\)$/\1/p' | tail -1)"
if [ -z "$n" ] || [ "$n" -eq 0 ]; then
  echo "FAIL: the browser UI suite ran but discovered no tests — a green with nothing in it" >&2
  exit 1
fi

# The same population pin tier 2 carries, and for the same reason: a floor of one
# cannot tell 10 tests from 1, so a silently-dropped test file reads as a pass. The
# file count is the external number; a per-test literal would go red on every new
# test and train the next person to bump it.
files="$(find test/ui -maxdepth 1 -name '*.test.mjs' | wc -l | tr -d ' ')"
# Five: smoke, lifecycle and redactbounds, plus tabs.test.mjs (P06.S01, where P05's
# carried acceptance clause — re-fit and dpr-heal on activation — finally gets driven;
# both halves are about layout and a device pixel ratio, neither of which exists at
# tier 2), gestures.test.mjs, save.test.mjs, pageops.test.mjs and finalize.test.mjs. The count said "three" while the literal below said
# five, which is the sort of drift this guard exists to catch one level down.
expect_files=10
if [ "$files" -ne "$expect_files" ]; then
  echo "FAIL: expected $expect_files browser UI test files, found $files — a test file was added or dropped." >&2
  echo "      If deliberate, update expect_files in this script." >&2
  exit 1
fi
if [ "$n" -lt "$files" ]; then
  echo "FAIL: $n tests ran across $files files — a file contributed nothing, so its tests are silently not running" >&2
  exit 1
fi

exit "$code"
