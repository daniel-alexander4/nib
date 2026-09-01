# docs/red-proofs.md, tier 1: "one host on two transports is folded into one" (/pending 20, v1.117.312)
#
# `heldOnSameHost` drops its transport term, so a QUIC endpoint REPLACES the TCP one on the same IP
# and the race loses a transport the peer offered.
#
# **Found by mutation, and the first fixture written for it did not catch it.** Every endpoint in
# this package is built through `ep()`, which is TCP-only, so dropping the transport check left the
# whole package green; the arm added for it then put both transports on the SAME PORT, where the
# port-differs term shielded the transport term and the test still proved nothing. Two transports on
# one host are two sockets and therefore two ports — which is both the realistic shape and the only
# one that exercises this.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestOneHostOnTwoTransportsIsTwoCandidates -count=1"
EXPECT="the race lost a transport the peer offered"
