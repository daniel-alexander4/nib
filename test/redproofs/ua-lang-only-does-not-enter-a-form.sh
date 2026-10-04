# docs/red-proofs.md, tier 1: "langOnly stops at a form XObject boundary — a form drawn from a pattern or a glyph procedure reads as semantic content"
# (P04.S04, v1.148.0)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestADrawnPatternAndGlyphContributeNoSubjectAndNoEvent -count=1"
EXPECT="form-in-pattern: a drawing operator inside a nested stream reached the content events"
