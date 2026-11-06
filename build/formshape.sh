#!/usr/bin/env bash
# The form-shape instrument (/pending 391): is a structural fingerprint of a form stable across a new revision or a
# re-scan of the same form, and distinct across different forms? A negative result closes 391 (and tells 395 and 396).
#
# Not a tier and not a gate, like build/accuracy.sh and build/formrt.sh, and for their reason: real forms are somebody's,
# so the corpus is a LOCAL list that is never committed:
#
#     ~/nib/formshape-corpus.txt        (or NIB_FORMSHAPE_LIST=/some/list.txt)
#
# One path per line. A BLANK LINE starts a new form: the paths in one block are the same form (a newer revision, a
# re-scan at another DPI) and paths in different blocks are different forms. Aim for ~20 recurring forms, a handful
# re-scanned, two with a newer revision. With no list this skips cleanly; a `<list>.sha256` manifest beside it
# (`build/pdfcpudiff.sh --manifest <list>`) pins which files were read.
#
# It needs no browser: the reader is pdfops.MapPage, whose answers are fractions of the displayed page, so a scan's DPI
# does not move them. A scan with no text layer has nothing this reader fingerprints and is counted UNMEASURED, which is
# the declared gap (its picture needs web/detect.js). It prints COUNTS only — never a path, a word or a name.
#
# Usage: ./build/formshape.sh
set -uo pipefail
cd "$(dirname "$0")/.."

. "$(dirname "$0")/tiergate.sh" # nib_skip, nib_corpus_check
LIST="${NIB_FORMSHAPE_LIST:-$HOME/nib/formshape-corpus.txt}"
[ -f "$LIST" ] || { [ -z "${NIB_FORMSHAPE_LIST:-}" ] || { echo "FAIL: NIB_FORMSHAPE_LIST names $LIST and there is no such file" >&2; exit 1; }; }
[ -f "$LIST" ] || nib_skip "no form corpus at $LIST (a local list of PDF paths, a blank line between forms; see this script's header)"
nib_corpus_check "$LIST"

echo "$(cat VERSION 2>/dev/null || echo dev) · $(grep -vc '^\s*\(#\|$\)' "$LIST") documents · list sha256 $(sha256sum "$LIST" | cut -c1-12)"
NIB_FORMSHAPE_LIST="$LIST" go test ./internal/pdfops/ -run TestFormShapeCorpus -count=1 -timeout 2h -v 2>&1 | grep -E '^(FORMSHAPE|--- (FAIL|SKIP)|FAIL|ok|panic)'
exit "${PIPESTATUS[0]}"
