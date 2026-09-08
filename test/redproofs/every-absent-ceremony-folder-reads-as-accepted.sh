# docs/red-proofs.md, tier 1: "every absent ceremony folder reads as accepted" (/pending 377, v1.128.25)
#
# The defect: the new state drops its `Joined` condition and answers `accepted` for any absent
# directory — the mirror image of the bug it fixed, and the one a blanket rewording would have
# shipped. A folder the user deleted then reads as a ceremony they are taking part in.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAGenuinelyEmptyCeremonyFolderStillSaysSo -count=1"
EXPECT="want \"unavailable\""
