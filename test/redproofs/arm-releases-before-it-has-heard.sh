# docs/red-proofs.md, tier 1: "An arm that is watching but has heard nothing YET releases"
# (P07.S05e, v1.117.207)
#
# The defect: `holdDHT` treats "no sighting" as "nothing is on the link", collapsing two states
# that are not the same — never heard YET, and not looking at all. Announcements arrive at 2/s
# and the base wait is two seconds, so whether an arm publishes becomes a race between its
# answer loop starting and the feed's first wait ending: a first sighting landing at 2.1 s
# arrives after the record is already on the public DHT.
#
# It is also what makes the acceptance clause true as written — an arm with nothing on the link
# reaches the DHT within `lanFirstBudget`, not within the base.
#
# The dial side sets neither field, so it must stay untouched; the test's second arm is that
# control, because a hold that applied everywhere is an outage rather than a fix.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnArmThatIsWatchingButHasHeardNothingStillHolds"
EXPECT="has not heard its peer YET"
