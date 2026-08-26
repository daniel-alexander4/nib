#!/usr/bin/env bash
# Re-prove a row of docs/red-proofs.md on demand.
#
# That ledger records which defect was reintroduced and which assertion fired, and it names
# its own gap in the section "What this ledger is not":
#
#   "There is no --fixture switch that replays these on demand, so re-proving a row is a
#    manual edit-run-revert. That is a real gap and it is named here rather than left for
#    someone to discover: the cheap half (this record) is done, the expensive half (a mode
#    that reintroduces a defect on request and asserts the specific assertion fires) is not."
#
# This is the expensive half.
#
# ── Why it is not a --fixture switch in the product ─────────────────────────────
# The obvious shape is a flag the app reads that turns a defect back on. This repo has
# already paid for that shape once: `toolbarStyle` shipped half-built and its default would
# have hidden the toolbar for every existing vault — "a loaded gun, not inert" (v1.109.1). A
# switch whose whole purpose is to break the program is the same gun with a better excuse,
# and it would ship in the binary users run.
#
# So nothing is added to the product. Each row is a PATCH against the tracked tree, applied
# to a throwaway copy exported from HEAD. The product has no idea this exists.
#
# ── What it asserts ─────────────────────────────────────────────────────────────
# That the named check FAILS with the defect applied — and it distinguishes the two ways
# that can go wrong, because they mean opposite things:
#
#   * the patch did not apply       → the row is STALE; the code moved under it
#   * the patch applied and the     → the check no longer catches its own defect, which is
#     check still PASSED              the ledger's claim being false
#
# A row that cannot be re-proved is worth more as a loud failure than as a line of prose.
#
# Usage: ./build/redproof.sh [name|--all]   (no argument lists what is recorded)
set -uo pipefail
cd "$(dirname "$0")/.."
REPO="$PWD"
DIR="test/redproofs"

list() {
  echo "recorded red proofs:"
  for m in "$DIR"/*.sh; do
    [ -e "$m" ] || { echo "  (none)"; return; }
    echo "  $(basename "${m%.sh}")"
  done
}

name="${1:-}"
[ -n "$name" ] || { list; exit 0; }

# ── --all: replay every row, and report the set rather than the first failure ────
#
# `verify_test.go`'s count guard names its own blind spot in so many words: it can see a row
# that DISAPPEARS and not one that no longer re-proves, "and running the whole set is a
# minutes-long job that belongs in a sweep rather than in `go test`". It stayed a known gap
# because there was no one command to run in that sweep — the first person to actually do it
# hand-rolled a shell loop, and found EIGHT invalid rows of 81 (2026-08-25, v1.117.156).
#
# So the sweep gets a door. It exits non-zero if any row fails and prints every failure, not
# just the first: a run that stops at the first stale patch tells you nothing about the other
# seventy, which is how "some rows are stale" stays indistinguishable from "one row is stale".
if [ "$name" = "--all" ]; then
  pass=0; failed=""
  for m in "$DIR"/*.sh; do
    [ -e "$m" ] || { echo "no rows recorded" >&2; exit 1; }
    n="$(basename "${m%.sh}")"
    if out="$("$0" "$n" 2>&1)" && printf '%s' "$out" | grep -q '^ok:'; then
      pass=$((pass + 1))
    else
      failed="$failed $n"
      echo "── $n"
      printf '%s\n' "$out" | grep -v '^re-proving' | head -6
    fi
  done
  if [ -n "$failed" ]; then
    echo
    echo "FAIL: $pass row(s) re-proved;$failed did not." >&2
    echo "      A row that cannot be re-proved claims a coverage it no longer has." >&2
    exit 1
  fi
  echo "ok: all $pass recorded rows still go red against their own defects"
  exit 0
fi
spec="$DIR/$name.sh"
[ -f "$spec" ] || { echo "FAIL: no red proof named '$name'" >&2; list >&2; exit 1; }

# Each spec declares PROVE (the command that must fail) and TIER (for the message); the
# defect itself lives beside it as <name>.patch, GENERATED with `git diff` rather than typed.
# A hand-written diff gets its line numbers wrong and then fails as "stale" for a reason that
# has nothing to do with the code — which is the one failure mode this script must not invent.
PROVE=""; TIER=""
# shellcheck disable=SC1090
. "$spec"
patchfile="$DIR/$name.patch"
[ -n "$PROVE" ] || { echo "FAIL: $spec sets no PROVE" >&2; exit 1; }
[ -f "$patchfile" ] || { echo "FAIL: $patchfile is missing" >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
# HEAD, not the working tree: a proof must run against what is committed, or a half-finished
# edit in the tree decides whether it passes.
git archive HEAD | tar -x -C "$WORK" || { echo "FAIL: could not export HEAD" >&2; exit 1; }
# node_modules is not tracked and tier 2/3 need it. Symlinked, never copied — it is ~100 MB.
[ -d node_modules ] && ln -s "$REPO/node_modules" "$WORK/node_modules"

cd "$WORK"
if ! patch -p1 --silent <"$REPO/$patchfile"; then
  echo "FAIL: the recorded defect no longer applies to HEAD — '$name' is STALE." >&2
  echo "      The code moved under it. Re-record the patch or retire the row." >&2
  exit 1
fi

# EXPECT is mandatory, and a missing one is a hard error rather than a permissive default.
if [ -z "${EXPECT:-}" ]; then
  echo "FAIL: '$name' records no EXPECT token." >&2
  echo "      A row that only requires a non-zero exit is satisfied by the check having been" >&2
  echo "      DELETED — see the block below. Add EXPECT=<a string only the real assertion prints>." >&2
  exit 1
fi

echo "re-proving '$name' (${TIER:-unknown tier}) — the check below MUST fail…"
if eval "$PROVE" >"$WORK/out.log" 2>&1; then
  echo "FAIL: with the defect applied, the check still PASSED." >&2
  echo "      docs/red-proofs.md claims this check catches this defect. It does not." >&2
  tail -20 "$WORK/out.log" >&2
  exit 1
fi

# THE THIRD FAILURE MODE: red for the wrong reason.
#
# This block used to be absent, and its absence is the V1 defect in the file that teaches V1.
# The harness asserted only that $PROVE exited non-zero — so a check that no longer EXISTS
# reports "still goes red against its own defect". Measured: `node --test <deleted file>` exits
# 1, and `empty-state-message`'s patch touches web/style.css rather than the test file, so
# deleting test/jsdom/theme.test.mjs made this row print ok. Any compile break, syntax error or
# missing node_modules in the exported tree does the same. (The tier-1 row was safer only by
# accident: `go test -run <nonexistent>` exits 0.)
#
# So the assertion is the TOKEN the real check prints, not the exit status. A guard verified by
# its own absence is exactly what docs/red-proofs.md exists to stop being possible.
if ! grep -qF -- "$EXPECT" "$WORK/out.log"; then
  echo "FAIL: '$name' went red, but not for its own reason." >&2
  echo "      Expected the check's own assertion to print: $EXPECT" >&2
  echo "      A non-zero exit alone is also what a DELETED or uncompilable check produces." >&2
  tail -30 "$WORK/out.log" >&2
  exit 1
fi
echo "ok: '$name' still goes red against its own defect, and said so"
