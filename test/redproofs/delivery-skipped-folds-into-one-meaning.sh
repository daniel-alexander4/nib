# docs/red-proofs.md, tier 2: "skipped means two different things" (/pending 353, v1.117.351)
#
# The defect: `renderDeliveryOutcomes` branches on `skipped` alone, so the party that ENDED the
# proceeding is reported as one who "already had it".
#
# `deliveryOutcome`'s own doc says `delivered` is the discriminator: skipped AND delivered is a
# re-run correctly not repeating itself; skipped and NOT delivered is the decliner, and only that
# branch carries a reason. A convener told their decliner already has the document has been told
# the opposite of true.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonydeliver.test.mjs"
EXPECT="ended this proceeding"
