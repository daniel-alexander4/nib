# docs/red-proofs.md, tier 1: "leaving after signing is permitted" (P05.S01, v1.128.10)
#
# The defect: a party who has already signed is allowed to leave. It withdraws nothing — the
# signature is on the document and the other parties are relying on it — and it removes the stored
# invitation `rearmDeliveries` needs, so the only effect is that this party's own finished copy
# never arrives. A strictly self-harming action with no upside, which the route should name rather
# than perform.
#
# The discriminator is P02.S02's: on a machine that did not convene, `WriteMirror`'s only
# production callers are that party's own completed hop, so a record on disk means the hop happened.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestLeavingIsRefusedWhereItWouldOnlyCostTheUser/a_ceremony_this_machine_has_already_signed' -count=1"
EXPECT="leaving after signing returned"
