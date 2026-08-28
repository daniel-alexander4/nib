# docs/red-proofs.md, tier 1: "internal/server hand-rolls an atomic write again"
# (/pending 287, v1.117.194)
#
# The defect: a temp-file-plus-rename written inside `internal/server` instead of calling
# `internal/atomicfile`. That is precisely what the deleted `writeFileAtomic` was — rename-only, no
# fsync, sitting beside `internal/vault`'s function of the SAME NAME with the opposite contract.
# `handleVaultImport` reached for the wrong one to replace `vault.nib`, so the rename was atomic and
# the data blocks were not durable: a power loss inside the writeback window leaves the vault
# present and garbage while the original — the only copy of the identity — is already gone.
#
# **The patch puts the hand-rolled write BESIDE a real door call**, deliberately. A guard that only
# required `atomicfile.` to appear somewhere in the file would pass exactly that shape, and the
# whole failure being reproduced here is a second implementation nobody compares against the first.
#
# The check is structural, and durability is why: proving a write survived a crash needs a crash,
# and proving one did not is proving a negative about the page cache. What is checkable — and what
# actually regressed — is whether a caller reaches the door at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryFileWriteGoesThroughTheAtomicDoor -count=1"
EXPECT="calls os.Rename directly"
