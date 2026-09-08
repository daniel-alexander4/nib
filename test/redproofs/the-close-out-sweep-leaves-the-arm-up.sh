# docs/red-proofs.md, tier 1: "the close-out sweep leaves the arm up" (/pending 378, v1.128.25)
#
# The defect: `closeOutEnded` stops calling `stopListeningFor`, so a ceremony moved out of the live
# set by the deadline goes on holding the slot — the leave route's defect at the other of the rule's
# two doors.
#
# **Recorded because it SURVIVED the first mutation pass.** Every test written alongside the fix
# drives the leave route, so deleting this site left them all green. A rule that reaches one of its
# two sites is ADR-009's shape, and a mutation on the site nobody drove is how it is found.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheCloseOutSweepReleasesTheArmToo -count=1"
EXPECT="left this machine listening for it"
