# docs/red-proofs.md, tier 1: "a park survives the fix that cleared it" (/pending 349, v1.117.309)
#
# The scoping is removed from the fixed-park arm so it runs over every entry, including design
# judgements it cannot adjudicate — and it then reports fixes that are not fixes.
#
# **The arm exists because the Go scan lacked what the JS one has had since it was written.**
# `test/jsdom/published.test.mjs` is what told this sweep that `sessionStatus.diagnosis` had gained
# a reader; the Go side — the scan that covers `internal/server` at all — had only the
# missing-field arm, so a park whose defect was FIXED sat on looking like coverage and would
# silently re-park the field the day somebody deleted its reader.
#
# The scope is `/pending`-referencing entries, and it is the honest one: those are claims about a
# tracked defect, where the close must be visible. The matcher is deliberately loose — a bare word
# in a JS reader, a mention in the shape's own defining package — which is safe in the direction the
# main loop uses it and unsafe in this one. Measured on the first unscoped run: three flagged, two
# of them correct as they stood.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryPublishedObservableHasANamedReader -count=1"
EXPECT="now HAVE a reader"
