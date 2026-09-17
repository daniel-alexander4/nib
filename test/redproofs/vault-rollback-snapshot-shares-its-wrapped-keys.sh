# docs/red-proofs.md, tier 1: "the door's slot snapshot shares its wrapped keys" (/pending 549)
#
# The defect: `mutateLocked` snapshots `v.ssh` one level deep again —
# `slots := append([]Slot(nil), v.ssh...)` — which copies the Slot structs and leaves every
# `Slot.Wrapped []byte` pointing at the original array. A rollback then hands the mutation's own
# wrapped key back as the restored one.
#
# It is the sibling of `vault-rollback-snapshot-shares-its-arrays` on the half of the vault that is
# not in Contents: the half that decides WHO CAN OPEN IT. A vault whose in-memory slot no longer
# matches the file's believes it is sealed to a key the file was never sealed to.
#
# Why the existing slot test cannot stand in for this one, measured rather than asserted:
# `TestAFailedSaveRestoresTheKeySlots` stays GREEN under this patch. Both mutators of `v.ssh` move
# whole structs — AddKey appends a fresh slot, RemoveKey shifts the slice — so neither can tell a
# one-level copy from a deep one, and `Keys()` deliberately omits `Wrapped`, so nothing reachable
# through this package's public surface can see the difference at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/vault/ -run TestAFailedSaveRestoresASlotsWrappedBytes -count=1"
EXPECT="the door's snapshot shares the Wrapped array with the mutation"
