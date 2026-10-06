# docs/red-proofs.md, tier 1: "A document queued while locked opens in the next session (ADR-086, v1.184.0)"
#
# The defect: a launch after the last window closed ends the session's documents and leaves its locked
# queue, so a file handed to the closed session opens beside the one this launch asked for.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestAQueuedDocumentDoesNotOutliveItsSession"
EXPECT="opens beside it"
