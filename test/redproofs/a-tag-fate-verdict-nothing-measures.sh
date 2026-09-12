# docs/red-proofs.md, tier 1: "every declared tag fate is the MEASURED fate" (P01.S03, v1.129.16)
#
# The defect: the verdict the byte count produced. `Rotate` declared `dropped` while it carries
# the whole tree — one of NINETEEN rows that said `dropped` about an operation that preserves
# tagging intact. The guard that existed then asked only "does this output lie?" and never
# compared a declaration to a document, so all nineteen sat in the table with nothing red.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestEveryDeclaredFateIsTheMEASUREDFate"
EXPECT="declares \"dropped\" and measures \"carried\""
