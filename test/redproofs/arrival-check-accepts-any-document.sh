# docs/red-proofs.md, tier 1: "the arrival check accepts any document" (P07.S02b, v1.117.157)
#
# The defect: `ceremonyID.checkArrival` returns nil before it asks anything — the shape a gate
# takes when somebody "temporarily" disables it. C17's substance is that a party reconciles what
# it was handed: a document with no ceremony record at all (an unrelated file arriving under a
# ceremony arm), and a document carrying a valid record for a DIFFERENT ceremony (a convener
# running two chains under one invitation, which is what RosterHash exists to make impossible).
#
# The guard drives the control FIRST — the document this invitation was actually made for passes —
# because a gate that refuses everything satisfies both refusals.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheArrivalCheckRefusesADocumentTheInvitationDoesNotDescribe -count=1"
EXPECT="a document with no ceremony record at all was accepted"
