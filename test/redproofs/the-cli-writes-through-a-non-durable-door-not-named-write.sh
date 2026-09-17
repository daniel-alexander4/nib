# docs/red-proofs.md, tier 1: "the CLI writes through a non-durable door not named Write" (/pending 550)
#
# The defect: `internal/cli` reaches `atomicfile.WriteFrom` — a door whose own comment says it is
# *"ATOMIC, and deliberately not durable, the same choice `Write` makes and for the same reason"* —
# for a file the user named. A crash inside the writeback window then leaves a truncated file where
# the original was, after the command has printed that it was written.
#
# **This is not a hypothetical shape: it is the hole the guard had.** Before /pending 550 the check
# was `strings.Contains(src, "atomicfile.Write(")`, and `atomicfile.WriteFrom(` does not contain
# that string. Measured rather than argued — with this exact patch applied, the pre-550 guard
# printed `12 file(s) scanned, 1 durable writer(s), 0 non-durable` and PASSED. So the row proves
# the property the guard now asserts (a non-durable DOOR is refused) and not the one it used to
# (a non-durable NAME is refused), which is the whole difference.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestTheInPlaceRewriteIsDurableNotMerelyAtomic -count=1"
EXPECT="reaches a NON-DURABLE atomicfile door"
