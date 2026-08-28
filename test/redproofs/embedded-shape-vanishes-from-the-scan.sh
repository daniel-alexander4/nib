# docs/red-proofs.md, tier 1: "a shape with an embedded field vanishes from the reader scan"
# (/pending 284, v1.117.189)
#
# The defect: `discoverObservables` treats an embedded field as "fields this scan cannot see" and
# drops the WHOLE shape. `discovery.Seen` embeds `Announcement`, so it was never discovered at all —
# and the `published` entry naming its readers claimed a coverage it had never had.
#
# **The failure mode is the one this scan exists to prevent, turned on the scan itself.** A shape
# that vanishes reads identically to a shape that publishes nothing: the scan reports a smaller
# number, every remaining entry passes, and nothing says a field went uncovered. It was caught only
# because the `published` keys were later validated against the discovered set — the discovery half
# had been unchecked.
#
# The check that fires is that validation: `published` names `discovery.Seen` and the scan no longer
# finds it.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryPublishedObservableHasANamedReader -count=1"
EXPECT="was NOT discovered as a published observable"
