# docs/red-proofs.md, tier 1: "one inbound DHT ping replaces a good node cache" (/pending 707, v1.169.12)
#
# The defect: cacheSet writes the routing table over the cache — every table node kept whether or not it
# answered us, and the previous cache dropped — so one stranger's ping can become the whole cache.
TIER="tier 1 — go test"
PROVE="go test ./internal/rendezvous/ -run '^TestOneInboundPingDoesNotReplaceAGoodNodeCache$' -count=1"
EXPECT="a stranger's single datagram replaced the list"
