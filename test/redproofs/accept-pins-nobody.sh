# docs/red-proofs.md, tier 1: "accepting an invitation pins nobody" (P07.S02b, v1.117.157)
#
# The defect: `/api/ceremony/accept` parses the invitation, answers 200, and establishes no pin —
# so D21's step is not removed and the party still has to type a fingerprint by hand. The route
# would look like it worked: the response carries the roster, the ceremony id and the convener's
# six-word name, and only the next arm fails.
#
# The guard is written as **arm fails, accept, arm succeeds** rather than as "a pin appeared in
# the vault", because the refusal this closes is `handleSessionArm`'s and a pin that does not
# satisfy the door it was created for satisfies nothing.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAcceptingAnInvitationRemovesTheManualPin -count=1"
EXPECT="after accepting the invitation, arming still failed"
