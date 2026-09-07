# docs/red-proofs.md, tier 2: "an absent note is rendered as a negative" (P02.S04, v1.128.12)
#
# The defect: the card defaults a missing note to `{presented: false}` and tells the user the
# spoken check did not appear. Absence means UNKNOWN — an older build, a failed write, a hop that
# has not happened — so this accuses a party of skipping a check they may well have performed,
# silently, on every ceremony that predates this version.
#
# **The first mutation for this row THREW**, which is why the row's patch is written the way it is.
# Reading `c.verification.presented` with no note is a TypeError; the card render unwound and the
# absence assertion passed for the wrong reason. The defect a user would actually ship is a
# DEFAULT, not a crash, and only that shape reaches the assertion.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/spokencheck.test.mjs"
EXPECT="Absence is UNKNOWN"
