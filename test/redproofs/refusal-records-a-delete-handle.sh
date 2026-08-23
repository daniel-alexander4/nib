# docs/red-proofs.md, tier 1: "a definitive refusal records a delete handle anyway" (/pending 265,
# v1.117.134)
#
# The defect: a UPnP delete is keyed on (external port, protocol) with NO internal client, so a handle
# recorded for a mapping that was never made arms a cross-host destructive call at whatever else holds
# that port. The rule existed with a comment and no test, because mapViaUPnP called discoverIGD
# unconditionally and discoverIGD does real SSDP multicast. The locations seam is what made this
# testable at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/portmap/ -run TestMapViaUPnPLoop -count=1"
EXPECT="recorded 1 delete handle"
