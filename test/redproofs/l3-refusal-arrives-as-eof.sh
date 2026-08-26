# docs/red-proofs.md, tier 1: "an L3 refusal reaches the initiator as EOF" (P07.S03a, v1.117.162)
#
# The defect: no named refusal is written at all — the state the product was in before this slice.
# `refusalAck` recognised exactly two classes; everything else returned `(0, false)`, wrote no ack
# frame, and the receiver closed. So every protocol refusal arrived at the initiator as
# `receive co-signed document: EOF`, which reads as a network fault and invites the retry a
# refusal must not invite. D23: never a hang, never a silent no-op.
#
# Driven END TO END over both transports, with a stimulus asserting that both ends negotiated the
# named-refusal protocol first — without it the assertion would pass on a transport where ALPN was
# never wired, forever.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAnL3RefusalReachesTheInitiatorByName -count=1"
EXPECT="want ErrNotYourTurn"
