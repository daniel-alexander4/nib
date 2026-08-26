# docs/red-proofs.md, tier 1: "a ceremony hop cannot be dialled over TCP" (P07.S05b, v1.117.180)
#
# The defect: `handleSessionInitiate` selects the glare/shared-endpoint dial path on
# `cer != nil && cer.rz != nil` alone. That path feeds its race through `filterQUIC` — *"the shared
# endpoint speaks QUIC, and a non-QUIC candidate cannot be handshake-dialled on it"* — and
# `dialerCeremony` opens that endpoint for EVERY ceremony, unconditionally. So a TCP candidate is
# filtered out of every ceremony dial and the hop races an empty set until `connectDeadline`: five
# minutes, receiver armed and idle, no error until the deadline.
#
# **No tier had ever carried a document over a ceremony dial on TCP**, which is why it survived
# four slices: the tier-4 N>=3 probe passes `transport=tcp` with an invitation and is refused 409
# by L3 at the near end BEFORE any network work; every other ceremony test is in-process with a
# hand-built channel; and the two-party tier-4 runs carry no invitation, so `cer` is nil and they
# take the else branch. Found by P07.S05b's relay driver, whose first TCP hop hung.
#
# The guard is structural for the reason `TestBothSidesOfAHopMirrorIt` gives: the property is which
# branch a request takes, and a completed QUIC request looks identical either way. The behavioural
# driver is tier 4 (`./build/pairrepro.sh -n 4`), which is not in the default run.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestACeremonyHopIsNotForcedOntoQUIC -count=1"
EXPECT="selected without consulting the requested transport"
