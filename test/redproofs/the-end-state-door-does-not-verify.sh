# docs/red-proofs.md, tier 1: "the end-state door does not verify" (/pending 380, v1.128.27)
#
# The defect: `OpenEndState` decrypts and returns without checking the object against the anchor.
# The bytes at that target come off the public DHT and are written by whoever reached it first, so
# a correctly sealed, correctly signed end state for ANOTHER proceeding would open here — and a
# party would be told their ceremony had ended on the strength of somebody else's decline.
#
# **Recorded because it SURVIVED the first mutation pass.** The happy-path test proves an object
# round-trips and says nothing about what the door refuses, which is the door's whole job.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAPlantedEndStateIsRefused -count=1"
EXPECT="for a DIFFERENT proceeding opened against this invitation"
