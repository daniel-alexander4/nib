# docs/red-proofs.md, tier 1: "A note's placement is verified, not assumed" (ADR-045)
#
# The defect: the placement list is read against the source pages in reverse, so each note is
# offered the wrong page's tile.
#
# What fires is not a geometry check — it is `placementMatrix`'s identity check, which requires the
# form's decoded content to END with the page content the capture took. So a mapping this code got
# wrong abandons the carry and loses the notes rather than putting them on the wrong sheet, which is
# the "attempted and abandoned, never half-done" half of ADR-045 doing its job.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestANUpCarriesEveryNoteOntoTheSheetItsPageLandedOn -count=1"
EXPECT="came out"
