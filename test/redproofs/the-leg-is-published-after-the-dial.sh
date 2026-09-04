# docs/red-proofs.md, tier 1: "the leg is published before it is attempted" (/pending 370, v1.117.354)
#
# The defect: `beginLeg` is called AFTER `deliverToParty` rather than before it.
#
# **This mutation SURVIVED its first pass and the guard exists because of it.** The jsdom test stubs
# the progress route with a canned answer, so it never exercises the server's ordering, and the unit
# test drives `beginLeg` directly — so a leg published after the dial compiled and went green. It
# names each party only once it has stopped being the one the convener is waiting on: a progress
# surface reporting exclusively the past.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheLegIsPublishedBeforeItIsAttempted"
EXPECT="AFTER the dial"
