# docs/red-proofs.md, tier 1: "a refused arrival tells nobody on this machine" (/pending 345, v1.117.302)
#
# `Confirm` refuses at the arrival gate and returns, exactly as it did between P08.S04a and this
# change. The PEER is unaffected — the error still becomes a named wire code — so every existing
# refusal test stays green. What is gone is the local half: the arm is a background goroutine with
# no response to write into, so the user sits waiting for a proceeding their own machine has just
# declared over.
#
# This is P08.S04a's own inventory row P5, whose zero-meaning is stated as "the peer is told and the
# local user is not". The row named a kind (`ceremony-ended`) that a named search found nowhere in
# the tree; the kind was owed rather than the row mistaken.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestARefusedArrivalTellsTheLocalUser -count=1"
EXPECT="want a sticky \"ceremony-ended\""
