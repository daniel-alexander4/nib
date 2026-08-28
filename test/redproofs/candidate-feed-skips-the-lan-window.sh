# docs/red-proofs.md, tier 1: "The candidate feed fetches before the link has answered"
# (P07.S05d, v1.117.203)
#
# The defect: `feedCandidates` reaches the DHT with no hold at all. This is the half that makes
# the lazy bootstrap worth anything — `publishLoop` had always taken `browseWindow` as its first
# delay and said why, and the FETCH did not. A bootstrap deferred to first use with an unwindowed
# fetch immediately after it moves the first off-link packet by microseconds, so the two changes
# are one fix and this row is why.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheFeedDoesNotTouchTheDHTInsideTheLANWindow"
EXPECT="reached the DHT"
