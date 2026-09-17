# docs/red-proofs.md, tier 1: "an OCR'd scan's page image is never artifacted" (/pending 514, v1.133.11)
#
# The defect, verbatim as it stood from P06.S06 to /pending 514: the drawing reader classified every
# `Do` as a form XObject, so no door ever saw the page image an OCR'd scan draws. `TagOCRLayer`
# returned `tagged=true` and wrote `/MarkInfo /Marked true` over a picture no element covered, and
# veraPDF failed the result on ua1 7.1 t3 at `/Im0 Do` — as did `nib ua`, which has always read this
# correctly and was the only thing telling the truth about it.
#
# **What could not see it, which is why it survived a whole phase.** `/pending 495` built the claim
# door's guard on `unmarkedTextRuns`, and a pure scan has NO text: the guard answered 0 and let the
# claim through. `internal/uacheck`'s oracle corpus — the one thing that asks veraPDF per clause per
# document — deliberately hosts its OCR layer on a TEXT page rather than a picture, so law 5 had never
# put this shape to the oracle at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAnOCRdScanDeclaresItsPageImageAnArtifact -count=1"
EXPECT="an OCR'd scan fails 7.1 t3"
