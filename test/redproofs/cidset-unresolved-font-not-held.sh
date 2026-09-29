# docs/red-proofs.md, tier 1: "7.21.4.2 t2 passes beside a font it never read" (/pending 722, v1.169.7)
#
# The defect: checkCIDSetsComplete skips an unresolved font instead of holding a refusal for it, so one
# resolved CID font with an exact /CIDSet answers Pass over a font pdfcpu dropped and nib never read.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run '^TestAnUnresolvedFontRefusesTheCIDSetClauseBesideOneThatResolved$' -count=1"
EXPECT="text in a font that does not resolve reports pass"
