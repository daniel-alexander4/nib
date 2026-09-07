# docs/red-proofs.md, tier 1: "the consent block is the nominal size template" (P02.S03, v1.128.8)
#
# The defect: the consent surface reports `p2p.NominalBlockRect()` — which is what the quote route
# has always answered with, and which is NOT a placement. Its own doc: "this is a size template,
# not a placement — the caller wants a rect of the right shape and must not care where it says it
# is." Sending it here would draw a box in the corner of page zero and call it the signer's block.
#
# The row asserts an EQUALITY against `p2p.PlacementFor` on the same bytes and roster — the same
# door the stamp calls eight lines after `Confirm` returns, on the same `inbound` the confirmer was
# handed. Two implementations checked for agreement is the shape ADR-009 refuses; this is one
# implementation reached twice, and this row is what stops a hand-copy replacing it.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheBlockShownIsTheBlockStamped -count=1"
EXPECT="and the signature is stamped at page"
