# docs/red-proofs.md, tier 1: "an unreadable record reads as 'I never signed'" (/pending 320, v1.117.273)
#
# The defect: `persistedFor` collapses every `ReadMirror` error to nil again, so a permission
# problem, an I/O fault, a damaged mirror and a version skew all give the same answer as a genuine
# miss. `coSignExchange` then falls through to `Confirm` and mints a SECOND, differently-timestamped
# signature from one identity — what D24 forbids and what C01 counts on the artifact.
#
# The check drives three outcomes separately — absent is a MISS, stored is a HIT, unreadable is
# UNKNOWN — because the danger of the fix is the opposite error: an over-broad "unknown" refuses
# every FIRST hop, since a machine that has never signed has no mirror at all. That is not a
# hypothetical; it is what the first cut did, and two ceremony tests caught it. The absent arm is
# that regression pinned, and it stays GREEN under this patch, which is what proves the three arms
# are not one wearing three hats.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnUnreadableStoredContributionIsUnknownAndNotAMiss -count=1"
EXPECT="an unreadable record reported a MISS"
