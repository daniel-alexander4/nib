# docs/red-proofs.md, tier 1: "the block-height floor is raised" (/pending 286, v1.117.307)
#
# `maxBlockLines` goes from 8 to 20. Nothing breaks, no test about wrapping fails, and every
# over-long recital that used to be refused now convenes — which is exactly why this row exists.
#
# The constant is a LINE COUNT and the decision behind it was a FONT SIZE: the client sizes text at
# `min(lineH*0.7, 9pt)` on a 280x84pt block, so 8 lines is 6.65pt and 20 lines is **2.66pt**. Below
# about 6pt a signature block on a legal instrument stops being readable in print, and the documents
# are signed and distributed by the time anyone notices.
#
# The check asserts the PROPERTY rather than restating the number — a test that compared the
# constant to itself would pass under this patch.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheBlockHeightHonoursTheLegibilityFloor -count=1"
EXPECT="below the 6.5pt floor"
