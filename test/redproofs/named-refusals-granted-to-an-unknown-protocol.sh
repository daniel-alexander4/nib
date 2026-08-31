# docs/red-proofs.md, tier 1: "the capability floor admits everyone" (P08.S05a, /pending 338, v1.117.290)
#
# The defect: SpeaksNamedRefusals stops guarding its own THRESHOLD. The floor is looked up in
# sessionALPN, so if alpn2 ever leaves that list the threshold ranks 0 and `rank >= 0` is true for
# every peer — including one negotiating nothing. A two-byte named refusal then goes to a nib/1
# peer, which reads it as a document mismatch and prints a tampering verdict caused by a version
# skew: the D32 violation the `named` gate exists to prevent.
#
# This is the fail-OPEN the floor introduced. The equality it replaced degraded the other way.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAnUnknownProtocolCannotBeGrantedTheCapability -count=1"
EXPECT="was GRANTED named refusals"
