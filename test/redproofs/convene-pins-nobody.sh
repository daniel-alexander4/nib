# docs/red-proofs.md, tier 1: "convening pins nobody" (P07.S02b, v1.117.157)
#
# The defect: `/api/ceremony/convene` stops pinning its roster — the state this route was in until
# this slice, when its only vault write was `AddCeremonySecret`. D21 is usually read as being
# about the invitee, and it is the same step for the convener, who otherwise types N-1
# fingerprints by hand before arming against any of them.
#
# This is also the first Go test of any kind over the convene route: S02a live-verified it with a
# scratchpad script that no longer exists, so between the two slices the product's only
# ceremony-creating surface was exercised by nothing committed.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestConveningPinsItsRosterAndKeepsTheSecretOutOfTheMirror -count=1"
EXPECT="is not pinned after convening"
