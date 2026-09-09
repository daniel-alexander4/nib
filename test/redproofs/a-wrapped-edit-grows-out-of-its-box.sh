# docs/red-proofs.md, tier 1: "a wrapped edit grows out of its box" (P01.S02, v1.128.65)
#
# The plan said the overflow ladder should "wrap within the box". Measured, that is not
# available in the common case: two lines of 12pt Helvetica occupy 27.74pt against the 18pt a
# 20pt-tall box leaves after the anchor inset, and pdfcpu's position:bl anchors the block at
# the BOTTOM — so the extra lines grow UPWARD, over whatever sits above. On a text document
# that is the previous line, which is exactly the content a cover-and-replace edit is sitting
# beside.
#
# A box drawn around one line of text has room for one line. Wrapping it unconditionally
# trades a horizontal overrun for a vertical one, and the vertical one is worse: the
# horizontal case runs into the margin, the vertical case runs into other text.
#
# The gate is a measurement, not a heuristic — lines x mdpdf.CoreLineHeight against the box's
# own height — and the guard drives BOTH sides of the boundary with the same text, so a wrap
# that ignored height entirely cannot pass by picking a friendlier fixture.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run 'TestWrapIsGatedOnMeasuredVerticalRoom|TestEachFitOutcomeIsReachableAndDistinct' -count=1"
EXPECT="want \"overran\""
