# docs/red-proofs.md, tier 1: "The DHT hold renews from now instead of from the last sighting"
# (P07.S05e, v1.117.207)
#
# The defect: `holdDHT` waits a full `lanFirstBudget` on every iteration rather than the time
# REMAINING since the last sighting, so a single sighting holds the DHT tier forever. That is
# not a privacy win, it is a ceremony that can never fall back: the peer walked off the link
# after being seen once, and the arm waits for it until the ceremony's thirty-day bound.
#
# The falsifiable half of holding on evidence is that it LAPSES, and this is the row that says
# so. The guard bounds its context so a non-lapsing hold fails in seconds rather than hanging
# to the package timeout — a guard that hangs is read as a flake and rerun rather than fixed.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheDHTHoldRenewsOnEvidenceAndLapsesWithout"
EXPECT="the hold never lapsed"
