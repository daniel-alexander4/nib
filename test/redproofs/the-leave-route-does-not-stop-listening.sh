# docs/red-proofs.md, tier 1: "the leave route does not stop listening" (/pending 378, v1.128.25)
#
# The defect: `handleCeremonyLeave` prunes the invitation and does not release the arm. This is what
# SHIPPED at v1.128.10, and the route's own doc claimed otherwise — *"stops the arm on the next
# sweep"*. `rearmCeremonies` SKIPS a ceremony it holds no invitation for; a skip is not a teardown,
# so the standing arm went on holding the single interactive slot until Nib was quit, which is the
# exact condition the lever was shipped to relieve.
#
# **The acceptance test written alongside that lever cannot see this**, and its shape is the tell:
# `TestLeavingStopsTheArmAndKeepsItStopped` disarms the session BY HAND as setup, then asserts only
# that the next sweep does not re-arm. That second half is real and is still asserted; the first
# half was asserted nowhere.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestLeavingACeremonyReleasesTheArmItHeld -count=1"
EXPECT="still armed for a ceremony the user has just left"
