# docs/red-proofs.md, tier 1: "convening twice on one document is not refused at the route" (P08.S07, v1.117.246)
#
# The defect: `conveneStatus` stops mapping `ErrAlreadyConvened` to 409, so a second convene on a
# document that already carries a record answers 400 — "everything the convener can fix" — for the
# one refusal that is NOT about a field they can correct. C04's whole point is that the answer to
# this is a different action, not a corrected roster.
#
# The refusal itself has existed since P07.S02a and was driven only at the PACKAGE. This row is the
# route: the status, the named sentence, and C04's cost clause, which the criterion asks for
# separately precisely because it is the half a builder omits.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestConveningTwiceOnOneDocumentIsRefusedAtTheRoute -count=1"
EXPECT="want 409"
