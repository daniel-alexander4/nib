# docs/red-proofs.md, tier 1: "`honest` is a post-condition, not a strip" (P01.S01, v1.129.16)
#
# The defect: v1.129.9 verbatim. `honest` drops `/StructTreeRoot` and `/MarkInfo` from every
# document it is handed instead of only from one whose tree anchors to nothing — so a document
# whose structure survived comes back with the structure deleted. That shipped for one session
# and destroyed real tag trees, on the strength of a byte count that could not see a compressed
# object stream. Deleting the guard clause is a two-line edit that reads as a simplification.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestHonestLeavesACarriedTreeALONE"
EXPECT="no business touching"
