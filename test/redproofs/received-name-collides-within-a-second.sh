# docs/red-proofs.md, tier 1: "two documents from one peer in one second collide" (/pending 342,
# P08.S05d, v1.117.317)
#
# The defect: `receivedName` back to naming a file from the peer's label and a one-second
# timestamp alone. That is what shipped, and it DESTROYED documents: `saveReceived` writes with
# `atomicfile.WriteDurable`, which renames over whatever is there, so the second arrival
# overwrote the first — after the sender had been told `ackOK`, so neither side could know.
#
# It was measured rather than argued. `ceremonyrepro.sh`'s two transfer legs run back to back and
# the probe showed both at the identical path:
#
#   PROBE[tcp]:  before=0 after=1  incoming/alice-20260831-110425.pdf
#   PROBE[quic]: before=1 after=1  incoming/alice-20260831-110425.pdf
#
# The patch keeps the digest COMPUTED and drops it from the name, so the mutation compiles. A
# version that deleted the line left `crypto/sha256` unused — the "did not compile" outcome this
# harness names separately from a real red, hit while recording this very row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTwoDocumentsFromOnePeerInOneSecondDoNotCollide"
EXPECT="share the name"
