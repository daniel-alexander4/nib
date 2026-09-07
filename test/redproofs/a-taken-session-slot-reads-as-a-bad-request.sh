# docs/red-proofs.md, tier 1: "a taken session slot reads as a bad request" (P02.S02, v1.128.6)
#
# The defect: `handleSessionArm`'s sentinel→status switch loses its `errSessionArmed` arm, so a
# second ceremony arm answers 400 instead of 409 — telling a user their request was malformed when
# what actually happened is that their machine is already listening.
#
# The mapping is new: extracting `armCeremonyHop` moved three status codes from the three points
# that produced them into one switch over sentinels. A named search over `internal/server/*_test.go`
# for `a session is already armed`, and for either of the other two messages, returned NOTHING — so
# the QUIC ceremony arm's refusals were asserted nowhere, and an extraction that swapped two of the
# three would have been invisible to the whole suite. This is the one of the three a Go test can
# reach; the other two need a socket that will not open.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestASecondCeremonyArmIsRefusedAsAConflict -count=1"
EXPECT="a second ceremony arm returned"
