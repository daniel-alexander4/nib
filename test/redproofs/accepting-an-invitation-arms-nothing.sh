# docs/red-proofs.md, tier 1: "accepting an invitation arms nothing" (P02.S02, v1.128.6)
#
# The defect: `/api/ceremony/accept` pins the convener, stores the invitation, answers 200 — and
# leaves the machine unarmed. From the convener's side that party is indistinguishable from one who
# ignored the invitation: the convener dials, nothing answers, and the proceeding stalls on somebody
# who believes they have done their part. Measured against HEAD before the slice: a successful
# accept left `/api/session/status` answering `{armed: false}`, and the only way on was for the user
# to find the Receive modal, pick the convener out of a peer list and press Arm.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestAcceptingArmsTheListenerAndOnlyInARealNibProcess/a_real' -count=1"
EXPECT="left this machine unarmed after"
