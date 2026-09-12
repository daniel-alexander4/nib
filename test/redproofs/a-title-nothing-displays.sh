# docs/red-proofs.md, tier 1: "a title written, and nothing told to display it"
# (PLAN-accessibility.md P03.S01, v1.129.30)
#
# The defect: `SetTitle` writes the XMP packet and the Info dict and never sets
# `ViewerPreferences /DisplayDocTitle`. The document passes any check for a title's PRESENCE
# — 7.1 t8 and 7.1 t9 both clear — and fails 7.1 t10, which is the clause that makes the
# title reach a reader. A viewer left on its default shows the FILE NAME in its chrome, so
# the symptom is a document that looks fine and whose title is never seen.
#
# Recorded because it is the half of the floor that has no visible consequence in a test that
# only reads back what it wrote: two of three keys is exactly the state SetTitle's own doc
# comment calls worse than none.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestSetTitleWritesTheWholeCatalogFloor"
EXPECT="a title nothing displays"
