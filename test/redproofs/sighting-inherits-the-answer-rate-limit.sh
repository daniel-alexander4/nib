# docs/red-proofs.md, tier 1: "The link sighting is reported below the answer rate limit"
# (P07.S05e, v1.117.207)
#
# The defect: the DHT hold's renewal hook sits BELOW `answerLoop`'s `hopAnnounceWindow` gate
# instead of above it. It reads as tidy — one place, after the decision — and it silently gives
# observing the period of announcing. That gate exists so a re-dial cannot stack a second
# announcer, which is a rule about ANNOUNCING; sightings arrive at 2/s and answers at most once
# per window, so the hold would stop renewing during exactly the stretch in which the peer is
# most present on the link. Two different rates over one stream, and one gate over both makes
# the second inherit the first.
#
# The guard asserts sightings EXCEED answers, not a count of either: a count is satisfied by
# both numbers being equal, which is the defect.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheSightingIsReportedBeforeTheAnswerRateLimit"
EXPECT="the hook is BELOW the answer rate limit"
