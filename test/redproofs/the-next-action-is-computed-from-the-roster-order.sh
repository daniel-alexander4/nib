# docs/red-proofs.md, tier 1: "whose turn is it, answered from the roster instead of L3"
# (P06.S03, D17 amendment/D23, ADR-009, v1.117.336)
#
# The naive answer, and the one a JS reimplementation over the roster produces: the first roster
# entry. It is WRONG whenever a convener does not sign — a non-signing convener holds a position in
# the roster and none in the signing order (D22), so the roster's first entry and the signing
# order's first entry are different parties.
#
# P06's criterion is that the panel's enabled action is "computed from the record by the same
# function the server's L3 check uses", and `p2p.NextContributor` is that function —
# `AdmitContribution`, the gate that REFUSES, is built on the same call. The fixture is built to
# disagree on purpose: a fixture where the roster order and the signing order agree cannot tell a
# shared rule from two rules that happen to match, which is the whole failure mode ADR-009 exists
# for.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheNextActionComesFromTheRecordAndNotTheRosterOrder -count=1"
EXPECT="which is the roster's FIRST entry"
