# docs/red-proofs.md, tier 1: "An identifier enters a nested /IDTree and no /Limits is widened to it (ADR-119, v1.202.0)"
#
# The defect: the entry is written into the right leaf and no /Limits on the way down is widened, so a reader following
# /Limits never finds it. A write-and-read-back test cannot see this: pdfcpu's relaxed validator repairs a
# leaf's limits on every read.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAnIDTreeEntryWidensEveryLimitOnItsWayDown"
EXPECT="/Limits read"
