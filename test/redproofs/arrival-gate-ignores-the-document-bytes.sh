# docs/red-proofs.md, tier 1: "the arrival gate never looks at the document's bytes"
# (P08.S03, C04, v1.117.328)
#
# The shipped state for two phases. `checkArrival` asked whether a record was present and verified,
# whether the deadline had passed, and whether the roster commitment matched the invitation — and
# all three pass for a DIFFERENT document carrying the same valid record, because the record is
# identical and none of the checks compares the bytes.
#
# `DocHash` exists for exactly this — *"Every party agrees to the same bytes and a resumed hop can
# prove it"* — and the only thing that compared it against a document was `CheckDocument`, which had
# ZERO production callers (`git grep CheckDocument( 4fbd279 -- '*.go'` outside tests and comments
# returns nothing).
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnUnsignedArrivalIsAnchoredToItsOwnRecord -count=1"
EXPECT="ADMITTED a different document"
