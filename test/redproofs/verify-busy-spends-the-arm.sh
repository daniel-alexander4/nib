# docs/red-proofs.md, tier 1: "a refused spoken check spends the arm" (P08.S05c, v1.117.314)
#
# The defect: `sv.saw.mark()` back as `ConfirmVerification`'s FIRST statement, ahead of the
# `setVerify` that discovers the seat is taken. That is what shipped. A gate refused with
# `errVerifyBusy` then reported its arm as spent, and `reached`'s own doc says what the mark
# means — "a connection put something in front of the local user" — which in the refused case
# is false: the goroutine returns an error and displays nothing.
#
# It is one line and it moves in the direction that looks harmless, which is why it needs a row:
# marking early reads as defensive, and the whole property is that it is not.
#
# `errVerifyBusy` had no test of any kind before this slice, so the incumbent-wins rule shipped
# with its reasoning in a doc comment and nothing exercising it.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestARefusedSpokenCheckDoesNotSpendTheArm"
EXPECT="SPENT the arm"
