# docs/red-proofs.md, tier 1: "a damaged ceremony is closed out on a deadline that did not verify"
# (P08.S06, P08.S03's four load classes, v1.117.330)
#
# The sweep decides from `Expires`. A record that does not parse or does not verify has no
# trustworthy `Expires`, so a close-out driven by one moves a LIVE ceremony out of the live set —
# and drops its pins — because a single file was damaged. P08.S03 built the four load classes for
# exactly this decision and all three unhealthy ones mean "leave it". The mutation is the missing
# guard rather than a wrong one, which is why the test drives all three classes and asserts the
# healthy shape of the same fixture IS closed out.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnUnreadableCeremonyIsNeverClosedOut -count=1"
EXPECT="was closed out"
