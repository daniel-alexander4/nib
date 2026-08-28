# docs/red-proofs.md, tier 1: "A quote route sizes the block by its own rule"
# (P07.S06, v1.117.210)
#
# The defect: a second implementation of the rule `NominalBlockRect` was written to be the one
# door for — and this is not hypothetical, it is what shipped. That function exists because "the
# rule had TWO implementations"; it fixed the hand-copied literal and left `handleCoSignQuote`,
# which computed a real placement over the open document to publish a rect whose POSITION the
# client never reads (`web/app.js:956` takes width and height and nothing else).
#
# The divergence was invisible because the half that differs is the half that is discarded. The
# guard asserts ROUTING through the door rather than comparing the literals, because comparing
# them is the copy agreeing with itself — the exact failure ADR-009 names, and the one that
# produced this defect.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryQuoteRouteSizesFromTheOneDoor"
EXPECT="sizes its block by some other rule"
