# docs/red-proofs.md, tier 1: "ContentDigest is blind to an attached exhibit" (P07.S02, v1.117.153)
#
# The defect: the embedded-files axis is dropped from the digest — the state before P07.S02.
# Measured at the slice grill: an attached Schedule-A.txt reading "rent is 1000/mo" removed and
# re-added under the SAME filename reading "rent is 100000/mo" left ContentDigest byte-identical
# and CheckDocument nil. The exclusion was ARGUED ("tamper-evidence for everything else is what
# the signatures are for") and the argument fails in the pre-signature window, which is the only
# window this digest is checked in — the same refutation that folded /Annots in, one axis over.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestContentDigestCoversAttachedExhibits -count=1"
EXPECT="contents changed under an unchanged filename"
