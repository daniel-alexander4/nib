# docs/red-proofs.md, tier 1: "the content anchor runs on signed arrivals too"
# (P08.S03, C04, v1.117.328)
#
# The over-correction, and the reason the anchor is conditional at all. `ContentDigest` covers each
# page's `/Annots`, a visible signature adds a widget annot, and the production path signs visibly —
# measured at P07.S02 — so from the first signature onward a document in flight legitimately does
# NOT hash to its record. An unconditional check refuses every honest hop from 2 on, which is the
# resumption case the ceremony exists for.
#
# **This row exists because the first cut of the test could not see it**: both fixtures were
# unsigned, so making the anchor unconditional left the whole test green. The signed arm was added
# with an appearance image, because an INVISIBLE signature does not move the digest either and the
# first attempt at that arm was itself vacuous.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnUnsignedArrivalIsAnchoredToItsOwnRecord -count=1"
EXPECT="refused a SIGNED arrival"
