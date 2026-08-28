# docs/red-proofs.md, tier 1: "A block's index counts non-signing roster entries"
# (P07.S06, v1.117.210)
#
# The defect: `SigningPositionOf` walks the whole roster instead of `SigningOrder`, so a
# non-signing convener burns a block slot. Every party after it is one position out, and the last
# one runs off the page sooner than the roster says it should — a page-box clip arriving early,
# for a party who never signs.
#
# It is invisible on any roster where signing order and roster order agree, which is most of them:
# the fixture puts a NON-SIGNING convener FIRST for exactly that reason, the same setup
# `PredecessorOf`'s test uses and for the same reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheBlockIndexIsTheRosterPositionNotASignatureCount"
EXPECT="burning a block slot"
