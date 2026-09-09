# docs/red-proofs.md, tier 2: "the invitation warning says what a holder can actually do"
# (/pending 427, v1.128.57)
#
# The defect: the shipped sentence said an invitation "lets its holder find this ceremony and
# nothing more". It is D21's own wording and it is false at the line. `Invitation.Encode` is
# `json.Marshal` then base64url with a checksum — nothing signs or encrypts it — so `base64 -d`
# yields every party's label, capacity and full fingerprint plus the recital; the per-party secret
# opens that leg's candidate records and published end state; and `candidate.go:60-68` records the
# sequence-ceiling denial as an accepted residual.
#
# The negative assertion is the one this proof drives. The three POSITIVE clauses are probed
# separately in the same test and each is red on its own deletion — an assertion probed only as a
# whole hides a dead conjunct.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonypanel.test.mjs"
EXPECT="nothing beyond the rendezvous"
