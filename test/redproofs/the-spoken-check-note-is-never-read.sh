# docs/red-proofs.md, tier 1: "the spoken check note is never read" (P02.S04, v1.128.12)
#
# The defect: `ReadStored` stops populating `Stored.Verification`, so the note is written to disk
# and reaches nobody. It fails in the safe-looking direction — every reader sees absence, which
# means UNKNOWN — and that is exactly what makes it invisible: nothing looks wrong, and the answer
# D5 asks for is simply never available.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheSpokenCheckRecordsAllThreeOutcomes -count=1"
EXPECT="no note was recorded"
