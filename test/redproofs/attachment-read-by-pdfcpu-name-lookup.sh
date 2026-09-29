# docs/red-proofs.md, tier 1: "an attachment is fetched back by its name" (/pending 745, v1.169.19)
#
# The defect: ReadAttachment finds the right entry and then fetches its bytes through pdfcpu's lenient
# name lookup (key, then the first /UF, /F or /Desc), so two entries sharing a name serve each other's bytes.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run '^TestEachListedAttachmentReadsItsOwnBytes$' -count=1"
EXPECT="another entry's bytes; want"
