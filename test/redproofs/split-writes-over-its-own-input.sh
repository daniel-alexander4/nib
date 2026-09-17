# docs/red-proofs.md, tier 1: "a split part silently replaces the document it was cut from"
# (/pending 569, v1.135.x)
#
# The defect: `writeSplitFiles` checks CONTAINMENT only — that the part lands inside the chosen
# directory — and never that the name it is about to write is the file it has just read. Measured
# through the real command before the fix existed: `nib split collide/foo1-2.pdf --out-dir collide
# --ranges 1-2 --prefix foo` replaced the 105,102-byte input with the 94,254-byte part, md5
# `235c2019…` → `fe51b69e…`, exit 0, and the only line printed was the part's own path — which is
# the input's path.
#
# This is the whole item, so the patch is the whole fix removed: the door call goes and the
# containment loop is what is left. It is the cheapest defect in this ledger to reintroduce by
# accident, because the code reads correct with it — a containment check is exactly what a reviewer
# expects to find in a loop that joins user-derived names onto a directory.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestASplitNeverWritesOverTheDocumentItIsSplitting -count=1"
EXPECT="the split overwrote the document it was splitting"
