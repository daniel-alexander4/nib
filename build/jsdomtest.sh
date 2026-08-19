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

# A floor of one is not an inventory. The argument above — "a runner that discovers
# NO tests looks exactly like a passing suite" — applies just as well to a runner
# that discovers 1 of 53: delete seven of the eight *.test.mjs files and the count
# check above still passes.
#
# So the population is pinned against the file system, which is the external source
# a self-referential count cannot be: every test file must contribute at least one
# test, and the FILE count itself is pinned as a literal. A literal per test would
# be worse than nothing — it goes red on every legitimate new test and trains the
# next person to bump the number, which is how V11's equality assertion rotted. A
# file count changes only when someone adds or deletes a file, which is deliberate.
Nib_files="$(find test/jsdom -maxdepth 1 -name '*.test.mjs' | wc -l | tr -d ' ')"
# One boot per file is this tier's standing rule — restore.test.mjs needs its own because
# its restore runs at module-evaluation time, and the same holds for every file since.
# Sixteen since P01.S02 added peername.test.mjs (v1.109.41). Bumping this literal is the
# deliberate act the guard exists to force; it went unbumped for nine commits, during which
# this harness EXITED 1 while still printing "# pass 96 / # fail 0" above it. Read the exit
# status, not the totals — the totals were true and the tier was red.
Nib_expect_files=16
if [ "$Nib_files" -ne "$Nib_expect_files" ]; then
  echo "FAIL: expected $Nib_expect_files jsdom test files, found $Nib_files — a test file was added or dropped." >&2
  echo "      If deliberate, update Nib_expect_files in this script." >&2
  exit 1
fi
if [ "$Nib_n" -lt "$Nib_files" ]; then
  echo "FAIL: $Nib_n tests ran across $Nib_files files — a file contributed nothing, so its tests are silently not running" >&2
  exit 1
fi

exit "$Nib_code"
