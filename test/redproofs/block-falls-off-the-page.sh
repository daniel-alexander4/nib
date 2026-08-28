# docs/red-proofs.md, tier 3: "A block that draws nothing a reader can see" (/pending 302, v1.117.212)
#
# The defect: `stackPlacement` puts every block 700 pt below where it belongs, so on an A4
# readme page every signature block falls off the bottom edge. The document is still valid,
# still carries six signatures, and every widget's /Rect is exactly what the placement
# computed — because the placement and the widget are the same number.
#
# **This is the row the whole file exists for.** P07.S06's structural guard,
# `TestABlockIsActuallyDRAWNWhereItWasPlaced`, stays GREEN under this patch: it compares the
# widget's rect to the placement's rect and they agree perfectly, both being wrong together.
# It says so at its own line — "it cannot see a block drawn white on white, or an /AP stream
# that positions its content outside its own BBox" — and this is the same blindness reached
# from the other side. Only rendering the page catches it.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="changed no pixel on the page it placed its block on"
