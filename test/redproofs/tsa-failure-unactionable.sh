# docs/red-proofs.md, tier 1: "A TSA failure nobody could act on" (sweep 11, v1.117.48)
#
# The defect: describeSignFailure is bypassed and runSign returns the raw library error, so an
# unreachable timestamp authority surfaces as `sign: failed to replace signature: failed to
# create signature: get timestamp: non success response (0)` — naming neither the authority
# the user typed nor the fact that nothing was signed. All three signing doors converge on
# runSign, so this is the one place to fix it (ADR-009).
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestAnUnreachableTimestampAuthoritySaysSoAndSignsNothing"
EXPECT="not identifiable as a timestamp problem"
