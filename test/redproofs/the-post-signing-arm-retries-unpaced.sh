# docs/red-proofs.md, tier 1: "both retry arms route through the door" (/pending 369, v1.117.353)
#
# The defect: the post-signing promote retries with a bare `continue`, as it always did.
#
# **/pending 369 named one unbounded continue and there were two.** The pre-signing re-race at
# least tested its deadline inline; this one tested nothing — a peer that connects and fails to
# promote spun the arm until `postSignDeadline`. The guard asserts ROUTING through `reraceWait`
# rather than either site's numbers, because a guard written against the site the item named would
# have said nothing about the site it did not.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestBothRetryArmsRouteThroughTheReraceDoor"
EXPECT="want 2"
