# docs/red-proofs.md, tier 1: "the ceremony record is marked in the attachments list" (P06.S09,
# D29, v1.117.348)
#
# The defect: `nib-ceremony.json` listed as an ordinary embedded file. It is the one attachment in
# the product the user did not add, and the reason every editing operation on the document is being
# refused — so a user reading the list has no way to connect the two.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheCeremonyRecordIsLabelledInTheAttachmentsList -count=1"
EXPECT="is NOT marked"
