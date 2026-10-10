# docs/red-proofs.md, tier 1: "A table attribute edit copies the attribute object and writes nothing into it (ADR-119, v1.202.0)"
#
# The defect: the one writer of a Table attribute copies the attribute object and never sets the key, so on a cell that already
# has a Table attribute object a span, a scope and the headers are accepted and none is written.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestACellEditNeverWritesThroughASharedAttributeObject"
EXPECT="want 2 and 1, both Column"
