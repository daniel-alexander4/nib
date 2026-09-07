# docs/red-proofs.md, tier 1: "an explicit arm is refused over a policy arm" (P02 close, v1.128.14)
#
# The defect: `handleSessionArm` does not displace this machine's own accept-time arm, so a party
# who accepts and then presses Arm is told `409 a session is already armed`.
#
# **D14 inverted D21's observable invariant and only tier 6 could see it.** D21's rule is that
# accepting removes the manual PINNING step, and `ceremonyrepro.sh` observes it the only way it can
# — accept, then arm, require 200. Once accepting ARMS, the step D21 removed came back as a
# conflict, and the message names nothing the user can act on.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnExplicitArmDisplacesTheAcceptTimeArm -count=1"
EXPECT="arming after accepting returned"
