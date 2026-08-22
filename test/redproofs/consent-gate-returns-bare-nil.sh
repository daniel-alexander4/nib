# docs/red-proofs.md, tier 1: "A consent gate returning a bare (false, nil) on timeout"
# (P05 sweep, v1.117.38)
#
# The defect: a consent gate's timeout branch returning the same value as a refusal. This is
# the SOURCE-level guard, not the wire one — the branch takes five minutes of wall-clock to
# reach, which is why none of the three gates was ever driven and two of them shipped wrong.
#
# The guard discovers the gates rather than listing them (a listed population cannot see the
# fourth gate somebody adds) and fails if it finds fewer than three, so it cannot report full
# coverage of nothing.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryConsentGateNamesATimeoutAsATimeout"
EXPECT="returns no TimedOut sentinel"
