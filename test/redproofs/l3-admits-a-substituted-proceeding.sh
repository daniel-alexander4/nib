# docs/red-proofs.md, tier 1: "L3 admits a substituted proceeding" (P07.S03, v1.117.159)
#
# The defect: the gate stops comparing each signature's roster commitment against the one this
# party verified at arm time. Valid signatures, in the right order, by the right identities —
# committing to a ceremony that is not this one.
#
# **Its reach is limited today and the limit is asserted separately.** No production attestation
# carries a `RosterHash` (neither `coSignExchange`'s `att` nor `cosignAttestation` sets one), so
# this refusal fires only on signatures that DO carry one — which is defence against a document
# some future build wrote, not against today's. Making signatures carry it is P07.S04's, and
# `TestTheCommitmentCheckIsLimitedUntilS04` is what stops that boundary being read as coverage.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheGateRefusesEachThingByName -count=1"
EXPECT="substituted proceeding: admitted"
