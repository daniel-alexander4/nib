# docs/red-proofs.md, tier 1: "The sequence-ceiling refusal returning bep44.Put{}"
# (P05 sweep, v1.117.38)
#
# The defect: at the sequence ceiling the callback returned `bep44.Put{}` to mean "refuse".
# But `getput.SeqToPut` has no error return, and v2.24.0 uses the result UNCONDITIONALLY —
# `put := seqToPut(autoSeq)` (exts/getput/getput.go:154) — fanning it out to every node in
# `op.Closest()` (:155-168), each of which calls `Server.Put`, whose first line writes to the
# LOCAL store (server.go:1081) before the context is ever consulted.
#
# What it costs: the branch whose entire job is to refuse published instead. Measured: an
# empty Put has no key, so it is IMMUTABLE, its target is sha1 of its nil value —
# da39a3ee5e6b4b0d3255bfef95601890afd80709, sha1 of the empty string — and `bep44.Check`
# ACCEPTS it. An empty item, at a target belonging to nobody, written to strangers.
#
# `roundtrip_test.go` asserts the refusal and never the silence, which is why nothing saw it.
TIER="tier 1 — go test"
PROVE="go test ./internal/rendezvous/ -run TestARefusedPublishEmitsNothingThatAnybodyStores"
EXPECT="the refusal returned the EMPTY item"
