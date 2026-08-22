# docs/red-proofs.md, tier 3: "Retry on the key-missing screen never retried" (sweep 10, v1.117.46)
#
# The defect: the `key-missing` recovery check sits AFTER the key-mode block in the auth form's
# submit handler. That state hides `#keyChoice` and never populates `keySelect`, so
# `selectedKeyMode()` returns its default `use`, `keySelect.value` is "", and the handler
# returns early with "No key selected." — an error about a control the user cannot see. The
# Retry button never re-reads the status, so the documented way out of a misplaced key is dead
# and the repoint button beside it is the only one that works.
#
# **Tier 3 and not tier 2**: the M2 flow this covers is a click-through, and its original defect
# was an overlay at z-index 200 swallowing clicks. jsdom has no hit-testing, so every
# interaction in the test is a real Playwright click whose actionability check IS the assertion
# that tier adds. This row is slow (it builds nib and drives Chromium) and that is the cost of
# proving a front-end flow rather than a function.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="Retry reported an error instead of re-checking the status"
