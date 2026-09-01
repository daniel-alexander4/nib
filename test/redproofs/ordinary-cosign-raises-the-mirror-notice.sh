# docs/red-proofs.md, tier 1: "an ordinary co-sign raises the mirror notice" (/pending 346, v1.117.302)
#
# The `ErrNoRecord` branch — a document with no ceremony, which is the MAJORITY of arrivals — starts
# planting the failure notice too. Nothing has failed on that path: there is no ceremony, so there
# is nothing to mirror, and `mirrorHop`'s own comment says the silent return is correct.
#
# It is the regression direction a fix for the row above would take if the branch were widened
# carelessly, and it is worse than the defect: a sticky warning on every ordinary co-sign trains the
# user to dismiss the one that means they have signed and kept nothing.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAFailedHopMirrorTellsTheSigner -count=1"
EXPECT="Every ordinary co-sign would then plant a sticky failure notice"
