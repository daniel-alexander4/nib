# docs/red-proofs.md, tier 1: "a stamped width counts the newline as a glyph" (P01.S01,
# v1.128.64)
#
# mdpdf.CoreWidth measures a string; pdfcpu SETS one line per "\n" and the form it emits is
# as wide as the widest line. Measuring the whole string counts the newline as an ordinary
# character and sums the lines end to end: "one\ntwo" measures 50.69pt against an emitted
# 20.02pt, and "a\nlonger second line" 116.05 against 97.38.
#
# This is not a latent concern. P01.S02's wrap outcome works BY inserting newlines, so a
# newline-naive door is wrong at the exact moment the next slice starts using it — and the
# plan's own acceptance clause for this slice ("matches the AFM value") is green against the
# naive version, which is why the oracle here is pdfcpu's emitted BBox instead.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestStampWidthMatchesEmittedBBox -count=1"
EXPECT="pdfcpu emitted a"
