# docs/red-proofs.md: "The rename rewrites the trademark line" (/pending 494, 2026-09-16)
#
# The defect: name id 7 is treated as one of the face's names. Liberation Sans's record 7 reads
# *"Liberation is a trademark of Red Hat, Inc. registered in U.S. Patent and Trademark Office and
# certain other jurisdictions."*, and the rename turns it into the same sentence about a trademark
# that does not exist.
#
# The rename exists partly BECAUSE of the licence — OFL 1.1 §3 wants the Reserved Font Name off a
# modified version — and §2 wants the copyright, trademark and licence records kept ON it. Those pull
# in opposite directions over one table, so the set that may be rewritten is the one thing here that
# a blanket replacement would get wrong while looking entirely correct.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestTheRenameKeepsTheCopyrightTrademarkAndLicenceRecords -count=1"
EXPECT="the trademark line, was rewritten"
