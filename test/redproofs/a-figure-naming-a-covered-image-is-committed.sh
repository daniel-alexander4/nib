# docs/red-proofs.md, tier 1: "A figure whose operator a fresh read does not find uncovered is committed (ADR-122)"
#
# The defect: the commit no longer requires a figure's operator to be a drawing the fresh read found, so a stale proposal is written.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAFigureThatIsNotWhereItWasProposedIsStale"
EXPECT="want the stale refusal"
