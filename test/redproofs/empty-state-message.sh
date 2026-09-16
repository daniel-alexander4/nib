# docs/red-proofs.md, tier 2: "A `:empty::after` message put back into `web/style.css`"
#
# The defect: an empty-state message returns to the stylesheet as generated content, where it
# cannot be selected or copied and is announced inconsistently by assistive tech.
TIER="tier 2 — the jsdom suite"
PROVE="node --test test/jsdom/theme.test.mjs"
# EXPECT is the token the real assertion prints. Not the exit status: a deleted test file also
# exits non-zero, and that is what this harness used to accept as proof.
#
# **And not the test's TITLE (/pending 505).** It was "empty-state message", which is in the title
# `no empty-state message is generated content` — and node's runner prints every title, passing
# or failing. So any other red in this file satisfied the token while this test passed.
# `TestNoRedProofTokenIsItsTestsName` refuses that shape for every row.
EXPECT="these rules put a message in generated content"
