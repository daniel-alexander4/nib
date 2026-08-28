# docs/red-proofs.md, tier 3: "The signature block, drawn white on white" (/pending 305, v1.117.214)
#
# The defect: `renderAttestation` strokes its frame and writes its text in '#fff' instead of
# '#000'. The block is then a white rectangle on white paper — placed, signed, and invisible.
#
# **This is the failure /pending 302 named and could not reach.** `blockink.test.mjs` supplies
# its own opaque magenta field on purpose, because a known colour is what makes "is this pixel
# the block's" a question about the pixel; it therefore says nothing about what the PRODUCT
# draws. And tier 2 cannot reach it either — jsdom has no canvas, so `ladderdefault.test.mjs`
# stubs `getContext` with a no-op `fillText` and `toBlob` with a one-byte PNG, which is honest
# about proving only that the POST fires.
#
# Measured under the patch: 0 dark pixels anywhere in the block's border band.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="drew nothing visible on it"
