# docs/red-proofs.md, tier 1: "the writer ignores the placed page and uses the last" (/pending 305, v1.117.267)
#
# The defect: `SignApproval` resolves the appearance to the document's LAST page instead of the one
# the placement named.
#
# **This is the discriminating proof, and the discrimination is the point.** It passes at n=4 —
# where the single allocated signature page IS the last page, so "the last page" and "the placed
# page" are the same answer — and fails only at n=9, where two signature pages are allocated and
# the first block belongs on a page that is not the last. A mutation that reddened both rows would
# prove the test runs; this one proves the NEW row catches something the old one could not.
#
# Two earlier mutations were tried and rejected for exactly that reason: forcing page 1 fails both
# rows, and a byte-counting page estimate was simply wrong. Recorded because "it went red" is not
# the standard — it has to go red in the case being added.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestABlockIsActuallyDRAWNWhereItWasPlaced -count=1"
EXPECT="was drawn on page 5 and placed on page 4"
