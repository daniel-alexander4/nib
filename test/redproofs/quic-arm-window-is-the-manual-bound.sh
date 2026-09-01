# docs/red-proofs.md, tier 1: "a QUIC ceremony arm takes the manual bound" (/pending 344, v1.117.301)
#
# `session.armCeremony` — the door a QUIC ceremony arm goes through — stamps `sessionAcceptTimeout`
# instead of asking `armWindowFor`. The TCP door is untouched, so a party armed over TCP still waits
# for the ceremony and one armed over QUIC is disarmed after five minutes: C05 failing on exactly
# one of the two transports, which is the failure P08.S04 was built to close.
#
# **This is the row the test could not be until /pending 344.** `TestACeremonyArmWaits…OnBothTransports`
# sent `Transport: "tcp"` for BOTH of its arms, so the QUIC door was never entered and this defect
# was invisible to it. Measured: with this patch applied, the version of that test at v1.117.300
# passes green. It now names the transport in its failure.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestACeremonyArmWaitsForTheCeremonyOnBothTransports -count=1"
EXPECT="a quic CEREMONY arm's window is"
