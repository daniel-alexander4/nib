# docs/red-proofs.md, tier 1: "the lang-only walk records drawing events — an inline image in a Type 3 glyph reads as uncovered content, a false FAIL of 7.1 t3 on an ordinary tagged page"
# (P04.S04, v1.148.0)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestADrawnPatternAndGlyphContributeNoSubjectAndNoEvent -count=1"
EXPECT="operator #28 \`BI … EI\` (inline image)\" — veraPDF makes no content item there"
