# docs/red-proofs.md, tier 1: "the route always contributes" (P07.S05, v1.117.173)
#
# The defect: `/api/session/initiate` stops asking the roster whether this party carries, so a
# `signs:false` convener signs a ceremony they were convened NOT to sign — and a signature cannot
# be taken back off a document.
#
# **Whether you sign is a fact about the roster, not a choice**, which is why there is no separate
# carry route and no flag on the request: a non-signing convener cannot accidentally sign and a
# signer cannot accidentally skip their turn, both unrepresentable rather than checked. The guard
# also asserts the ORDER — the decision must come before `buildCoSigned`, which is the door that
# applies the local signature.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestWhetherYouSignIsReadOffTheRoster -count=1"
EXPECT="the route can no longer both carry and contribute"
