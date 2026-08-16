#!/usr/bin/env bash
# Tier 2 of Nib's three-tier test harness: run the front-end tests that load the
# real web/app.js into jsdom.
#
# Usage: ./build/jsdomtest.sh
#
# Nib's client is 7k lines of JavaScript that the Go suite cannot see at all — the
# server never observes the browser's state, so every client-side failure mode is
# silent to `go test ./...`. This tier closes the DOM-observable part of that gap
# by running the shipped file, not a copy.
#
# Requires Node and a `npm install` (jsdom is the only dependency, dev-only, and
# node_modules/ is git-ignored). Skips cleanly without either, the same way the
# poppler/Ghostscript/veraPDF tests and build/winrepro.sh do — a fresh clone runs
# everything else without setting this up.
#
# Ceiling: jsdom models the DOM, not a rendering engine — no layout, no canvas, no
# media queries, and pdf.js itself is stubbed. build/uirepro.sh (tier 3) covers
# those against a real browser. See test/jsdom/boot.mjs for the full statement.
set -uo pipefail
cd "$(dirname "$0")/.."

command -v node >/dev/null 2>&1 || { echo "node not installed; skipping the front-end tests"; exit 0; }
[ -d node_modules/jsdom ] || { echo "jsdom not installed (run: npm install); skipping the front-end tests"; exit 0; }

Nib_out="$(node --test test/jsdom/ 2>&1)"
Nib_code=$?
echo "$Nib_out"

# A runner that discovers NO tests also exits 0, and would look exactly like a
# passing suite forever — the harness reporting health about a population it never
# had. So the count is checked, not just the exit code. (This is the harness
# applying to itself the rule it exists to enforce; the failure it prevents is the
# one nobody would ever see.)
Nib_n="$(printf '%s\n' "$Nib_out" | sed -n 's/^# tests \([0-9][0-9]*\)$/\1/p' | tail -1)"
if [ -z "$Nib_n" ] || [ "$Nib_n" -eq 0 ]; then
  echo "FAIL: the front-end suite ran but discovered no tests — a green with nothing in it" >&2
  exit 1
fi

exit "$Nib_code"
