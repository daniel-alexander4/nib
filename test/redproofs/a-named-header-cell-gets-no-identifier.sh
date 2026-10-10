# docs/red-proofs.md, tier 1: "A header cell a data cell names is given no /ID (ADR-119, v1.202.0)"
#
# The defect: a headers edit writes the identifier into the cell's /Headers and into the /IDTree and never onto the header
# cell itself, so the cell names a header no TH carries — 7.5 t2 fails, in nib and in veraPDF.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -count=1 -run TestATableCorrectedByCellEditsIsJudgedAsVeraPDFJudgesIt"
EXPECT="every data cell naming its headers: 7.5 t2 reports"
