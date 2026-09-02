# docs/red-proofs.md, tier 1: "a refusal with no wire code reaches the initiator as bare EOF"
# (P08.S03, C04, v1.117.328)
#
# Found by this slice's own review. The content anchor's refusal was a bare `fmt.Errorf` from
# `ceremony.CheckDocument` with no sentinel, so `refusalCode` returned 0, `refusalAck` wrote
# nothing, and the initiator saw `EOF` — rendered as a 502 with a D19 NETWORK cause, inviting the
# retry a refusal must never invite. That is verbatim the defect codes 10-12 and 13-14 were minted
# to close.
#
# **And the codeless path IS the threat-model path.** On an honest send the sender's own copy of the
# gate refuses first, so this refusal crosses the wire only when the sender skipped its own gate —
# which is the substitution attack the check exists for.
#
# It also escaped `refusalenum_test.go`, which enumerates `internal/p2p` sentinels: a bare error
# raised in `internal/ceremony` is invisible to it.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheRefusalEnumerationIsDerivedFromSource -count=1"
EXPECT="encode"
