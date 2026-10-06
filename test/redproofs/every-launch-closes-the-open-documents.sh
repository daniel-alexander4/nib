# docs/red-proofs.md, tier 1: "Every launch ends the session, window or no window (ADR-085, v1.183.0)"
#
# The defect: the session ends on ANY hand-off rather than on one that cancelled a grace, so
# double-clicking a second PDF while Nib is open closes the first under the window showing it.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestALaunchWhileAWindowIsOpenClosesNothing"
EXPECT="closed documents under an open window"
