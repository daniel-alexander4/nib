# docs/red-proofs.md, tier 1: "a second decline moves the recorded ender" (P08.S05e, v1.117.322)
#
# `ceremony.WriteTermination` is deliberately write-once and this marker is its companion, so it
# must be too. Nothing on the arrival path refuses a hop because a proceeding has already ended —
# the deadline gate reads `Expires`, not an end state — so a convener CAN collect a second decline.
# Overwriting moves the marker to the newer decliner, and the round goes back to walking the first
# one for the full 300 s connect deadline while naming the wrong party in its report.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheEnderIsRecordedOncePerCeremony -count=1"
EXPECT="a second, different party"
