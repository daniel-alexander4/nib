# docs/red-proofs.md, tier 2: "the armed pill names the arm as what its date bounds" (/pending 366,
# v1.117.346)
#
# The defect: the shipped wording. `until` is the ARM's own bound, and for a ceremony arm that is
# `ceremony.MaxCeremonyLife` — thirty days — because an invitation carries no deadline
# (`/pending 247`). So a user arming for a two-day proceeding read "armed until" a date a month
# away: true about the arm, misleading about the proceeding, and indistinguishable on that pill.
# The proceeding's own deadline is the ceremonies panel's "Open until", a few pixels away.
#
# The assertion is over PROSE and is worth exactly that: it catches the wording being reverted or
# dropped, and cannot tell whether a reader understands it. Said in the test file too.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/armedbound.test.mjs"
EXPECT="states a date without saying what it bounds"
