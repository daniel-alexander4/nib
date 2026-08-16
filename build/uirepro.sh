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

echo "building nib…"
go build -o "$WORK/nib" ./cmd/nib || { echo "FAIL: could not build nib" >&2; exit 1; }

# A throwaway HOME so the run creates its own vault and cannot touch the
# developer's real one — the same isolation the Go server tests use (t.Setenv).
mkdir -p "$WORK/home"
HOME="$WORK/home" NIB_NO_BROWSER=1 NIB_ADDR="127.0.0.1:$PORT" "$WORK/nib" >"$WORK/nib.log" 2>&1 &
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
out="$(node --test test/ui/ 2>&1)"
code=$?
echo "$out"

# Same trap tier 2 found: a runner that discovers no tests also exits 0, and would
# read as a passing suite forever.
n="$(printf '%s\n' "$out" | sed -n 's/^# tests \([0-9][0-9]*\)$/\1/p' | tail -1)"
if [ -z "$n" ] || [ "$n" -eq 0 ]; then
  echo "FAIL: the browser UI suite ran but discovered no tests — a green with nothing in it" >&2
  exit 1
fi

exit "$code"
