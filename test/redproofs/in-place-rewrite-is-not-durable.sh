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
# **The EXPECT token was stale and the row could not re-prove** (found 2026-09-16, /pending 569).
# It read `calls atomicfile.Write`, which is what this guard printed until /pending 550 reworded the
# assertion at v1.135.15 — from a message about a NAME to one about the PROPERTY, which was that
# item's whole point. The patch still applied, so `TestEveryRedProofStillApplies` stayed green: it
# checks that a row's defect applies and says nothing about whether the check then fires for its own
# reason. `./build/redproof.sh in-place-rewrite-is-not-durable` was red on main for that whole
# window, reporting "went red, but not for its own reason".
#
# The replacement names the FILE and the DOOR, not just the assertion, so this row and
# `the-cli-writes-through-a-non-durable-door-not-named-write` — which trips the same assertion from
# `commands.go` — cannot satisfy each other's token.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestTheInPlaceRewriteIsDurableNotMerelyAtomic -count=1"
EXPECT="cli.go reaches a NON-DURABLE atomicfile door: Write"
