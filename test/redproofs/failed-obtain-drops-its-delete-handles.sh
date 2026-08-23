# docs/red-proofs.md, tier 1: "a failed obtain drops the handles for a mapping that may exist"
# (v1.117.130, from grilling /pending 262)
#
# The defect, and this week's own change opened it: since v1.117.120 the mapper records a delete handle
# for every request that LEFT this host, and close() is the only thing that drains them.
# appendMappedCandidate's failure return did not close, so the handles were garbage-collected — the exact
# leak /pending 257 was built to close, re-opened at a different door BY 257's own change. The sharpest
# instance is the UPnP path where AddPortMapping answers 200, the mapping EXISTS, GetExternalIPAddress
# fails, and the whole obtain reports a miss.
#
# The guard is a SOURCE scan and says so at the line: the call site builds its own portmap.Client and
# cannot be driven with a fake, so it asserts that every early return between the mapper's creation and
# its storage closes it, with a stimulus that there are two such returns.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAFailedObtainStillClosesItsMapper -count=1"
EXPECT="returns without closing the mapper"
