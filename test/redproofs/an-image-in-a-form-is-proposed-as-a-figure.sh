# docs/red-proofs.md, tier 1: "An image a form XObject draws is proposed as a Figure, which the commit cannot bracket (ADR-122)"
#
# The defect: the refusal for an image drawn by a form is gone, so a Figure is proposed for an operator that is in another stream.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestWhatIsNotAPictureToDescribeIsNotAFigure"
EXPECT="proposed 1 figure(s)"
