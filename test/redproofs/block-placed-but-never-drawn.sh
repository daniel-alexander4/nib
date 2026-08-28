# docs/red-proofs.md, tier 1: "The block is placed and never drawn" (P07.S06, v1.117.210)
#
# The defect: `Contribute` drops the appearance, so the signature is applied invisibly. Every
# geometric check still passes — the placement is computed, the rect is inside the page, the pages
# are allocated — and `sign.Verify` reports an invisible signature exactly as it reports a visible
# one, so the document is valid and the block is simply not there.
#
# This is the positive control D25's clause asks for, generalised one level: the clause says a
# raster cannot distinguish "off the page" from "never drawn", and the same holds of placement
# ARITHMETIC, which cannot distinguish "placed correctly" from "not placed at all". Without this
# row every other placement test in the file would stay green against a no-op.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestABlockIsActuallyDRAWNWhereItWasPlaced"
EXPECT="never drawn"
