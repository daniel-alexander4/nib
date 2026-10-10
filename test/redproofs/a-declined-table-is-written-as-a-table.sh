# docs/red-proofs.md, tier 1: "A table the reviewer says is not one is refused instead of written as paragraphs (ADR-121, v1.204.0)"
#
# The defect: the Table reviewed as P is not read as declined, so the only answers to a ruled form are a table or decoration.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestATableReviewedAsNotOneIsWrittenAsParagraphs"
EXPECT="a table and its rows keep their type"
