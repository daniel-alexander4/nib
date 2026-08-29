# docs/red-proofs.md, tier 1: "an arm given two invitation sources picks one" (P08.S01, v1.117.240)
#
# The defect: `/api/session/arm` accepts both a stored ceremony id and a literal invitation, and one
# wins by code order. Both are individually valid, so the arm succeeds — and the caller who supplied
# the other has no way to learn which ceremony was armed for. Two sources for one value with the
# loser silent is the drift ADR-009 exists against, and `checkTransport` already records the answer
# for its own case: refuse, never downgrade.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnArmNamesOneSourceForItsInvitation -count=1"
EXPECT="one of the two was chosen silently"
