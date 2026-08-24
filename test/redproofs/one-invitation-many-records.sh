# docs/red-proofs.md, tier 1: "one invitation authorises any number of records" (P07.S02, v1.117.153)
#
# The defect: MatchesRecord stops comparing the commitment and falls back to the per-field checks,
# which compare NOTHING that varies between two records sharing a roster. Measured at the grill: a
# second record with the same id and roster but a different intent, docHash and deadline, signed by
# the same convener, matched the first record's invitation.
#
# The harm is a convener running two chains under one ceremony id — one party carried a lease, the
# other a deed of sale at a different price — with both parties' checks passing, because within
# each document every signature carries that document's own commitment.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestOneInvitationMatchesExactlyOneRecord -count=1"
EXPECT="ACCEPTED a second signed record"
