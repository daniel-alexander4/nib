# docs/red-proofs.md, tier 1: "rolemap self map is not a cycle" (/pending 548, 2026-09-16)
#
# The defect: `/RoleMap << /LI /LI >>` is read as a fixed point and not as a cycle, which is what
# `structure.go` asserted in prose until this row existed. veraPDF FAILS ua1 7.1-6 on the two `LI`
# elements of `7.1 General/7.1-t06-fail-a.pdf` and passes the other twelve — a corpus file the
# `/pending 548` entry did not know about, found by the standing corpus run as a FALSE PASS.
#
# It is the arm a rule reading `standardType`'s verdict alone cannot reach: the element still types,
# so neither `unresolved` nor an empty `standard` reports it.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestASelfMappedTypeIsCircularAndStillTypes -count=1"
EXPECT="a self-map is a circular mapping"
