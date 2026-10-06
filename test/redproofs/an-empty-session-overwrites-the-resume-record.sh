# docs/red-proofs.md, tier 1: "A window that closes with nothing open erases the session before it (ADR-085, v1.183.0)"
#
# The defect: an empty set is recorded like any other, so opening Nib, looking at nothing and closing
# it costs the user the session they could have resumed.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestTheLastWindowGoingRecordsWhatWasOpen"
EXPECT="replaced the record"
