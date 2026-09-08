# docs/red-proofs.md, tier 1: "the hop is not carried on the wire" (/pending 385, v1.128.30)
#
# The defect: `Encode` writes the no-hop sentinel whatever the announcement says, so every arm
# advertises itself as having no ceremony and a dialer is back to guessing between them.
TIER="tier 1 — go test"
PROVE="go test ./internal/discovery/ -run TestTheHopSurvivesTheWire -count=1"
EXPECT="a dialer would be sent to the wrong arm"
