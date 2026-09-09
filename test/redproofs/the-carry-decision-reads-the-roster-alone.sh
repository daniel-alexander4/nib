# docs/red-proofs.md, tier 1: "a signing convener carries once they have signed"
# (P01.S02a, /pending 436, v1.128.58)
#
# The defect: `carries()` was `!p.Signs` and nothing else — a pure roster test with no notion of how
# far the document had got. Under D22's hub the convener is at one end of EVERY hop, so a convener
# with `convenerSigns` true contributed at hop 1 and then re-entered `buildCoSigned` at hop 2, where
# `AdmitContribution` refused them `ErrNotYourTurn` at their own machine. `#cerISign` ships CHECKED,
# so that is the ceremony the setup sheet produces by default, and at three parties or more it could
# not advance past its first hop by any route.
#
# The tier-1 proof is here because the decision is made before a packet leaves. Tier 4 proves the
# same thing end to end — `relay tcp "" csigns` in build/pairrepro.sh — and that run was driven
# against this exact mutation: it failed with
#   FAIL: [tcp] initiate returned HTTP 409 before any spoken check:
#   {"error":"it is not this party's turn to sign: the document is waiting for 54e4… and this is afa3…"}
# which is the convener being refused at their own machine at hop 2. Recorded here rather than as a
# tier-4 row because a tier-4 row costs twenty minutes to replay and this one costs a second.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestASigningConvenerCarriesOnceTheyHaveSigned"
EXPECT="at hop 2 a signing convener does not carry"
