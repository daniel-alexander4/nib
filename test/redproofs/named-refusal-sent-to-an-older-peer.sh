# docs/red-proofs.md, tier 1: "a named refusal is sent to a peer that cannot read one" (P07.S03a, v1.117.162)
#
# The defect: `refusalAck` writes the named refusal frame regardless of what the peer negotiated.
#
# **This is the D32 violation the whole negotiation exists to prevent, and it is not a courtesy
# failure.** A build predating this version maps a ONE-BYTE reply through `refusalFor` and
# otherwise falls to `if !bytes.HasPrefix(final, mySignedPDF)` — so an unfamiliar frame reaches the
# prefix check and that user is told *"returned document is not the one sent this session"*: a
# verdict about the counterparty, reading as a replay or a tamper, produced by a version skew.
#
# The guard also asserts the other half, which stops the fix becoming "send nothing to anybody":
# the two refusal classes that PREDATE the negotiation still cross to an older peer.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestARefusalIsOnlySentToAPeerThatCanReadIt -count=1"
EXPECT="was sent to a peer that did not negotiate it"
