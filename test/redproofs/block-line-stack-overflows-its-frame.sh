# docs/red-proofs.md, tier 3: "The attestation runs into its own frame" (/pending 305, v1.117.214)
#
# The defect: `lineH` divides the block's usable height by `lines.length - 2`, so the line stack
# is half again too tall and the last lines run into the border and past the canvas, where they
# are clipped away.
#
# Measured under the patch: the block's lowest ink is at row 239 of its 240-row interior.
#
# **A gentler overflow does NOT reach this row, and the attempt is part of the record.**
# Dropping the padding from the divisor (`cv.height / lines.length`) leaves the last line's ink
# at row 227 of 240 — inside the frame, legible, and green. So this assertion catches an
# overflow that reaches the frame, and not every arithmetic slip that makes the stack too tall.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="running into its own frame"
