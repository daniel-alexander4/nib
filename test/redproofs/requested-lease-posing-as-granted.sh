# docs/red-proofs.md, tier 1: "a requested lease posing as a granted one" (/pending 253, v1.117.116)
#
# The defect: no way to tell a lease the router GRANTED from the one Nib asked for. The UPnP branch
# recorded `LifetimeSec: defaultLeaseSec` as though the router had answered with it — IGD's
# AddPortMapping has no lease out-argument — so on the mechanism D15 says most consumer routers run,
# D15's crash floor is unknown rather than 120 s, and an IGDv1 that ignores the lease installs a
# PERMANENT mapping that "120" cannot describe.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAGrantedLeaseIsDistinguishedFromARequestedOne -count=1"
EXPECT="must report as observed"
