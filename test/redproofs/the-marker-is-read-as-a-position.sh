# docs/red-proofs.md, tier 1: "the marker is read as a position" (/pending 377, v1.128.25)
#
# The defect: `Stored.Me` carries the marker on every class rather than only on `LoadOK` — which is
# the shape /pending 377 was first built as, and which `TestNoDegradedClassReportsAPosition` refused.
# `Me` is a pointer INTO the roster; a position shown beside a record this machine has just declined
# to trust is a claim about a roster nothing established. `Joined` is the marker's PRESENCE and
# needs no roster, which is why the discriminator is that field and not this one.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestNoDegradedClassReportsAPosition -count=1"
EXPECT="reported a position"
