# docs/red-proofs.md, tier 1: "an edit that does not fit cannot be saved" (P01.S02, v1.128.65)
#
# The plan's third overflow outcome was to "refuse with a sentence naming what happened", and
# implemented as an HTTP refusal that is a data-loss-shaped bug rather than a safety rail.
#
# /api/bake is not the edit button. It is what EVERY save, print, flatten, export, PDF/A
# conversion and both signature paths run through — 24 call sites in web/app.js — and the
# client's own rule at the call site is that a bake which is not OK aborts the whole
# operation, because returning un-baked bytes would silently drop the user's covers, fields,
# stamps and notes. So a 409 here does not decline one edit: it makes a document carrying one
# over-long edit impossible to save, print or sign at all, with no way for the user to get
# their work out.
#
# Nib stamps what the user typed and reports on the response that it did not fit. The guard
# asserts the STATUS and the whole PDF first and the report second, in that order, because the
# order is the point.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestBakeReportsAnOverrunAndStillReturnsTheDocument -count=1"
EXPECT="every save runs through here"
