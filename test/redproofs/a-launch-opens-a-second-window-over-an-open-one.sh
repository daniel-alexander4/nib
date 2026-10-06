# docs/red-proofs.md, tier 1: "A launch is told to open a window although one is open (ADR-086, v1.184.0)"
#
# The defect: the hand-off's answer never says a window already has the document, so every launch opens
# another window on the same Nib, each showing every document.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestALaunchWhileAWindowIsOpenIsPushedToThatWindow"
EXPECT="opens a second window"
