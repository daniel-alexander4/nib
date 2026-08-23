# docs/red-proofs.md, tier 1: "D19 cause 3 promises a port-forward to a user it knows nothing about"
# (/pending 251, v1.117.115)
#
# The defect: cause 3's `else` branch ends "…would let it connect". That branch is reached by the
# port-map tier learning NOTHING — `mapUnroutable` is set only where a gateway ANSWERED — so absence
# of evidence was read as evidence of a controllable NAT. An IPv6-transition CGNAT subscriber is one
# instance: `sharedSpace` is 100.64/10, IPv4-only. The patch also removes the v6-reachable branch, so
# a host with a directly reachable IPv6 endpoint is told again that a direct connection is impossible.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestCause3NeverPromisesWhatItCannotKnow -count=1"
EXPECT="promises a port-forward"
