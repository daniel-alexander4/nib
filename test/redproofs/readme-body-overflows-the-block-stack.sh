# docs/red-proofs.md, tier 1: "The body grows down into the signature blocks" (P07.S08, v1.117.144)
#
# The defect: the refusal is made inert AND the body is grown past its envelope, which is the
# state this slice could have shipped by simply writing more prose. Measured: the shipped page
# is 31 lines with a last baseline of 315, and the two-party block stack tops out at 220 — so
# the budget is 6.8 lines and the honest N-party account was first costed at 12-20.
#
# The block appearance is an opaque white fill (web/app.js renderAttestation), so a collision
# does not overlap the trust text, it ERASES it. Nothing else in any tier can see this: the
# rendered position clamps, the extracted text is unchanged, and PageCount is a constant of the
# spec. It is deliberately pinned at TWO signers — D25 records that block 3 already overlaps on
# the unmodified tree, and re-pointing this at allocated signature pages is P07.S06.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestReadmeBodyClearsTheAttestationStack -count=1"
EXPECT="the signature-block stack starts at"
