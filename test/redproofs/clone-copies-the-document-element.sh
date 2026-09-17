# docs/red-proofs.md, tier 1: "A duplicated page carries its own grouping elements" (/pending 529)
#
# The defect: `cloneRoot` no longer stops at a `/Document`, so duplicating the only page of a
# one-page document copies the `/Document` element with it and the file comes out declaring TWO.
#
# A `/Document` is "a complete document" (ISO 32000-1 table 323/333). Repeating a page repeats a
# division of content, not the document the content is in. The stop is self-limiting — a
# `/Document` over a multi-page source spans pages and stops the climb on the page test anyway — so
# this is the one shape that can see it.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestADuplicatedTableIsATableAndTheOriginalKeepsItsWIDTH -count=1"
EXPECT="holds 2 /Document elements"
