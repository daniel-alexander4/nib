# docs/red-proofs.md, tier 1: "the arrival exemption stops being named" (/pending 337, v1.117.303)
#
# The ADR-009 half. `openArrival`'s exemption comment loses the phrase that identifies it, so a
# reader arriving at `s.addDoc(` sees an ordinary install where the tree has five other routes
# calling `addDocCapped` — and cannot reconstruct four slices' worth of argument for why this one
# is different.
#
# It goes red for a comment, deliberately: ADR-009 says a deliberate exemption is NAMED AT THE SITE,
# and an exemption nobody can find is indistinguishable from an oversight. This item exists because
# the naming that was there rested on a premise that had quietly stopped applying, and nothing
# noticed for four slices.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnArrivalCannotBePumped -count=1"
EXPECT="no longer names its exemption"
