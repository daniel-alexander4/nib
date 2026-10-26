#!/usr/bin/env bash
# The form round-trip instrument (/pending 271): does a form filled by pdf.js read back through pdfcpu, and one
# filled by pdfcpu read back in the window — on forms this repo did not author?
#
# Not a tier and not a gate, like build/accuracy.sh and for its reason: real forms are somebody's, so the corpus
# is a LOCAL list of paths that is never committed:
#
#     ~/nib/formrt-corpus.txt        (or NIB_FORMRT_LIST=/some/list.txt)
#
# One path per line. With no list this skips cleanly. A list is easy to make — every PDF on the machine that
# says it has a form:  find ~ -iname '*.pdf' -print0 | xargs -0 grep -la '/AcroForm' > ~/nib/formrt-corpus.txt
#
# What it does: starts a nib on a throwaway home (a fresh build, or NIB_FORMRT_BIN to judge an installed one),
# opens each document in a real browser, and walks both directions (test/formrt/roundtrip.test.mjs). The originals
# are opened and never written; the filled copies live in the work directory for one comparison each and are
# removed. It prints counts and field TYPES only — never a field's name, a value or a path.
#
# Usage: ./build/formrt.sh [results.json]     (default: dist/formrt/<version>.json)
set -uo pipefail
cd "$(dirname "$0")/.."

. "$(dirname "$0")/tiergate.sh" # nib_skip
LIST="${NIB_FORMRT_LIST:-$HOME/nib/formrt-corpus.txt}"
[ -f "$LIST" ] || { [ -z "${NIB_FORMRT_LIST:-}" ] || { echo "FAIL: NIB_FORMRT_LIST names $LIST and there is no such file" >&2; exit 1; }; }
[ -f "$LIST" ] || nib_skip "no form corpus at $LIST (a local list of PDF paths; see this script's header)"
nib_corpus_check "$LIST"
command -v node >/dev/null 2>&1 || nib_skip "node not installed; skipping the form round-trip"
[ -d node_modules/playwright-core ] || nib_skip "playwright-core not installed (run: npm install); skipping the form round-trip"
BROWSER="${NIB_UI_BROWSER:-}"
if [ -z "$BROWSER" ]; then
  for c in google-chrome google-chrome-stable chromium chromium-browser microsoft-edge brave-browser; do
    if p="$(command -v "$c" 2>/dev/null)"; then BROWSER="$p"; break; fi
  done
fi
[ -n "$BROWSER" ] || nib_skip "no Chromium-family browser found; skipping the form round-trip"

VERSION="$(cat VERSION 2>/dev/null || echo dev)"
RESULTS="${1:-dist/formrt/$VERSION.json}"
mkdir -p "$(dirname "$RESULTS")"

PORT="${NIB_FORMRT_PORT:-18561}"
BASE="http://127.0.0.1:$PORT"
# Under /tmp whatever TMPDIR says: Chromium refuses a profile whose socket path is too long.
WORK="$(mktemp -d /tmp/nib-formrt.XXXXXX)"
SERVER_PID=""
cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" >/dev/null 2>&1
  rm -rf "$WORK" # filled copies of somebody's forms: never kept, whatever the run did
}
trap cleanup EXIT
trap 'cleanup; trap - EXIT; exit 130' INT TERM

. "$(dirname "$0")/launchkey.sh" # nib_answers, launch_token
if nib_answers "$BASE"; then
  echo "FAIL: something is already serving $BASE — set NIB_FORMRT_PORT to a free port" >&2
  exit 1
fi

BIN="${NIB_FORMRT_BIN:-}"
if [ -z "$BIN" ]; then
  echo "building nib…"
  go build -o "$WORK/nib" ./cmd/nib || { echo "FAIL: could not build nib" >&2; exit 1; }
  BIN="$WORK/nib"
fi

mkdir -p "$WORK/home" "$WORK/config"
HOME="$WORK/home" XDG_CONFIG_HOME="$WORK/config" \
  NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 NIB_ADDR="127.0.0.1:$PORT" "$BIN" >"$WORK/nib.log" 2>&1 &
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

echo "$("$BIN" version | head -1) · $(grep -vc '^\s*\(#\|$\)' "$LIST") documents · results → $RESULTS"
export NIB_UI_BASE="$BASE" NIB_UI_BROWSER="$BROWSER" NIB_UI_WORK="$WORK" NIB_UI_CSRF="$CSRF"
export NIB_FORMRT_LIST="$LIST" NIB_FORMRT_BIN="$BIN" NIB_FORMRT_RESULTS="$RESULTS"
TMPDIR=/tmp node --test --test-timeout=14400000 test/formrt/roundtrip.test.mjs >"$WORK/run.log" 2>&1
code=$?
node test/formrt/report.mjs "$RESULTS" || { echo "FAIL: no results — the run's own log follows" >&2; grep -vE '/[^ ]+\.pdf' "$WORK/run.log" | tail -30 >&2; exit 1; }
exit "$code"
