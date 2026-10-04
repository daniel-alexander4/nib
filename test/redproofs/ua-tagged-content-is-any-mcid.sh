# docs/red-proofs.md, tier 1: "taggedContent counts any resolved /MCID as tagged, without asking whether its element reaches the structure tree root — a detached element passes 7.1 t3"
# (P04.S04, v1.148.0)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestAnMCIDIsNotTaggedContentUntilItReachesTheRoot -count=1"
EXPECT="an element detached from the structure tree: 7.1 t3 = pass"
