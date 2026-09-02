# docs/red-proofs.md, tier 1: "a refusing gate anchors an end state on the record.json beside it"
# (P08.S05e, v1.117.322)
#
# `ReadTermination`'s own doc states the rule — "`rec` must come from the document or the
# invitation, never from the `record.json` beside it" — and `LoadState` restates it where it
# deliberately takes the weaker anchor: "a gate that REFUSES on a termination must anchor on the
# document or the invitation instead, because a planted matching pair verifies against itself."
# `checkDeliveredPayload` is such a gate. Without the invitation binding, a matching
# record-and-termination pair dropped into ~/nib/ceremonies/<id>/ verifies against itself and
# writes a durable false `Ended`, which `rearmDeliveries` then uses to skip the ceremony forever —
# so the party never receives its real copy and nothing tells them why.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnEndStateIsCheckedAgainstTheInvitationNotTheFileBesideIt -count=1"
EXPECT="bind the record to the invitation"
