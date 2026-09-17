# docs/red-proofs.md, tier 1: "A carried note goes through the matrix its content did" (ADR-045)
#
# The defect: `attachNotes` writes the SOURCE rect onto the sheet instead of the transformed one.
# A tile check cannot see this — note 1's untransformed rect is still inside the top tile and still
# on the right sheet. The icon box is 20 points square in page space and the placement scales by
# 595/842, so a carried note is 14.13 points square and an untransformed one is 20.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestANUpsNoteIsScaledAndNotMerelyMoved -count=1"
EXPECT="copied without the matrix"
