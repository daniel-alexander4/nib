# docs/red-proofs.md, tier 1: "the vault exports a persist-only door" (/pending 549)
#
# The defect: `persist` is exported again, as `Save` — which is how it shipped until /pending 549.
#
# An exported persist-only method buys a caller outside this package nothing: every accessor here
# returns a copy, so nothing out there can change what save() would write. What it costs is a full
# re-encrypt under a fresh nonce over the file holding the ONLY copy of the signing identity, for a
# caller who changed nothing — and a way to persist that does not go through `mutateLocked`.
#
# The reason it needs a guard rather than a comment: `TestEveryVaultMutationGoesThroughOneDoor`
# exempts this one method from its "nothing outside the door calls save()" rule, and that exemption
# is sound only because the method mutates nothing — a property of the body, not of any caller. That
# scan reads `internal/vault` and nothing else, so it cannot see a caller in `internal/server` at
# all. Measured before the fix: `grep -rn '\.Save()' --include='*.go'` over the repo found three
# call sites, all in vault.go, plus one in-package test.
TIER="tier 1 — go test"
PROVE="go test ./internal/vault/ -run TestNoExportedDoorPersistsTheVaultWithoutMutating -count=1"
EXPECT="an exported *Vault method writes the vault file directly"
