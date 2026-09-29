# docs/red-proofs.md, tier 1: "pdfops' form walkers decode a form at every draw" (/pending 742, v1.169.16)
#
# The defect: formWalkBudget.formContent never answers from its cache, so every walker that shares the
# budget re-inflates a form XObject at each draw — /pending 721's defect in pdfops.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run '^TestAFormDrawnRepeatedlyIsDecodedOnceByEveryWalker$' -count=1"
EXPECT="the run reader decoded a form drawn 50 times 50 times, want once"
