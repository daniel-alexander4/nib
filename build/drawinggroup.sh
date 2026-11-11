#!/usr/bin/env bash
# The drawing-grouping instrument (PLAN-image-edit P03.S03 T02): how the one grouping of a page's painted paths into
# drawings (`groupDrawnPaths`) reads the first 4 pages of each document of a local list — counts by kind and by each
# cause the edit door refuses by, pages where the comparison budget ran out (their own column, never offered), and
# pages where the grouping differs from the rule before T01 — and how many of a hand-labelled sample's drawings a group
# still matches exactly. The window offers drawings only while that share is at least 0.9.
#
# Not a tier and not a gate, like build/objectdelete.sh, and for its reason: the documents are somebody's, so the
# corpus and the labels are LOCAL files, never committed:
#
#     ~/nib/population-corpus.txt               (or NIB_DRAWINGGROUP_LIST=/some/list.txt)
#     ~/nib/drawing-labels.tsv                  (or NIB_DRAWINGGROUP_LABELS=/some/labels.tsv; format in
#                                                internal/pdfops/drawinggroup_corpus_test.go)
#
# It prints COUNTS only — never a path. `--dump <dir>` writes the sample pages instead (each rendered with its groups
# outlined and numbered, and the groups' keys), which is what a person labels from; it judges nothing.
#
# Usage: ./build/drawinggroup.sh [--dump <dir> [--skip N]]   (--skip passes over the first N sample pages: a fresh
#        sample for judging a change made after the first was labelled)
set -uo pipefail
cd "$(dirname "$0")/.."

. "$(dirname "$0")/tiergate.sh" # nib_skip, nib_corpus_check
LIST="${NIB_DRAWINGGROUP_LIST:-$HOME/nib/population-corpus.txt}"
LABELS="${NIB_DRAWINGGROUP_LABELS:-$HOME/nib/drawing-labels.tsv}"
[ -f "$LIST" ] || { [ -z "${NIB_DRAWINGGROUP_LIST:-}" ] || { echo "FAIL: NIB_DRAWINGGROUP_LIST names $LIST and there is no such file" >&2; exit 1; }; }
[ -f "$LIST" ] || nib_skip "no corpus at $LIST (a local list of PDF paths; see this script's header)"
nib_corpus_check "$LIST"
[ -f "$LABELS" ] || LABELS=""

DUMP=""
SKIP=0
if [ "${1:-}" = "--dump" ]; then
	DUMP="${2:?--dump needs a directory}"
	mkdir -p "$DUMP"
	[ "${3:-}" = "--skip" ] && SKIP="${4:?--skip needs a number}"
fi

echo "$(cat VERSION 2>/dev/null || echo dev) · $(grep -vc '^\s*\(#\|$\)' "$LIST") documents · list sha256 $(sha256sum "$LIST" | cut -c1-12) · labels ${LABELS:+sha256 $(sha256sum "$LABELS" | cut -c1-12)}${LABELS:-none}"
NIB_DRAWINGGROUP_LIST="$LIST" NIB_DRAWINGGROUP_LABELS="$LABELS" NIB_DRAWINGGROUP_DUMP="$DUMP" NIB_DRAWINGGROUP_SKIP="$SKIP" \
	go test ./internal/pdfops/ -run '^TestDrawingGroupCorpus$' -count=1 -timeout 4h -v 2>&1 | grep -E '^(DRAWING|--- (FAIL|SKIP)|FAIL|ok|panic|\s+drawinggroup_corpus_test.go)'
exit "${PIPESTATUS[0]}"
