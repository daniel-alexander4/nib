# docs/red-proofs.md, tier 1: "the scan image is tagged instead of artifacted" (/pending 514, v1.133.11)
#
# The defect: the OCR door brackets its uncovered drawings with a marked-content sequence that carries
# an MCID rather than with `/Artifact`. The picture is then "real content" — and the MCID it is given
# belongs to a `P` element that already describes a WORD, so a reader following the tree is handed the
# page's whole image as part of somebody's sentence.
#
# **Nothing else catches it, which is the point of this row.** `/pending 514` chose `/Artifact` over a
# tagged element, and the thing that decides between them cannot be a conformance verdict: veraPDF's
# 7.1 t3 is `isTaggedContent == true || parentsTags.contains('Artifact') == true`, so BOTH shapes pass
# it, and the mutated document is measured passing it in this very run. Only the byte assertion sees
# the difference. That is why the test asserts the bracket and not merely the verdict.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAnOCRdScanDeclaresItsPageImageAnArtifact -count=1"
EXPECT="not bracketed as an artifact"
