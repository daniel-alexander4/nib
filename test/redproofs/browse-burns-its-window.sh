# docs/red-proofs.md, tier 1: "The browse's early exit removed"
#
# The defect: a browse waits D16's full window even after the link has gone quiet, so every
# LAN ceremony pays it — and once P05 races the tiers, a tier holding its answer until the
# window closes loses to a slower tier that reported sooner.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestABrowseStopsOnceTheLinkGoesQuiet"
EXPECT="after hearing its answer"
