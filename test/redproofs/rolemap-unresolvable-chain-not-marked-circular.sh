# docs/red-proofs.md, tier 1: "rolemap unresolvable chain not marked circular" (/pending 548, 2026-09-16)
#
# The defect: only the self-map arm sets `circular`, so a chain that comes back to a name already on
# the path resolves to nothing AND reports no cycle — the common case, and the one the corpus file
# `7.1-t05-fail-d.pdf` carries. The two arms are set in one place precisely so neither can be lost
# without the other being noticed.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestACircularRoleMapFailsTheClauseThatIsAboutIt -count=1"
EXPECT="nib holds the cycle and this is the clause it breaks"
