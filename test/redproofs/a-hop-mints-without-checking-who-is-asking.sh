# docs/red-proofs.md, tier 1: "only the convener can call the next party" (P01.S02b, v1.128.59)
#
# The defect: `/api/ceremony/hop` stops asking whether this machine convened the ceremony, so any
# party who holds a mirror — and EVERY party writes one — reaches the mint and gets another party's
# full-strength channel secret plus a dial in their name.
#
# **The mint door checks nothing about who is asking**, which is why the rule lives at the door
# instead: `convenerInvitationFor` resolves the convener from the record, re-hashes the roster and
# returns the per-party secret. Before P01.S02b every gate was at `handleCeremonyInvites` and the
# second caller had no deadline check at all — ADR-005's "1 of 6" shape at 1 of 2.
#
# The same mutation also drops the HOP BUDGET, which is the other half of the rule: a hop that
# starts now and finishes after `Expires` asks somebody to consent to a signature on a proceeding
# that has already ended.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheHopRouteRefusesBeforeItDials -count=1"
EXPECT="a party who did not convene answered"
