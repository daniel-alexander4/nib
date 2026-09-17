# docs/red-proofs.md, tier 1: "The optional-content carry skips its reachability refusal"
# (/pending 525, v1.134.x)
#
# The defect: `carryOptionalContent` carries the subtree without asking whether it reaches a page the
# selection dropped.
#
# `/OCProperties` holds indirect references and pdfcpu writes by reachability, so a reference in it
# naming a dropped page puts that page's dictionary — and therefore its `/Contents` — back into the
# output that no page tree names. It is `pageselect.go`'s `/StructTreeRoot` hazard one key over, and
# with the walk removed it is not theoretical: the output holds THREE page objects for a two-page
# selection, which is the deleted page still in the file.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestOptionalContentReachingADroppedPageIsRefused -count=1"
EXPECT="the output holds 3 page objects, want the 2 it kept"
