# docs/red-proofs.md, tier 1: "Quit leaves the resume record to a handler the exit cuts off (ADR-085, v1.183.0)"
#
# The defect: Quit requests the exit without recording first, trusting the window stream's close to
# do it — and the teardown does not wait for that handler's vault write.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestQuitRecordsBeforeItExits"
EXPECT="by the time Quit answered"
