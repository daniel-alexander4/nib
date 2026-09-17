# docs/red-proofs.md, tier 1: "A carried note goes through its page's own /Rotate" (ADR-045)
#
# The defect: `placementMatrix` reads the form content's rotation prefix and then discards it.
# `ContentBytesForPageRotation` prepends that `cm` INSIDE the form for a page carrying `/Rotate`, so
# form space is not page space and an annotation rect — always in unrotated page space — has to go
# through it. Measured: the note lands at (246.5, 637.5) instead of [550.5 549 560.5 559], off a
# 595-point-high sheet entirely.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestANUpCarriesANoteThroughAPagesOwnRotation -count=1"
EXPECT="leaves the rect in unrotated page space"
