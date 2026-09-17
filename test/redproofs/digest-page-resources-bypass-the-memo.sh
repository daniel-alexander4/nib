# docs/red-proofs.md, tier 1: "The stream memo never stores" (/pending 488)
#
# The defect: `decodeStream`'s store is disabled, so every object is decoded again on every
# sighting — the original O(pages x resources) cost, and 74.6% of the profile that opened
# /pending 488.
#
# This row exists because the SPEED half of that item needs a falsifiable check of its own, and a
# stopwatch cannot be it: a timing assertion on a shared machine fails when the box is busy and
# passes when it is quiet, which is the opposite of a regression test. The check counts decodes.
#
# **The first predicate written for it was vacuous, and applying a patch is what found that.** It
# asserted that decodes PER PAGE do not grow with the document — but without a memo each page
# decodes its own resources exactly once per resource name, which is a constant per page (2.0 at 15
# pages, 2.0 at 60). The defect is a multiplier on a constant, so the ratio could not see it and the
# test went GREEN with the patch applied. The predicate is the TOTAL now: 8 decodes at 15 pages and
# 8 at 60 with the memo, 30 and 120 without.
#
# **And the first PATCH was wrong in the mirror-image way.** It passed a nil memo from
# `hashPageResources`, which disables the decode COUNTER along with the memo — the counter hangs off
# the memo — so the check went red on its setup guard ("decoded no streams at all") rather than on
# its assertion. `redproof.sh`'s third outcome caught it: red, but not for its own reason. The
# defect is now inside `decodeStream`, where the counter still runs and only the storing stops.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestASharedFontIsDecodedTwiceAndNotOncePerPage -count=1"
EXPECT="the total is scaling with the page count"
