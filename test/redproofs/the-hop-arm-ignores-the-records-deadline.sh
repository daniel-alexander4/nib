# docs/red-proofs.md, tier 1: "the hop arm ignores the record's deadline" (P02.S02, v1.128.6)
#
# The defect: a ceremony hop arm is bounded by `MaxCeremonyLife` whatever this machine knows. That
# was the shipped behaviour until this slice, on the argument — written at `armWindowFor` — that
# "an interactive arm holds an INVITATION, and an invitation carries no deadline". Half right: it
# holds no invitation deadline, but a party re-arming after their document has arrived holds a
# RECORD, and for them thirty days was a bound in name only. `armWindowFor`'s own comment calls an
# over-long arm "the worse failure, because it holds the machine's one network-reachable surface
# open for a proceeding that has ended".
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheHopArmsWindowIsTheRecordsDeadlineWhereThisMachineHoldsARecord -count=1"
EXPECT="with the record on disk the hop window is"
