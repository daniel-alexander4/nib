# docs/red-proofs.md, tier 1: "the narrow rect taken unconditionally" (P02.S08, v1.129.144)
#
# The defect: the per-stream RECT is read unconditionally while the per-stream TEXT keeps its guard,
# so a kid naming a stream the page walk never reached gets its text from the page index as designed
# and its box from a map that has nothing — `hasRect` silently goes false and every ancestor's union
# shrinks.
#
# **Found by a BLIND mutation pass** — a subagent shown the production code and none of the tests,
# asked for five mutations and ranked this one least likely to be caught. It was right: it survived
# every assertion the slice had, because text is asserted far more often than geometry and the
# fallback path is the one a test author believes is unchanged behaviour. The targeted pass could not
# have found it; the model that picks a mutation from the predicate is the model that wrote the test.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAKidNamingAnUnreachedStreamFallsBackToThePage -count=1"
EXPECT="The TEXT fell back to the page index and the BOX did not"
