# docs/red-proofs.md, tier 1: "a waiting party is disarmed in minutes" (P02.S02, v1.128.6)
#
# The defect: the hop window's no-record fallback floors SHORT, as `deliveryWindowFor`'s does. It is
# the same line read in the wrong direction, and it is the mistake this row exists to keep out —
# copying a sibling's conservative default into a case whose conservative direction is the opposite.
#
# A delivery arm exists only after this party has signed, so a missing record there is anomalous and
# a short floor is safe. A hop arm exists BEFORE the document arrives, so a missing record is the
# ordinary state of a party who accepted and is waiting: flooring it short takes that signer off the
# network minutes after they accepted, which is the failure D14 exists to remove.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheHopArmsWindowIsTheRecordsDeadlineWhereThisMachineHoldsARecord -count=1"
EXPECT="with no record on disk the hop window is"
