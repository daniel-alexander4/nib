# docs/red-proofs.md, tier 2: "the signature panel cannot be opened on a ceremony document"
# (P07.S05a, v1.117.178)
#
# The defect: `updateBadge` hides the signature-details button on any document with no
# signatures — the right rule for a modal that LISTS signatures, and the wrong one for a
# **convened but unsigned** document, which has two obliged signers and nothing else to say.
# The server published `obliged: 2, signed: 0` and no user could open it.
#
# **This is one of TWO doors, and each closes the surface alone.** The other is
# `ceremony-panel-returns-early-unsigned`. They are separate rows because restoring either
# one on its own makes the panel unreachable again, so a single row would let the other
# regress silently.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="the details button is hidden on a ceremony document"
