# docs/red-proofs.md, tier 1: "The stream memo keyed on length rather than object number" (/pending 488)
#
# The defect: `decodeStream` keys its memo on `len(sd.Raw)` instead of the object number.
#
# This is the aliasing failure a memo invites, and it looks harmless: a decoded stream really is a
# pure function of its raw bytes, so "the same length" feels close enough to "the same stream".
# It is not — two distinct fonts, two distinct page images or two content streams of equal encoded
# length alias onto one entry, and the second one hashes as the first. The digest then reports two
# different documents as identical, which is the failure direction that MATTERS here: ADR-013 has
# `checkArrival` comparing a counterparty's document against the record at hop 1, so a collision is
# a substituted page that the comparison waves through.
#
# It is recorded because nothing about the code's shape objects to it. The memo is still bounded,
# still populated on second sighting, still returns bytes it really decoded.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestTheFastPageWalkAndTheStreamMemoAgreeWithTheOldAlgorithm -count=1"
EXPECT="the one-pass walk plus memo gives"
