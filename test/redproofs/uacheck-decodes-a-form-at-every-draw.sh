# docs/red-proofs.md, tier 1: "the checker decodes a form at every draw" (/pending 721, v1.169.6)
#
# The defect: decodedContent never answers from its cache, so a form XObject drawn N times is inflated
# N times — pdfcpu hands back a fresh stream dictionary at every dereference and caches nothing.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run '^TestAFormDrawnRepeatedlyIsDecodedOnce$' -count=1"
EXPECT="decoded it 50 times, want once — the decode is cached per object"
