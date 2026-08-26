# docs/red-proofs.md, tier 1: "a MUTATING route escapes D29's freeze" (P07.S02a, v1.117.155)
#
# The defect: `handleSave` loses its `ceremonyFreeze` call — the state the slice found and the
# reason the freeze needed a THIRD site. It reaches neither commit door (it writes the file
# itself and assigns doc.data under the lock), so the freeze hanging on `commitMutation` and
# `commitBarrier` covered eleven mutating routes and not this one, while `/api/save` sits in
# tier 2's own MUTATING inventory. A save on a convened document would reach the disk — and the
# file on disk is the copy every other party was invited to sign.
#
# **This row is also the guard's own red proof, which is why the COMMENT is left in place.**
# The first draft of `TestEveryMutatingRouteReachesTheCeremonyFreeze` substring-matched the raw
# body, and `handleSave` names both commit doors only inside the comment explaining that it
# reaches neither — so the guard read that sentence as proof of coverage and this exact deletion
# left the suite green. The patch deletes the call and keeps the prose: if the comment-stripping
# is ever removed, this row goes green and says so.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryMutatingRouteReachesTheCeremonyFreeze -count=1"
EXPECT="/api/save (handleSave) is a MUTATING route and reaches neither a commit door"
