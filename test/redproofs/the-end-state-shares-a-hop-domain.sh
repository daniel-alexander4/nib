# docs/red-proofs.md, tier 1: "the end state shares a hop domain" (/pending 380, v1.128.27)
#
# The defect: `EndStateSeed` derives on `hop-0`'s info string instead of its own, so the end state
# is published at a target a real hop uses. `derive`'s own doc states the rule this breaks: "a value
# used for one purpose can never be the value used for another — the failure that turns a rendezvous
# key into a decryption key."
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestTheEndStateDerivationsAreSeparatedFromTheHopFamily -count=1"
EXPECT="equals hop 0's seed"
