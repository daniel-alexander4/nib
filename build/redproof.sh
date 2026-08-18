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
# Usage: ./build/redproof.sh [name]     (no name lists what is recorded)
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

echo "re-proving '$name' (${TIER:-unknown tier}) — the check below MUST fail…"
if eval "$PROVE" >"$WORK/out.log" 2>&1; then
  echo "FAIL: with the defect applied, the check still PASSED." >&2
  echo "      docs/red-proofs.md claims this check catches this defect. It does not." >&2
  tail -20 "$WORK/out.log" >&2
  exit 1
fi
echo "ok: '$name' still goes red against its own defect"
