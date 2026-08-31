# docs/red-proofs.md, tier 1: "A file changed on disk is never reported" (/pending 333, v1.117.289)
#
# The defect: diskChanged answers false unconditionally. This is the defect as ORIGINALLY
# SHIPPED — a document records path and data and nothing describing the file, so the bytes read
# at open are served forever and a browser reload re-fetches the same copy.
#
# The check is the report door: /api/doc must say the file moved, because that field is the only
# thing the client can raise a banner from.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestDocReportsAFileChangedUnderneathIt"
EXPECT="still reports diskChanged=false"
