# docs/red-proofs.md, tier 1: "The added-after-signing warning could go quiet" (sweep 11, v1.117.49)
#
# The defect: addedAfterVerdict returns `trailing` alone, dropping the error — so a trailing
# check that could not read a signed document reports AddedAfter=false and the document looks
# wholly signed. Fail-closed is `trailing || err != nil`: an unreadable check on a signed doc
# warns rather than reporting clean, which is the safe direction for an integrity tool and
# leaves the Valid verdict untouched.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestAddedAfterFailsClosed"
EXPECT="must not report the document clean"
