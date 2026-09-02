# docs/red-proofs.md, tier 1: "an auto-confirming verifier reaches an interactive arm" (P08.S05d,
# v1.117.317)
#
# The defect: a reference to the delivery leg's auto-confirming `Verifier` from
# `handleSessionArm`. `runVerification` refuses a nil Verifier outright — "a nil Verifier is not
# 'skip the check' — it is a caller that forgot, and the whole of L2 is that no path reaches the
# exchange unconfirmed" — and an auto-confirming one is that forgotten gate made legitimate by
# SCOPE alone. So the scope is the whole of the safety argument, and "only the delivery path uses
# it" is a claim of ABSENCE that cannot be settled by reading the delivery path.
#
# The guard asserts ROUTING through `deliverOneLeg` rather than comparing known sites, so a tenth
# site added later fails whatever it looks like (ADR-009).
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheUnattendedGatesHaveOneDoor"
EXPECT="referenced outside deliverOneLeg"
