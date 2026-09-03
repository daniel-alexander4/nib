# docs/red-proofs.md, tier 1: "an accept that could not save leaves no pin" (/pending 364, v1.117.344)
#
# The defect, and it was live until this row: every vault mutator writes `v.contents` and then
# calls `save()`, so a failed save leaves the change standing in memory. `/api/ceremony/accept`
# answered 500 *"nothing was accepted"* while the convener stayed pinned for the life of the
# process — and `POST /api/session/arm` against them then returned 200. The machine had accepted
# the invitation and told its user it had not.
#
# Driven at the ARM door rather than at the pin list, because that refusal is what the pin exists
# to satisfy: a pin that does not open the door it was created for satisfies nothing.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnAcceptThatCouldNotSaveLeavesNothingBehind -count=1"
EXPECT="arming against the convener SUCCEEDED"
