# docs/red-proofs.md, tier 1: "a TCP ceremony arm cannot re-deliver" (/pending 289, v1.117.187)
#
# The defect: `runSession` returns as soon as a session is served, so on the TCP ceremony path the
# listener closes the moment the co-sign completes and a reconnect is met with `connection
# refused`. P05.S10's criterion 15 — a lost writeback re-delivers the cached signature rather than
# re-signing — was implemented on the QUIC ceremony path only. `coSignExchange` still wrote its
# cache on TCP; nothing could ever come back for it.
#
# **It survived because the one behavioural drive of re-delivery ran QUIC.** The TCP rule was
# guarded structurally — `TestEveryCeremonySessionGetsItsCeremony` asserts both call sites PASS a
# ceremony — and asserting that a value is handed over says nothing about what the receiver does
# with it. The item that filed this said the drive needed tier 4; its own QUIC twin was a package
# test, and the only transport-specific part was the handful of lines that dial.
#
# The check is table-driven over transport, so the QUIC subtest stays green while the TCP one goes
# red — which is what makes the failure attributable to the transport rather than to the fixture.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestCeremonyReDeliversAfterReconnect -count=1"
EXPECT="connection refused"
