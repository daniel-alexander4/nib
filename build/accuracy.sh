#!/usr/bin/env bash
# The accuracy harness (ADR-088): where do detected fields and search-redaction boxes actually land?
#
# Not a tier and not a gate. The four tiers answer "does it work"; this answers "how well", as numbers, on real
# documents — and real documents are somebody's, so the corpus is a LOCAL list of paths that is never committed:
#
#     ~/nib/accuracy-corpus.txt        (or NIB_ACCURACY_LIST=/some/list.txt)
#
# One path per line. With no list this skips cleanly, like a tier whose dependency is absent.
#
# What it does: builds nib; maps each document's busiest pages and writes a copy with its form fields removed
# (`TestAccuracyPrep` — a document's own fields are the answer key, and the copy is the form as someone without them
# has it); opens each copy in the real app in a real browser, presses Detect fields and runs a search-redaction for
# words whose true extent the map knows; and scores what came back (`test/accuracy/accuracy.mjs`).
#
# Usage: ./build/accuracy.sh [results.json]     (default: dist/accuracy/<version>.json)
set -uo pipefail
cd "$(dirname "$0")/.."

. "$(dirname "$0")/tiergate.sh" # nib_skip
LIST="${NIB_ACCURACY_LIST:-$HOME/nib/accuracy-corpus.txt}"
[ -f "$LIST" ] || nib_skip "no accuracy corpus at $LIST (a local list of PDF paths; see this script's header)"
command -v node >/dev/null 2>&1 || nib_skip "node not installed; skipping the accuracy harness"
[ -d node_modules/playwright-core ] || nib_skip "playwright-core not installed (run: npm install); skipping the accuracy harness"
BROWSER="${NIB_UI_BROWSER:-}"
if [ -z "$BROWSER" ]; then
  for c in google-chrome google-chrome-stable chromium chromium-browser microsoft-edge brave-browser; do
    if p="$(command -v "$c" 2>/dev/null)"; then BROWSER="$p"; break; fi
  done
fi
[ -n "$BROWSER" ] || nib_skip "no Chromium-family browser found; skipping the accuracy harness"

VERSION="$(cat VERSION 2>/dev/null || echo dev)"
RESULTS="${1:-dist/accuracy/$VERSION.json}"
mkdir -p "$(dirname "$RESULTS")"

# Its own port, so it can run beside a tier-3 run.
PORT="${NIB_ACCURACY_PORT:-18541}"
BASE="http://127.0.0.1:$PORT"
WORK="$(mktemp -d)"
SERVER_PID=""
cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" >/dev/null 2>&1
  if [ "${1:-0}" != "0" ]; then
    echo "the run failed — work dir PRESERVED at $WORK" >&2
    return
  fi
  rm -rf "$WORK"
}
trap 'cleanup $?' EXIT
trap 'cleanup 1; trap - EXIT; exit 130' INT TERM

. "$(dirname "$0")/launchkey.sh" # nib_answers, launch_token
if nib_answers "$BASE"; then
  echo "FAIL: something is already serving $BASE — set NIB_ACCURACY_PORT to a free port" >&2
  exit 1
fi

echo "building nib…"
go build -o "$WORK/nib" ./cmd/nib || { echo "FAIL: could not build nib" >&2; exit 1; }

echo "mapping the corpus…"
mkdir -p "$WORK/corpus"
NIB_ACCURACY_LIST="$LIST" NIB_ACCURACY_OUT="$WORK/corpus" go test ./internal/pdfops -run 'TestAccuracyPrep$' -count=1 >"$WORK/prep.log" 2>&1 || {
  echo "FAIL: could not prepare the corpus" >&2; cat "$WORK/prep.log" >&2; exit 1
}

mkdir -p "$WORK/home" "$WORK/config"
HOME="$WORK/home" XDG_CONFIG_HOME="$WORK/config" \
  NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 NIB_ADDR="127.0.0.1:$PORT" "$WORK/nib" >"$WORK/nib.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 60); do
  nib_answers "$BASE" && break
  sleep 0.25
done
nib_answers "$BASE" || { echo "FAIL: nib did not come up on $BASE" >&2; cat "$WORK/nib.log" >&2; exit 1; }
CSRF="$(launch_token "$BASE" "$WORK/nib.log")"
[ -n "$CSRF" ] || { echo "FAIL: could not trade the launch key nib logged for a token" >&2; exit 1; }
curl -fsS -X POST "$BASE/api/ssh/enroll" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
  -d "{\"mode\":\"create\",\"keyPath\":\"$WORK/home/.ssh/id_ed25519\"}" >/dev/null || {
  echo "FAIL: could not enroll a key" >&2; exit 1
}

export NIB_UI_BASE="$BASE" NIB_UI_BROWSER="$BROWSER" NIB_UI_WORK="$WORK" NIB_UI_CSRF="$CSRF"
node test/accuracy/accuracy.mjs "$WORK/corpus/manifest.json" "$RESULTS"
