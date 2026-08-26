# docs/red-proofs.md, tier 1: "an unknown refusal code decodes to a known one" (P07.S03a, v1.117.162)
#
# The defect: `errorForCode` returns a concrete sentinel for a code it does not recognise instead
# of `ErrRefusedUnknown`. A build then tells its user something specific and false about why the
# other side refused — a verdict about the counterparty produced by a version skew, which is D32's
# whole subject and the reason the unknown case has a sentinel of its own.
#
# The guard also asserts the code is NAMED in the message, so a bug report can say which one, and
# it walks the whole enumeration in both directions: a wire code is a value two builds must agree
# on, and two tables that map it are a protocol that can disagree with itself.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestEveryRefusalCodeRoundTripsToItsOwnSentinel -count=1"
EXPECT="want ErrRefusedUnknown"
