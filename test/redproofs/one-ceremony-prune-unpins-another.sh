# docs/red-proofs.md, tier 1: "ending one ceremony unpins a peer another still needs" (P07.S02b, v1.117.157)
#
# The defect: `PruneCeremonyPeers` drops the whole pin as soon as one of its scopes matches —
# which is what a single `Ceremony string` forced. Measured on a probe before the fix: two
# `AddCeremonyPeer` calls for one fingerprint left `ceremony-A`, and pruning A removed the pin
# outright, so ceremony B's next arm refused an unpinned peer the user had never unpinned.
#
# The same counterparty across two matters is the ordinary case for this product's user, not an
# exotic one. `PinnedPeer.Ceremonies` is a set for this reason and `contentsVersion` moved with it.
TIER="tier 1 — go test"
PROVE="go test ./internal/vault/ -run TestTwoCeremoniesCanShareAPin -count=1"
EXPECT="ending ceremony A removed a peer ceremony B still needs"
