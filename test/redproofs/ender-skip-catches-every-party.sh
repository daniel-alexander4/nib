# docs/red-proofs.md, tier 1: "the round skips EVERY party once anyone has ended the proceeding"
# (P08.S05e, v1.117.322)
#
# The worst outcome this rule has, and the two-party roster the test was first written with could
# not see it: with one non-convener party who IS the ender, "skip the ender" and "skip everybody"
# produce identical output. The round would then report success having delivered to nobody, which
# is C06's telling half silenced by the very change that was meant to make it cheap.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheRoundReportsTheEnderSkippedRatherThanDialling -count=1"
EXPECT="did not end the proceeding"
