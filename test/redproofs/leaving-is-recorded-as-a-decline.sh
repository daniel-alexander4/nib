# docs/red-proofs.md, tier 1: "leaving is recorded as a decline" (P05.S01, v1.128.10)
#
# The defect: the local receipt records `declined` where the user chose to leave. They are one
# keystroke apart in a user's head and completely different in the record — a decline is an
# attested refusal the convener learns about and the roster is entitled to act on; leaving reaches
# nobody. This machine's receipt is the only artifact that can keep them apart locally, so a wrong
# value here is a durable misstatement of what its user did (D17).
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestLeavingWritesNoTermination -count=1"
EXPECT="leaving recorded the end state"
