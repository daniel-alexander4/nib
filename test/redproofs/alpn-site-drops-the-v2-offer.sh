# docs/red-proofs.md, tier 1: "an ALPN config site drops the v2 offer" (P07.S03a, v1.117.162)
#
# The defect: one of the seven `NextProtos` assignments goes back to `[]string{alpn}`. Connections
# made through that listener or dialer never negotiate the named-refusal protocol, so every L3
# refusal on that path silently reverts to arriving as EOF — and **every behavioural test stays
# green**, because they drive the sites that were left alone.
#
# That is the population-floor shape, one layer below `TestL2CoversEveryDocumentCarryingEntryPoint`:
# a rule that holds at the call sites somebody remembered.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestEveryALPNConfigSiteOffersTheSameList -count=1"
EXPECT="sets NextProtos to something other than sessionALPN"
