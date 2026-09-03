# docs/red-proofs.md, tier 1: "a failed accept leaves no ghost in the panel" (/pending 364, v1.117.344)
#
# The defect: `WriteMe` calls `MkdirAll`, and `ListStored` lists every well-named directory under
# `~/nib/ceremonies/` without requiring a record. Writing the marker before the two vault writes
# therefore left a folder behind whenever the accept failed, and the panel P06 had just shipped
# listed it as `state:"absent"` — *"its folder may have been removed, or it was interrupted before
# anything was written"* — blaming a removal for a ceremony the same user had just been told was
# never accepted.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnAcceptThatCouldNotSaveLeavesNothingBehind -count=1"
EXPECT="the ceremonies listing carries"
