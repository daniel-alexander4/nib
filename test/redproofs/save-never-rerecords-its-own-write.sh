# docs/red-proofs.md, tier 1: "Save does not re-stamp the baseline it just wrote" (/pending 333, v1.117.289)
#
# The defect: handleSave omits recordDisk after assigning doc.data. WriteDurable RENAMES into
# place, so the inode and the mtime both change on every save Nib itself performs — and the
# document then reports "changed on disk" from the moment the user first saves it, against Nib's
# own write, forever.
#
# The row nobody writes, and the one whose absence ships a permanently armed warning: every other
# check in this feature stays green under it. The second half of the assertion is the consequence
# that actually reaches the user — every save after the first is refused with 412.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestSaveRerecordsItsOwnWrite"
EXPECT="immediately after ITS OWN save"
