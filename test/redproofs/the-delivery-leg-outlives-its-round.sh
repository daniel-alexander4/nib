# docs/red-proofs.md, tier 1: "the leg does not outlive its round" (/pending 370, v1.117.354)
#
# The defect: the closure `beginLeg` returns does not clear the entry.
#
# **The clear is a RETURNED CLOSURE precisely so a caller cannot take the publish and forget it.**
# A round that returned early — or panicked into safe.Recover — would leave a ceremony reporting a
# leg in flight forever, which is the stale-artifact failure this was put in memory to avoid: an
# artifact is evidence something STARTED, never that it is still running.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheRoundReportsTheLegItIsOn"
EXPECT="survives the round that published it"
