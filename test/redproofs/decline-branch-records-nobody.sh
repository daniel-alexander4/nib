# docs/red-proofs.md, tier 1: "the decline branch ends the ceremony and records nobody"
# (P08.S05e, v1.117.322)
#
# `endCeremony` had no test of any kind before this slice — `grep endCeremony --include=*_test.go`
# returned nothing repo-wide — so the one production site that writes the marker was pinned only at
# tier 4. With the write gone, `endedBy` returns "" forever, the walk skips nobody, and every unit
# test of the marker and of the walk stays green while the round goes back to spending its whole
# connect deadline on a party it cannot reach.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheDeclineBranchRecordsWhoEndedIt -count=1"
EXPECT="does not record WHICH party"
