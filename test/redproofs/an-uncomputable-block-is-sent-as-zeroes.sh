# docs/red-proofs.md, tier 1: "an uncomputable block is sent as zeroes" (P02.S03, v1.128.8)
#
# The defect: a document whose placement cannot be computed yields `&pendingBlock{}` rather than
# nil. A rect of `[0,0,0,0]` on page 0 is a LOCATION, and this is the absence of one — the client
# would draw a zero-sized box in the corner of the first page and the signer would be shown a
# placement Nib does not have.
#
# The same distinction `pendingView.Recital` draws one field over, and the same one `Stored.Me` and
# `Stored.Ended` draw: absence and a zero value are different facts, and only the surface can keep
# them apart if the wire does.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnUncomputableBlockLeavesTheRestOfTheSurface -count=1"
EXPECT="is a LOCATION"
