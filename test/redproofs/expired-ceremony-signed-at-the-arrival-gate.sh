# docs/red-proofs.md, tier 1: "an expired ceremony is signed at the arrival gate" (P08.S04a, v1.117.282)
#
# The defect: the signing party's own deadline check is gone. Nothing else on that path compares
# `Expires` to `now` — `Record.Verify`'s only clock is a FUTURE ceiling — and the one refusal that
# exists has a single production caller on the CONVENER's side. So the signer is collected into a
# proceeding D28 declares over, and the convener holds the only clock that could have said so.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheArrivalGateRefusesAnEndedProceeding -count=1"
EXPECT="was accepted"
