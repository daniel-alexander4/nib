# docs/red-proofs.md, tier 1: "VerifyAgainst never compares a termination's unsigned label with the anchor's id — ceremony A's valid end state verifies against ceremony B"
# (/pending 686, v1.166.7)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestATerminationIsRefusedWhenItsLabelNamesAnotherCeremony -count=1"
EXPECT="verified against ceremony"
