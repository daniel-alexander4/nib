# docs/red-proofs.md, tier 1: "a relay accumulates a document per hop" (P07.S05, v1.117.176)
#
# The defect: `installCeremonyResult` stops replacing by ceremony id and opens a document every
# time — which is what the relay did through `addDoc`, D10's arrival path.
#
# Each hop returns the SAME proceeding one signature further on, so a nine-party ceremony left the
# convener holding **nine copies against a count cap of eight**: the last hop would have been
# refused for a reason with nothing to do with the ceremony.
#
# Driven at NINE hops rather than two, because two is also what a door that replaces "the active
# document" looks like, and eight is where the cap is — a fixture that stops at seven cannot tell a
# working replace from a lucky one. Its controls are the other half: a SECOND ceremony gets its own
# document, and an arrival with no ceremony still adds, which is D10 and is unchanged.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestARelayReplacesTheBatonRatherThanAccumulating -count=1"
EXPECT="the registry holds"
