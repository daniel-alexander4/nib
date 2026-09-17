# docs/red-proofs.md, tier 1: "A carried note goes through its page's own crop offset" (ADR-045)
#
# The defect: `placementMatrix` composes only the sheet's `cm`, dropping the form's `/Matrix`.
# pdfcpu writes that matrix as a translation by `-cropBox.LL` (`createNUpFormForPDF`), so it is the
# identity only for a page whose box starts at the origin. `SplitPage`'s second tile carries
# `/CropBox [297.5 0 595 842]`, and without the term the note moves 148.75 points right — clean out
# of a 297.5-point-wide image.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestANUpCarriesANoteThroughAPagesOwnCropOffset -count=1"
EXPECT="the form /Matrix translates by"
