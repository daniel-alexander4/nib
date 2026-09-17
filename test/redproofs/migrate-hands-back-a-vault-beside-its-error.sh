# docs/red-proofs.md, tier 1: "Migrate hands back a live Vault beside its error" (/pending 549)
#
# The defect: `Migrate` ends `return v, v.persist()` again — the shape `/pending 502` removed from
# `Create` three functions up, and the shape Migrate still carried afterwards.
#
# On a failed write the caller gets a non-nil Vault: SSH-sealed, holding a content key sealed to
# key slots the file does not carry, while the file on disk is still the old password vault. A
# caller that reads the value before checking the error — or writes `v, _ :=` — runs a Nib that
# believes it migrated, against a next launch that says it did not.
#
# Not a live defect when it was recorded, and the row says so rather than overstating it: the one
# production caller (`internal/server/auth.go`'s handleMigrate, found by
# `grep -rn 'vault\.Migrate' --include='*.go'`) checks the error and discards the Vault. This is the
# shape closed before the second caller exists, which is why the assertion is on the returned value.
TIER="tier 1 — go test"
PROVE="go test ./internal/vault/ -run TestAFailedMigrateHandsBackNoVault -count=1"
EXPECT="Migrate returned a live Vault beside its error"
