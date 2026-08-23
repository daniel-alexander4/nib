# docs/red-proofs.md, tier 1: "a handler parses 128 MiB before checking the pin" (/pending 261, v1.117.126)
#
# The defect: a handler that works from POSTED bytes parses its whole multipart body and runs the PDF
# operation before resolving the document the result installs into. The law is not broken — the resolve
# at the commit refuses and nothing lands in the wrong document — so no status code can show this. What
# it costs is a full parse and a page operation on a document that is already gone. The guard checks the
# ORDERING (ADR-009: the door, not the text each site prints) and walks the package rather than a list,
# because the population is what changed: /api/assemble was the fourth member and had been excluded from
# the pinning inventory on a reason that was false.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAHandlerThatCommitsResolvesBeforeItReadsTheBody -count=1"
EXPECT="does not resolve the addressed document until line"
