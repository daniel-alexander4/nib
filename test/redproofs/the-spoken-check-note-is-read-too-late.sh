# docs/red-proofs.md, tier 1: "the spoken check note is read too late" (P02.S04, v1.128.12)
#
# The defect: the note is read AFTER `ReadStored`'s LoadState branches instead of before them.
#
# It is invisible on a healthy ceremony and wrong on exactly the population D5 is about. A party who
# has accepted and not yet signed holds no `record.json`, so `ReadStored` returns at
# `LoadAbsent` — and their hop is the one a reader most wants to ask about, because it has not
# happened yet. Moving one line hides the answer for every ceremony still in flight.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheNoteSurvivesAnUnreadableRecord -count=1"
EXPECT="carries no spoken-check note"
