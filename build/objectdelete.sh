#!/usr/bin/env bash
# The object-delete instrument (PLAN-image-edit P01.S01, D3's falsification): for each picture of each page of a local
# list of documents (the first 4 pages, the first 4 editable pictures on each), remove its painting operator in memory,
# write the document, read the page back, and say whether exactly that picture went: one picture fewer, the survivors'
# digests and boxes the original's minus the addressed one in order, the text-run count unchanged. It also counts the
# documents that fail a no-op rewrite ("not editable, rewrite fails"), whether each deleted picture's bytes are still
# in the file and still extractable, and the PDF/UA checker's defect count before and after (one deletion a document).
# Its move mode (PLAN-image-edit P02.S01) moves every movable picture it tries a little and shrinks it, and prints an
# OBJECTMOVE line: movable, attempted, clean (read back where asked, the page otherwise as it was), and why the rest
# were not movable, by cause.
#
# Not a tier and not a gate, like build/formshape.sh, and for its reason: the documents are somebody's, so the corpus
# is a LOCAL list that is never committed:
#
#     ~/nib/population-corpus.txt        (or NIB_OBJECTDELETE_LIST=/some/list.txt)
#
# One PDF path per line; blank lines and lines starting with # are skipped. With no list this skips cleanly. It prints
# COUNTS only — never a path. Three verdicts: clean (exit 0), a finding (a deletion read differently), and too few
# pictures read (fewer than 1,000 — a run over nothing says nothing).
#
# Declared: it does not render the page before and after (the plan's pdftoppm comparison); that is S02's to add.
#
# Usage: ./build/objectdelete.sh
set -uo pipefail
cd "$(dirname "$0")/.."

. "$(dirname "$0")/tiergate.sh" # nib_skip, nib_corpus_check
LIST="${NIB_OBJECTDELETE_LIST:-$HOME/nib/population-corpus.txt}"
[ -f "$LIST" ] || { [ -z "${NIB_OBJECTDELETE_LIST:-}" ] || { echo "FAIL: NIB_OBJECTDELETE_LIST names $LIST and there is no such file" >&2; exit 1; }; }
[ -f "$LIST" ] || nib_skip "no corpus at $LIST (a local list of PDF paths; see this script's header)"
nib_corpus_check "$LIST"

echo "$(cat VERSION 2>/dev/null || echo dev) · $(grep -vc '^\s*\(#\|$\)' "$LIST") documents · list sha256 $(sha256sum "$LIST" | cut -c1-12)"
NIB_OBJECTDELETE_LIST="$LIST" go test ./internal/pdfops/ -run '^TestObjectDeleteCorpus$' -count=1 -timeout 4h -v 2>&1 | grep -E '^(OBJECTDELETE|OBJECTMOVE|\s+objectdelete_corpus_test.go|--- (FAIL|SKIP)|FAIL|ok|panic)'
exit "${PIPESTATUS[0]}"
