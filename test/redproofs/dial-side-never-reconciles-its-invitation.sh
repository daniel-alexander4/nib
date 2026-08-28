# docs/red-proofs.md, tier 1: "C17 at the door nobody checked" (P07.S07b, v1.117.219)
#
# The defect: `handleSessionInitiate` stops calling `checkArrival`, restoring the state the
# deepdive found — `checkArrival` with exactly ONE caller, on the receiving side.
#
# A party who INITIATES reads its ceremony identity from a pasted invitation and hands that
# roster to the L3 gate and to `buildCoSigned`, which stamps the roster's label, capacity and
# recital onto the signature it applies. All three come from an unsigned invitation.
# `checkCeremonyDeadline` runs there and verifies the record's own signature, which is a
# different question: it says the record is genuine, not that it is the record this invitation
# names.
#
# The guard asserts ROUTING and ORDER, not the text — ADR-009 — because a check that runs after
# `buildCoSigned` leaves the user signed into a proceeding this build has just refused, and a
# signature cannot be taken back off a document.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheDialSideAlsoRoutesThroughTheArrivalCheck"
EXPECT="never reconciles its invitation against the document"
