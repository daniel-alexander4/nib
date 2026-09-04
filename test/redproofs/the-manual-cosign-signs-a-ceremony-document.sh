# docs/red-proofs.md, tier 1: "a document in a proceeding is not co-signed outside it"
# (/pending 368, P06.S09, D29, v1.117.348)
#
# The defect: the shipped one. With no invitation, `cer` is nil, `l3Roster()` returns the zero
# Roster, and nothing on the manual co-sign path looked at the document — so a party holding a
# convened document open, using the ordinary controls, produced a signature with no roster, no L3
# gate and no arrival check. The far end refuses it on a prefix mismatch, after the user has spent
# a signature they cannot take back.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheManualCoSignRefusesADocumentInACeremony -count=1"
EXPECT="signed a document that carries a ceremony record"
