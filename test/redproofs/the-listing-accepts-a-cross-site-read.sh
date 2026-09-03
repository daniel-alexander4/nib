# docs/red-proofs.md, tier 1: "a cross-site page can reach the ceremonies listing"
# (P06.S01, v1.117.334)
#
# The SAME mutation as its sibling row, graded on the other half — and this half is a live defect
# that P06.S01 closes rather than a hypothetical. `requireUnlocked` applies the CSRF check and the
# loopback-origin check to NON-GET methods only; `handleCeremonyInvites`' own doc comment says so,
# which is why that route is a POST. So behind that gate this listing had no origin check at all —
# and since P08.S06 a GET to it runs a close-out sweep, which moves ceremony directories and drops
# vault pins. A state-changing side effect on a GET, reachable from any page the user has open.
# `requirePublicLoopback` refuses `Sec-Fetch-Site: cross-site` before the handler runs.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheCeremoniesListingRefusesACrossSiteRead -count=1"
EXPECT="returned 200, want 403"
