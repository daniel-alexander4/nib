# docs/red-proofs.md, tier 1: "a harness opens a socket on accepting" (P02.S02, v1.128.6)
#
# The defect: the accept-time re-arm runs on any `Server`, not only one backing a real Nib process.
# A test constructs a `Server` dozens of times, isolating `configDir` and NOT `$HOME` —
# `EnableDeliveryRearm`'s own doc records what an ungated sweep cost when this happened one queue
# over: "TempDir RemoveAll cleanup: directory not empty" across five unrelated tests.
#
# The assertion is an ABSENCE, so it is timed against something rather than read alone: the same
# table's other case arms within the same wait, with the gate as the only difference. Without that
# pairing "not armed" is indistinguishable from an arm that was merely slower than the check.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestAcceptingArmsTheListenerAndOnlyInARealNibProcess/a_harness' -count=1"
EXPECT="opened a listener on a Server that never"
