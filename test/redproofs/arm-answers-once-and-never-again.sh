# docs/red-proofs.md, tier 1: "an arm answers once and never again" (P07.S05c, v1.117.183)
#
# The defect: the rate limit becomes permanent — an arm answers the first sighting of its peer and
# is silent forever after. It looks like a working mechanism and it is the mechanism's exact
# opposite: an arm lives for the ceremony (up to D33's thirty days) and a convener returns to it
# once per HOP, so silencing it after the first answer means every hop past the first cannot find
# that party on the link and falls through to the public DHT — the D6 leak the whole clause exists
# against.
#
# **A run cannot see it, which is why the policy is separated from the socket.** Every hop of every
# run in the tree falls inside the five-minute announce window, where the arm's own original
# announcement is still going out and the answer is not needed. Only a fake clock reaches the state
# where it is.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheArmAnswersItsOwnPeerAndNobodyElse -count=1"
EXPECT="a later hop of the same ceremony cannot find this party"
