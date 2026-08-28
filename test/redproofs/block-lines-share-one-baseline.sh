# docs/red-proofs.md, tier 3: "Five attestation lines on one baseline" (/pending 305, v1.117.214)
#
# The defect: `fillText(ln, pad, pad + i * lineH)` loses its `i * lineH`, so every line is drawn
# at the same y and the block renders as one illegible smear of overprinted text.
#
# Measured under the patch: the block's five equal strips carry [6927, 0, 0, 0, 0] dark pixels.
#
# The strips are why this row reads cleanly. The first version of the check clustered contiguous
# inked rows into bands and expected one per line; against the SHIPPED renderer it reported six
# for five, because "Signer: Nib User" ends its body at row 72 and its 'g' descender resumes at
# 75. Tuning that threshold until it answered five would have been tuning until green, so the
# check was replaced by one that never mentions `pad` or `lineH` — equal strips, each of which
# must carry ink.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="not laid out one line per row of the block"
