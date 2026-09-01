# docs/red-proofs.md, tier 1: "a failed durable write is reported as success" (/pending 343, v1.117.300)
#
# `Store` still attempts the write and discards its error. The document is delivered, the peer is
# satisfied, and the signer is never told their machine kept nothing.
#
# **It is the reason the interface was widened at all.** `Store` returns an error so `Receive` can
# turn it into a `persistError` and D24's "signed but not saved" becomes a sentence the signer sees;
# before P08.S02 the only channel was a `log.Printf` into a stderr a double-clicked launch sends
# nowhere. A swallowed error puts it back there, and every existing test stays green because the
# delivery itself is unaffected.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheContributionReachesDiskInsideStore -count=1"
EXPECT="Store reported success while its durable write had failed"
