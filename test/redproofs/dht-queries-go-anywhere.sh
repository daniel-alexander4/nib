# docs/red-proofs.md, tier 1: "a stranger's nodes list aims our DHT queries" (/pending 743, v1.169.18)
#
# The defect: screened.WriteTo sends a query to any address, not only those the cache rule admits, so a
# nodes list a stranger returns can point our queries at the LAN.
TIER="tier 1 — go test"
PROVE="go test ./internal/rendezvous/ -run '^TestAStrangersNodesListCannotAimOurQueriesAtTheLAN$' -count=1"
EXPECT="a stranger aimed our traversal at an address the node-cache rule refuses"
