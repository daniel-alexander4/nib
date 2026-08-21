# docs/red-proofs.md, tier 2: "A `:empty::after` message put back into `web/style.css`"
#
# The defect: an empty-state message returns to the stylesheet as generated content, where it
# cannot be selected or copied and is announced inconsistently by assistive tech.
TIER="tier 2 — the jsdom suite"
PROVE="node --test test/jsdom/theme.test.mjs"
# EXPECT is the token the real assertion prints. Not the exit status: a deleted test file also
# exits non-zero, and that is what this harness used to accept as proof.
EXPECT="empty-state message"
