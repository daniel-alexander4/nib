# docs/red-proofs.md, tier 1: "a dict /K drops the whole tree" (PLAN-ua-coverage.md P02.S08, v1.129.144)
#
# The defect, verbatim as it was written: `rewriteKidsAsMCR` called `ctx.DereferenceArray(k)` and
# treated any error as fatal. pdfcpu's `DereferenceArray` type-asserts to `types.Array` and returns a
# wrong-type ERROR for anything else, so `/K << /Type /MCR /Pg 2 0 R /MCID 0 >>` — ISO 32000-1's own
# Example 2, and what LibreOffice and Word emit for a single-kid element — returned false, and
# `carryTagsThroughNUp`'s all-or-nothing rule then abandoned the ENTIRE carry and dropped the tagging
# claim through `honest`.
#
# **It shipped green and was caught in review.** Every fixture in `internal/pdfops` wrote `/K [0]`,
# the array form, so no test in the repo held the shape the spec's own example uses — while
# `structartifact_test.go:310-313` had been driving exactly that dictionary somewhere else all along.
# `TestEveryKShapeSurvivesTheCarry` is the reader that now covers all four spellings of `/K`.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestEveryKShapeSurvivesTheCarry -count=1"
EXPECT="the whole structure tree was dropped because the carry could not read that spelling of /K"
