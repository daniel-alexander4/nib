# docs/red-proofs.md, tier 1: "a document with two ceremony records is editable" (/pending 745, v1.169.19)
#
# The defect: ceremonyFreeze treats ceremony.ErrTwoRecords as every other Extract error — "no ceremony" —
# so a document demonstrably under a ceremony, possibly two, accepts edits.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run '^TestADocumentWithTwoCeremonyRecordsStaysFrozen$' -count=1"
EXPECT="a document carrying two ceremony records is editable"
