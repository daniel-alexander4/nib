# docs/red-proofs.md, tier 1: "Nib invents the sentence the parties are bound by" (P07.S07b, v1.117.219)
#
# The defect: `Contribute` stops refusing a commitment-bearing signature with an empty recital, so
# `intent()` falls back to `defaultIntent` and the /Reason reads "I agree to sign this document."
# over a proceeding whose real recital is inside the commitment every other signature carries.
#
# This is what makes "defaultIntent is unreachable when a record is present" true BY CONSTRUCTION
# rather than by the roster happening to carry one. Its reachable cause is an invitation older
# than `Invitation.Intent`, or one assembled by hand.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestACeremonySignatureWithNoRecitalIsRefusedRatherThanDefaulted"
EXPECT="a ceremony hop with no recital signed anyway"
