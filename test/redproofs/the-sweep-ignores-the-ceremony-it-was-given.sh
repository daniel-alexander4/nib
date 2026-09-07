# docs/red-proofs.md, tier 1: "the sweep ignores the ceremony it was given" (P02 close, v1.128.16)
#
# The defect: `rearmCeremoniesPreferring` drops the preference and takes `ListStored`'s order — so
# on a machine holding an older accepted-and-unsigned ceremony, the one the user just joined is left
# unarmed. Arbitrary from their point of view, and measured at tier 4d as *"instance 3 could not arm
# before hop 1 (HTTP 409)"*.
#
# The check drives the sweep against a FREE slot, which is the honest scope of the guarantee: with
# one interactive slot, which of two live ceremonies wins is /pending 378's residual doubt and not a
# rule this sweep can settle.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheSweepPrefersTheCeremonyItWasGiven -count=1"
EXPECT="the sweep was given"
