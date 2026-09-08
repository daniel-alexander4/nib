# docs/red-proofs.md, tier 1: "the round opens an endpoint per leg" (/pending 376, v1.128.28)
#
# The defect: `runDeliveryRound` stops lending its endpoint, so `deliverToParty` opens its own — a
# UDP socket, a QUIC transport and a whole DHT server per party, each with its own
# `rate.NewLimiter(250, 64)`. That is the shipped state this row exists to keep from returning, and
# it is what made "make the round concurrent" look like a widening: W legs meant W×250/s of DHT send
# budget leaving one host with nothing bounding the total.
#
# **The mutation carries `_ = shared` deliberately.** Passing nil alone does not compile, and a
# compile break is `redproof.sh`'s "red, but not for its own reason" — indistinguishable from a
# deleted check.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheDeliveryRoundOpensOneEndpointForTheWholeWalk -count=1"
EXPECT="no longer hands its shared endpoint"
