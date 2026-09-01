# docs/red-proofs.md, tier 1: "the durable write never reaches disk" (/pending 343, v1.117.300)
#
# `ceremonyID.Store`'s durable half is removed and the in-memory cache left in place, so a
# reconnect INSIDE this process still re-delivers and everything the old suite could see is
# unchanged. What is gone is the only copy that survives the process — D24's whole subject.
#
# **This is the row the source scan cannot be.** `l3_test.go` asserts that `Store`'s body contains
# `persistContribution(`; that scan stays green for any mutation that keeps the call, and green is
# also what it prints when the call writes nothing. Until this row the property had no runtime
# reader of any kind (three named searches, /pending 343).
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheContributionReachesDiskInsideStore -count=1"
EXPECT="does not exist"
