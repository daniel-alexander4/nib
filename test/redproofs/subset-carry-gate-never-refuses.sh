# docs/red-proofs.md, tier 1: "The subset carry's output gate never refuses" (P02.S04b, v1.129.142)
#
# The defect: `subsetCarrying` ships whatever the carry produced instead of re-reading the written
# bytes and falling back to the honest loss. That gate is the slice's whole safety net — the reason
# every refusal inside `carryStructure` is free is that an incomplete carry is thrown away here.
#
# Measured before its reader existed: `if true` left `internal/pdfops` green (166 s, veraPDF included)
# AND `internal/server` green (199 s), because nothing in the repo produced an incomplete carry. The
# shape that does is the one `subsetCarrying`'s own header names: `clonePage` shares content streams,
# so a duplicated page whose content draws an MCID-bearing form XObject draws it twice, its marked
# content has two semantic parents, and one `/StructParents` key cannot name them both.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestTheOutputGateFallsBackToTheHonestLoss -count=1"
EXPECT="shipped a tagging claim"
