# docs/red-proofs.md, tier 1: "a dual-stack host publishes one family" (D8 tier 2, P05.S05)
#
# The defect: `publishableEndpoints` reverts to `addr := self.V4.Addr; if !addr.IsValid() {
# addr = self.V6.Addr }` and a single endpoint. This is the code that shipped from P04.S03
# until v1.117.43, and it is the reason D8's tier 2 was unreachable rather than merely
# untested.
#
# It reads as dual-stack support and is the opposite: the v6 line is reached ONLY when the v4
# observation is invalid, so the one host that never publishes a v6 address is the dual-stack
# host — which is the host tier 2 exists for. A peer then has no v6 address to dial, and no
# amount of correct socket work reaches the tier.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAPublishedRecordCarriesBothFamilies"
EXPECT="want 2"
