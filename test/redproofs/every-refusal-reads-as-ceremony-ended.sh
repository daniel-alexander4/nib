# docs/red-proofs.md, tier 1: "every arrival refusal reads as ceremony-ended" (/pending 345, v1.117.302)
#
# **The DISCRIMINATING row.** The notice is still produced — a user IS told, at the right moment,
# with a sentence — so an assertion that a refusal reaches the local surface stays green. What is
# wrong is which sentence: a roster mismatch, an unreadable record and a document from another
# proceeding all report that the ceremony has ENDED.
#
# It matters because the two kinds are the two ACTIONS. "The proceeding is over" tells the user to
# stop waiting; "this arrival was refused" tells them the arm is still worth holding. Collapsed into
# one, a party whose counterparty pasted the wrong document is told their ceremony is finished, and
# disarms.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestARefusedArrivalTellsTheLocalUser -count=1"
EXPECT="want \"arrival-refused\""
