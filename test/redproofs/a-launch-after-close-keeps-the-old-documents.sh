# docs/red-proofs.md, tier 1: "A launch after the last window closed keeps what that window held (ADR-085, v1.183.0)"
#
# The defect: the hand-off cancels the idle-exit grace and nothing else, so a launch arriving inside
# the ten seconds after Nib was closed is handed a process still holding every document the closed
# window had — and the window it surfaces adopts them all. This is the behaviour ADR-085 exists to end.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestALaunchAfterTheLastWindowClosedStartsFresh"
EXPECT="came back"
