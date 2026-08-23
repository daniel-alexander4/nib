# docs/red-proofs.md, tier 1: "a request the router received leaves nothing to delete" (/pending 257,
# v1.117.120)
#
# The defect: every error path in Map returns a zero Mapping, so a request that reached the router and
# then lost its reply left a mapping nothing could delete. It lived to lease expiry — measured at 7200 s
# against a 120 s request — and once the ceremony frees the ephemeral internal port, another process on
# the machine can bind it and be publicly reachable through the orphaned pinhole. The fixture asserts the
# gateway RECEIVED the request before claiming anything, so the row cannot pass over an absence.
TIER="tier 1 — go test"
PROVE="go test ./internal/portmap/ -run TestARequestThatReachedTheRouterLeavesADeletableHandle -count=1"
EXPECT="the mapping it may have created is undeletable"
