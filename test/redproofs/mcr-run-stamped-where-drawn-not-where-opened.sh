# docs/red-proofs.md, tier 1: "a run stamped where drawn, not where opened" (P02.S08, v1.129.144)
#
# The defect: `currentStm` returns the walker's CURRENT stream rather than the stream the innermost
# in-force marked-content sequence was OPENED in. ISO 32000-1 Table 324 defines `/Stm` as "the content
# stream containing the marked-content sequence", and `runWalker.mcStack` deliberately spans the form
# boundary — a `BDC` around a `Do` tags what the form draws. So a sequence opened on the page whose
# glyphs land inside a form belongs to the PAGE's stream, and stamping the run with the form files its
# text under a stream no `/Stm` will ever name.
#
# Invisible to every n-up fixture, because a carried n-up puts the `BDC` inside the form and the two
# readings coincide. It needs a document where they differ, which is what
# `TestASequenceOpenedOnThePageKeepsThePagesStream` builds.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestASequenceOpenedOnThePageKeepsThePagesStream -count=1"
EXPECT="it was stamped with the stream it was drawn in"
