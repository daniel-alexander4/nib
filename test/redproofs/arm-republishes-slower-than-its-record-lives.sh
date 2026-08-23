# docs/red-proofs.md, tier 1: "the arm republishes slower than its record lives" (/pending 269,
# v1.117.135)
#
# The defect: no guard on the arm side at all. candidatelife_test was right and entirely about the
# DIALLING side, so when P05.S09b gave the receive arm a 30-day window its record died 8 minutes in
# with every assertion green. The clause deliberately is NOT about the record's life: MaxCandidateLife
# is a reader-side ceiling every peer enforces, so asserting an expiry that covers a 30-day arm is
# unsatisfiable — the mirror image of a vacuous green. It is about COVERAGE.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnArmedSideStaysFindableForItsWholeWindow -count=1"
EXPECT="un-findable in the gap"
