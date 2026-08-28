# docs/red-proofs.md, tier 1: "A peer found on the link still races a two-second timer"
# (P07.S05d, v1.117.203)
#
# The defect: the dial side holds the DHT tier on `browseWindow` even when the browse already
# FOUND the peer on the link. Measured, and this is the row's whole point: with the bootstrap
# lazy and both DHT verbs already behind a 2 s window, a nine-party LAN relay still emitted 105
# off-link packets, and a stack probe named `publishWhenSlow`. `browseWindow` is how long a browse
# LISTENS; it was never a claim about how long a link-local dial takes, and a hop takes 1-3 s, so
# D6's suppression was a race the hop won often enough to leak.
#
# The dialer never had to guess: `peerAddresses` browses BEFORE the race, so a `sourceLAN`
# candidate is the link having already answered.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestALANFoundPeerHoldsTheDHTPastTheBrowseWindow"
EXPECT="reached the public DHT"
