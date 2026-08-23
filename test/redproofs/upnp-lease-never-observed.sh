# docs/red-proofs.md, tier 1: "the refresh loop never resolves the lease it could not read" (/pending 260,
# v1.117.127)
#
# The defect: UPnP-IGD's AddPortMapping has no lease out-argument, so a UPnP mapping carries our REQUEST
# wearing the granted lease's name. `LifetimeObserved` recorded that honestly and had THREE writers and
# ZERO production readers — the published-and-never-consumed shape this repo deletes fields for. The
# read-back is driven by the predicate rather than a tick count, because `p.current = nm` replaces the
# mapping after every refresh and the UPnP path returns a freshly unobserved one each time.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnUnobservedLeaseIsResolvedAndReported -count=1"
EXPECT="LifetimeObserved is decoration"
