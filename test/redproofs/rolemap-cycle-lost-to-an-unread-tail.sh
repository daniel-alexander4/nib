# docs/red-proofs.md, tier 1: "rolemap cycle lost to an unread tail" (/pending 548, 2026-09-16)
#
# The defect: the depth-bound guard is asked BEFORE the cycle search, so a document with a cycle at
# the top of its tree and elements past `maxWalkDepth` answers `CannotCheck` over a violation nib had
# already found. `/pending 496`'s law runs one way only — a rule may not PASS content it did not
# reach; a failure it did reach stays a failure.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestACycleAmongTheElementsNibReadIsSettledEvenWhenTheTailIsNot -count=1"
EXPECT="the cycle at the top is one nib established"
