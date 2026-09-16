# docs/red-proofs.md, tier 1: "A carry that deletes the /RoleMap" (PLAN-ua-coverage.md P02.S04b, v1.129.142)
#
# The defect: the carry drops `/RoleMap` along with `/IDTree`. It is not a hazard the way `/IDTree`
# is — a RoleMap holds NAMES, not object references, so it cannot re-anchor a removed page — which is
# exactly why nothing guarded it.
#
# Measured before its reader existed: adding this one line left the ENTIRE `internal/pdfops` suite
# green, veraPDF differential included. `subsetFixture`'s own doc names a RoleMap as one of the shapes
# the census document lacks, and the census genuinely has none — so obj 38 was the only RoleMap in the
# repo and nothing read it. The loss is not cosmetic: an element typed `/Para` means nothing without
# the map that says it is a `/P`, so a carried subset of a real LibreOffice or Word document would
# carry element types no reader can interpret.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestACarriedTreeKeepsItsRoleMap -count=1"
EXPECT="the carried RoleMap is"
