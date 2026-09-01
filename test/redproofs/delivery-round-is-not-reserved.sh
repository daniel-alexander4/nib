# docs/red-proofs.md, tier 1: "the deadline reserves the hops and nothing for the round" (P08.S05b, C10, v1.117.292)
#
# The defect as SHIPPED until P08.S05b: Convene reserved `hops x HopBudget` and nothing for the
# delivery round that follows the last hop, so a ceremony could be admitted whose own document
# could not be delivered inside the deadline the user set.
#
# The check is the boundary: enough for the hops and one second more must now be REFUSED. Nothing
# else in the tree is within twenty hours of that boundary, which is why this row exists at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestTheReservationCoversTheDeliveryRoundAndNotJustTheHops -count=1"
EXPECT="was admitted"
