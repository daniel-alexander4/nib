# docs/red-proofs.md, tier 1: "The open window is never told of a launch's document (ADR-086, v1.184.0)"
#
# The defect: a hand-off installs its document in the running Nib and nothing tells the window that is
# already open, so that window stays on its old tabs and only a second window can show the document.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestALaunchWhileAWindowIsOpenIsPushedToThatWindow"
EXPECT="stays on its old tabs"
