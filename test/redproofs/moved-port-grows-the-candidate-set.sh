# docs/red-proofs.md, tier 1: "a moved port grows the candidate set" (/pending 20, v1.117.312)
#
# **The bound this change must not weaken, made falsifiable.** The move is admitted — so every
# assertion about the refreshed mapping getting in stays GREEN — but it is APPENDED rather than
# replacing the dead address. `addrs` then passes MaxCandidates and the cap stops bounding anything.
#
# The gate refuses eviction in as many words, and its reason is distinct VICTIMS: "sixteen distinct
# victims over its life". Replacement on the same host adds no victim, which is why it is allowed;
# appending is the failure that reasoning actually forbids, and only a length assertion sees it.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAMovedPortReplacesItsHostRatherThanBeingRefused -count=1"
EXPECT="a move must REPLACE, not grow the set"
