# docs/red-proofs.md, tier 1: "the vault's rollback restores a header over bytes already
# overwritten" (/pending 510, v1.133.x)
#
# The defect: `mutateLocked`'s snapshot is a struct copy of `v.contents` instead of the marshalled
# payload. A struct copy shares every slice's backing array, so restoring it puts the old LENGTH
# back over elements the mutation has already moved — the rollback runs, reports success, and
# undoes nothing.
#
# It is the shape a correct-looking door ships with, and the reason this row exists rather than a
# comment: the fifteen-mutator census
# (`TestAFailedSaveLeavesNothingBehindAtEveryRoutedMutator`) stays GREEN under this patch, because
# a plain string or a replaced slice header restores fine either way. Only the four sites that
# write in place can tell — measured: 4 of 4 subtests red here, 15 of 15 census cases green.
#
# DeleteImage filters `Images[:0]` over the shared array; AddCeremonySecret upserts through
# `&v.contents.CeremonySecrets[i]` having ALREADY zeroed the secret it replaces, so a vacuous
# rollback loses the convener's only copy of an invitation secret.
TIER="tier 1 — go test"
PROVE="go test ./internal/vault/ -run TestAFailedSaveRestoresWhatTheMutationOverwroteInPlace -count=1"
EXPECT="compacts the SHARED backing array"
