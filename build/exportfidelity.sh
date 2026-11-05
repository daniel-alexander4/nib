#!/usr/bin/env bash
# The export oracle's corpus instrument (PLAN-import-export R1, /pending 945): how complete is the document model on
# real files, and how many tagged pages would an order-sensitive tree check move to tree-incomplete?
#
# Not a tier and not a gate, like build/accuracy.sh and build/formrt.sh, and for their reason: real documents are
# somebody's, so the corpus is a LOCAL list of paths that is never committed:
#
#     ~/nib/export-corpus.txt        (or NIB_EXPORTFIDELITY_LIST=/some/list.txt)
#
# One path per line, with a `<list>.sha256` manifest beside it (build/pdfcpudiff.sh --manifest <list> writes one). With
# no list this skips cleanly. It prints COUNTS only — never a path, a word or a name. It needs no browser and no
# LibreOffice: the reference is DocumentText, an independent walk of the page, and the figures are in the header of
# TestExportFidelityCorpus (internal/docmodel/read/fidelitycorpus_test.go), which reads them off.
#
# Usage: ./build/exportfidelity.sh
set -uo pipefail
cd "$(dirname "$0")/.."

. "$(dirname "$0")/tiergate.sh" # nib_skip, nib_corpus_check
LIST="${NIB_EXPORTFIDELITY_LIST:-$HOME/nib/export-corpus.txt}"
[ -f "$LIST" ] || { [ -z "${NIB_EXPORTFIDELITY_LIST:-}" ] || { echo "FAIL: NIB_EXPORTFIDELITY_LIST names $LIST and there is no such file" >&2; exit 1; }; }
[ -f "$LIST" ] || nib_skip "no export corpus at $LIST (a local list of PDF paths; see this script's header)"
nib_corpus_check "$LIST"

echo "$(cat VERSION 2>/dev/null || echo dev) · $(grep -vc '^\s*\(#\|$\)' "$LIST") documents · list sha256 $(sha256sum "$LIST" | cut -c1-12)"
NIB_EXPORTFIDELITY_LIST="$LIST" go test ./internal/docmodel/read/ -run TestExportFidelityCorpus -count=1 -timeout 6h -v 2>&1 | grep -E '^(EXPORTFIDELITY|--- (FAIL|SKIP)|FAIL|ok|panic)'
exit "${PIPESTATUS[0]}"
