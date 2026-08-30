# docs/red-proofs.md, tier 1: "the -w rewrite is atomic but not durable" (/pending 316, v1.117.261)
#
# The defect: `writeAtomic` reaches for `atomicfile.Write` instead of `WriteDurable` — the weaker
# of two same-shaped functions one letter apart. `-w` renames over the user's ONLY copy, so a crash
# inside the writeback window leaves a truncated PDF where the original was, after `nib: rewritten`
# has already been printed.
#
# This is the exact mistake `internal/atomicfile`'s package doc records paying for once already:
# `handleVaultImport` called the rename-only twin to replace `vault.nib`.
#
# Asserted structurally, and the reason is stated in the guard: fsync is not observable from inside
# the process, so what is checkable is which door the package reaches for — which is the thing that
# regressed. The guard cannot see a Sync() deleted inside `atomicfile` itself; that is the door's
# own contract and `internal/atomicfile`'s tests own it.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestTheInPlaceRewriteIsDurableNotMerelyAtomic -count=1"
EXPECT="calls atomicfile.Write"
