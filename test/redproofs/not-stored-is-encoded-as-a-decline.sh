# docs/red-proofs.md, tier 1: "a failed write is reported to the sender as a decline" (P08.S05a, v1.117.290)
#
# The defect: refusalAck stops mapping ErrNotStored, so a receiver whose disk failed sends either
# nothing (an EOF the sender reads as a transport fault) or the decline byte — a false statement
# about a person who said yes, which is the exact collapse ackTimedOut was added to undo one gate
# earlier.
#
# The check is the encode door: ErrNotStored must produce its own one-byte receipt.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestNotStoredIsItsOwnSentinel -count=1"
EXPECT="want the single byte"
