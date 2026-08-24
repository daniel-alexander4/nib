# docs/red-proofs.md, tier 1: "A pin derived from wire data" (L1, /pending 278)
#
# The defect: a non-test file in package `server` takes a wire type (discovery.Seen) and
# lets the value reach candidate.Fingerprint — the pin. L1 is "nothing learned from the
# network may influence WHICH peer is accepted; the pin comes from the vault".
#
# **This row is the reason the item existed.** docs/red-proofs.md cited
# `zz_l1fixture.go: redProofWireDerivedPin` as though it were durable, and no such file
# was ever in the tree — a throwaway recorded as a fixture, which is precisely what
# build/redproof.sh was written to make impossible. The fixture now lives HERE, inside
# the patch, where it is re-applied on every replay and cannot rot unnoticed.
#
# The guard is an AST taint analysis over NON-TEST files, so the fixture has to be real
# Go in the package rather than a test helper — that is why it is a whole added file.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestNothingWireDerivedReachesAPin -count=1"
EXPECT="sets candidate.Fingerprint from wire-derived"
