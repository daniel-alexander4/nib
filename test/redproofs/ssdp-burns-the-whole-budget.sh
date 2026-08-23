# docs/red-proofs.md, tier 1: "discovery spends the whole budget before the first HTTP byte"
# (/pending 264, v1.117.135)
#
# The defect: discoverIGD's read loop ran to its deadline unconditionally, so every UPnP obtain spent
# the full upnpHTTPBudget before its first HTTP byte — out of a portMapBudget that then had to cover
# three SOAP round trips. That squeeze is the recorded reason a lease read-back "does not fit" at
# obtain time, and why the written-but-unanswered POST window is the likely field case rather than a
# theoretical one.
#
# The wire timing is not driven — SSDP is real multicast and this package's ceiling forbids reaching
# for it — so the row is about the ARITHMETIC, which is where the defect lives.
TIER="tier 1 — go test"
PROVE="go test ./internal/portmap/ -run TestTheUPnPBudgetLeavesRoomForTheCallsItHasToMake -count=1"
EXPECT="still costs the full budget"
