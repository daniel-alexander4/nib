# docs/red-proofs.md, tier 1: "A table spanning the page still carries" (/pending 529)
#
# The defect: `cloneRoot` stops checking that a candidate ancestor's subtree lies on the duplicated
# page, so a `/Table` whose rows are on two pages gets copied whole.
#
# **It records the alternative `/pending 529` itself proposed.** That entry's other path was to
# refuse the carry where no ancestor lies wholly within the duplicated page — a table spanning the
# page break. Climbing past such an ancestor is that refusal arriving by the back door: the copy
# names a page the repeat is not making, the output fails the carry's own gate, and the fate comes
# out `dropped`. A document whose only fault is a table crossing a page break loses its whole tree.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestATableSPANNINGThePageCarriesItsRowsRatherThanRefusing -count=1"
EXPECT="came out dropped"
