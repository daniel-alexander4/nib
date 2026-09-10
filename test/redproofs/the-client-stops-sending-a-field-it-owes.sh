# docs/red-proofs.md, tier 1: "every request field a handler reads is one some client sends"
# (/pending 447, v1.128.96)
#
# The OTHER half, and it is the one that proves the client scan reads anything at all. The guard has
# two sides and a server-side scan that came back empty would report zero members — indistinguishable
# from a clean run. This patch deletes the client's own `address` append on /api/session/initiate,
# which the handler reads, so a guard whose client half has gone blind stays green here.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryRequestFieldAHandlerReadsIsOneSomeClientSends -count=1"
EXPECT="/api/session/initiate address"
