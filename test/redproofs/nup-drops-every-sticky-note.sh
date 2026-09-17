# docs/red-proofs.md, tier 1: "An n-up carries the comments of the pages it composes" (ADR-045, /pending 562)
#
# The defect: `NUp` does not call `carryNoteAnnots`, which is what the operation did until this
# change. `api.NUp` composes its sheets as new page objects and never looks at `/Annots`, so the
# source page dictionaries leave the page tree taking every annotation with them.
#
# Measured on a three-page document with one AddNotes sticky note per page: 3 in, 0 out. The
# consequence that made it visible is /pending 488's digest golden, where ContentDigest of
# `notes-then-nup` was byte-identical to that of `nup2`.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestANUpCarriesEveryNoteOntoTheSheetItsPageLandedOn -count=1"
EXPECT="came out"
