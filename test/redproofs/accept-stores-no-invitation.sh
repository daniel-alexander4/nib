# docs/red-proofs.md, tier 1: "accepting an invitation stores nothing" (P08.S01, v1.117.240)
#
# The defect: `/api/ceremony/accept` pins the convener and does not keep the invitation, so a party
# who accepts and then restarts Nib has a pin, an identity, and no way to rejoin the ceremony —
# `ceremonyFor` begins at `ParseInvitation(text)` and there is no text. D21 removed a manual step;
# without this it came back one process boundary out.
#
# The guard is written as **arm by ceremony id succeeds**, not as "a row appeared in the vault", for
# the reason `accept-pins-nobody` gives about pins: a row that does not satisfy the door it was
# created for satisfies nothing. Its own stimulus is an arm naming a ceremony this machine never
# accepted, which must be refused — so the pass below cannot come from a build that ignores the field.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAcceptPersistsTheInvitationSoAReArmNeedsNoPaste -count=1"
EXPECT="arming by ceremony id after accepting returned"
