# docs/red-proofs.md, tier 1: "a user's own arm is displaced" (P02 close, v1.128.14)
#
# The defect: `displacePolicyArm` stops requiring the incumbent to be `byPolicy`, so ANY arm
# request tears down a live session's slot.
#
# The rule it breaks is the one `setVerify` already keeps for gates and for the same reason: the
# incumbent wins. Displacement exists for a guess this machine made on its own, and widening it to a
# user's own arm turns "the second request wins" into the product's behaviour — which is the
# tripwire D22 protects, not a courtesy.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAUsersOwnArmIsNeverDisplaced -count=1"
EXPECT="a second arm over a live USER arm returned"
