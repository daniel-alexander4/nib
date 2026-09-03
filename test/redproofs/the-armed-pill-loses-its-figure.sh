# docs/red-proofs.md, tier 2: "the fix for the wording keeps C05's figure" (/pending 366,
# v1.117.346)
#
# The other direction, and it needs its own row because it is the failure mode the FIX could
# introduce. The date on that pill is C05's and is the whole reason a five-minute manual bound and
# a thirty-day ceremony bound are distinguishable at all — a wording fix that deletes it trades
# this ambiguity for the one it replaced. The mutation removes the figure and keeps the sentence.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/armedbound.test.mjs"
EXPECT="the title carries no date"
