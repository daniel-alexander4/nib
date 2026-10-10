# docs/red-proofs.md, tier 1: "A ticked piece takes the ground painted under the page with it (ADR-125)"
#
# The defect: a named piece takes what is centred in it, as a drawn rectangle does, so a page's ground goes into the tag of whatever sits at the page's middle.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestPiecesNamesExactlyWhatIsTaken"
EXPECT="took the ground under the page with it"
