# docs/red-proofs.md, tier 2: "the signature panel draws nothing on a ceremony document"
# (P07.S05a, v1.117.178)
#
# The defect: `openSigDetails` begins `if (!signers.length) return;`, so even with the button
# visible the panel never draws for a **convened but unsigned** document — the case C18 is
# most about, two obliged signers and none of them signed.
#
# **The second of two doors** (see `ceremony-panel-unreachable-unsigned`), and the one that
# would survive a fix applied only to the button: the control would be offered and clicking
# it would do nothing at all.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="the panel did not report the ceremony's completeness"
