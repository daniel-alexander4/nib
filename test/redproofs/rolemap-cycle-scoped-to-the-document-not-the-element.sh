# docs/red-proofs.md, tier 1: "rolemap cycle scoped to the document not the element" (/pending 548, 2026-09-16)
#
# The defect: the clause is read as a statement about the `/RoleMap` dictionary — fail the file if
# any mapping loops — which is what its own words say and is not what it means. veraPDF's object for
# 7.1-6 is `PDStructElem`, measured: `7.1-t05-fail-d.pdf` scores 2 passed checks and 2 failed over its
# four elements. A cycle no element enters is a PASS, and a producer that left a dead private mapping
# behind would have been failed for it on every file.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestACircularRoleMapNoElementUsesIsNotAFailure -count=1"
EXPECT="veraPDF checks the element, not the dictionary"
