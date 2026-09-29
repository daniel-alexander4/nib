# docs/red-proofs.md, tier 1: "nib pagenum --continuous writes over an input it has not read" (/pending 663, v1.169.9)
#
# The defect: OutputOverwritingAnotherSource never finds a clash, so input 1's output lands on input 2's
# file (another folder, same base name) before input 2 is read — its pages vanish, exit 0.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run '^TestContinuousPagenumNeverWritesOverALaterInput$' -count=1"
EXPECT="was overwritten before it was read"
