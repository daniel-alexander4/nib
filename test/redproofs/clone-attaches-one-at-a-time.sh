# docs/red-proofs.md, tier 1: "A duplicated page's structure reads page then page" (/pending 529)
#
# The defect: `attach` puts each copy into its parent's `/K` on its own rather than as a block, so
# each lands immediately after its OWN original. That is where the interleaved reading order came
# from — `H1(p1) H1(p2) P(p1) P(p2) …`, a reader hearing every element of the original followed at
# once by its copy.
#
# It is the ordering clause's own row: under this patch both grouping clauses stay green and only
# the walk-order one fires, so the three assertions of this item are independent.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestADuplicatedPagesStructureReadsPageThenPage -count=1"
EXPECT="goes BACK a page mid-walk"
