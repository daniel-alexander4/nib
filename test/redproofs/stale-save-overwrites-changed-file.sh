# docs/red-proofs.md, tier 1: "A stale Save overwrites a file that changed on disk" (/pending 333, v1.117.289)
#
# The defect: handleSave's precondition sits AFTER atomicfile.WriteDurable instead of before it.
# The route still answers 412 and the banner still appears — and the user's file is already gone,
# because the write happened first.
#
# This row is what separates "we noticed" from "we noticed after destroying it", and a
# status-only assertion cannot tell those apart: both spellings answer 412. The check reads the
# BYTES ON DISK and asserts they are still what the external writer put there.
#
# Distinct from `disk-change-goes-unreported`, which is about the detector answering at all.
# This one assumes the detector works and is about WHERE its answer is consulted.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestSaveRefusesToOverwriteAFileThatChanged"
EXPECT="the save reached the disk anyway"
