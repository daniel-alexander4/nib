# docs/red-proofs.md, tier 1: "a non-canonical record verifies" (P07.S02, v1.117.153)
#
# The defect: Verify stops refusing a record whose stored form is not the form its own commitment
# binds. rosterPreimage hex-decodes fingerprints and renders Expires as RFC3339 seconds, so BOTH
# axes are folded away — two byte-different records carry one valid ConvenerSig. Measured: an
# uppercased roster entry convened green and was then refused by the invited party's MatchesRecord
# with a message printing two strings that differ only in case.
#
# Each arm of the test asserts the mutation does NOT break the signature before asserting the
# refusal, so the malleability is proved rather than assumed.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAVerifiedRecordIsCanonical -count=1"
EXPECT="want ErrNotCanonical"
