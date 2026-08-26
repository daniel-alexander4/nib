# docs/red-proofs.md, tier 1: "the durable write is a plain os.WriteFile" (P07.S02a, v1.117.155)
#
# The defect: `atomicfile.WriteDurable` becomes `os.WriteFile` — no temp file, no fsync, no
# rename. The bytes are written THROUGH the existing file, so the target is truncated the moment
# the write starts and an interrupted write leaves a truncated vault where the only copy of the
# signing identity was. And the mode is honoured only on CREATE, so a vault or a ceremony mirror
# written over an existing file keeps whatever mode it already had.
#
# **This row is a guard's red proof.** Measured at the slice's diff review: the package's other
# two tests — content-at-the-requested-mode, and overwrites-leaving-no-temporary — both stay GREEN
# against this patch, because `os.WriteFile` satisfies every observation they make. The one test
# that could tell them apart hit its own `t.Skip`. It now discriminates on INODE and on the mode
# of a file pre-created at 0644, which is the state `os.WriteFile` preserves and a rename does not.
TIER="tier 1 — go test"
PROVE="go test ./internal/atomicfile/ -run TestWriteDurableREPLACESTheFileRatherThanWritingThrough -count=1"
EXPECT="the bytes were written THROUGH the existing file"
