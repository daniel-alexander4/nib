# docs/red-proofs.md, tier 3: "An appearance that fills half its own rectangle" (/pending 302, v1.117.212)
#
# The defect: `stackPlacement`'s gap goes from 12 pt to -42 pt, so consecutive blocks overlap
# by half their height. The measured block's rect is then half-covered by the block below it,
# which was already painted — so only 51.9% of its own rectangle is ink this contribution put
# there.
#
# It stands in for the class the structural guard cannot reach at all: an /AP stream whose
# BBox or Matrix clips the appearance to part of its rectangle. That produces a block that is
# placed, drawn, and unreadable, and every check on the file's structure calls it correct.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="of its own widget rect"
