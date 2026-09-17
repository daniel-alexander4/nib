# docs/red-proofs.md, tier 1: "a refused hop arm opens its sockets first" (/pending 517, /pending 381)
#
# The defect, verbatim as it stood until /pending 517: `armCeremonyHop` bound a UDP socket, started
# a DHT server on it and opened a handshaked QUIC listener, and only THEN asked whether the
# interactive slot was free — closing all three again when it was not.
#
# The slot is one, and `rearmCeremonies` reaches this door on every unlock, vault import and accept,
# treating `errSessionArmed` as the ordinary outcome ("the user has the slot; this is not a fault").
# So the ordinary case was the one paying for a listener nobody would own. Nothing left the machine
# while it was open — `rendezvous.Open` runs no bootstrap, which is ADR-011's lazy door — so what
# this removes is local churn and an unowned listener, not an off-link leak.
#
# `slotTaken` was not new: /pending 381 built it, and `armForDelivery` has asked it before opening a
# socket ever since. This arm, the other half of the same rule, never called it — ADR-009's shape in
# the small.
#
# The check distinguishes the two orderings by their SENTINELS rather than by counting file
# descriptors: with an unbindable address, asking the slot first answers `errSessionArmed` and
# opening the endpoint first answers `errCeremonyEndpoint`.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestARefusedHopArmOpensNoSocket' -count=1"
EXPECT="the endpoint is opened before the slot is asked for"
