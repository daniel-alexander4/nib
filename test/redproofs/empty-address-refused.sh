# docs/red-proofs.md, tier 2: "The ladder is unreachable — an empty address is refused" (P05.S12, v1.117.103)
#
# The defect: sessionInit() refused to POST a co-sign without a typed address
# (`if (!address) { toast("Enter the peer's address"); return; }`), so the shipped LAN
# tier — and, for an invited ceremony, the DHT — could never be reached from the product.
# The manual address was not merely undemoted; it was the only path a user had.
#
# What it costs: the whole traversal ladder D9 makes the default is dead code from the
# UI's point of view. A user whose peer is armed on the same LAN is told to type an
# address they have no way to know, for a peer Nib could have found by browsing.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ladderdefault.test.mjs"
EXPECT="the empty-address refusal is back"
