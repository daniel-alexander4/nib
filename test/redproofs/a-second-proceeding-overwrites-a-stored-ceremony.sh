# docs/red-proofs.md, tier 1: "a second proceeding overwrites a stored ceremony" (/pending 318, v1.117.276)
#
# The defect: `refuseDifferentProceeding` discriminates on the ID instead of the roster commitment —
# the naive guard. Since the attack IS a colliding id, `stored.ID != r.ID` is false and the write
# goes through, destroying what for a non-convener is the sole durable copy of the document carrying
# their own signature.
#
# **Both assertions fire under this patch, and that is the point of the OTHER mutation.** Moving the
# guard below the writes instead fires ONLY the byte-identity assertion — the error is still
# returned, but the bytes are already clobbered and, since v1.117.271, the sidecar already unlinked.
# The two mutations together prove the refusal and the byte-identity are independent facts rather
# than one wearing two hats. That second mutation is not recorded as its own row: it is a
# placement, not a defect anyone would reintroduce by editing the predicate.
#
# The check carries a SETUP assertion that the legitimate per-hop rewrite still succeeds — every hop
# rewrites the mirror, so a guard that refused that would break the product while passing every
# other assertion here.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestASecondProceedingCannotOverwriteAStoredCeremony -count=1"
EXPECT="a second proceeding wrote into another ceremony's directory"
