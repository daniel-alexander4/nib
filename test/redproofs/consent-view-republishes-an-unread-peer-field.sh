# docs/red-proofs.md, tier 1: "The consent view publishes no unread peer fields" (/pending 252, v1.117.350)
#
# The defect: `pendingView` carries the connected peer's own attestation validity again — a field
# published to the browser and read by nothing, which is the `historyEvicted` class.
#
# **It is recorded because neither reader scan can catch it.** `published.test.mjs` matches on a
# bare property name and `.valid` is satisfied by `renderConsentSigners`'s `s.valid`, a
# `pendingSigner`; the Go scan has no entry for `pendingView` at all and falls through to the same
# name match. Both were laundering this exact field before it was deleted, which is why the
# decision was implemented as a deletion plus this guard rather than as a park entry.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheConsentViewPublishesNoUnreadPeerFields"
EXPECT="publishes \"valid\" again"
