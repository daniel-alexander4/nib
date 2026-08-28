# docs/red-proofs.md, tier 1: "A recital nothing reconciles" (P07.S07b, C17, v1.117.219)
#
# The defect: `MatchesRecord` stops comparing the invitation's recital against the record's.
#
# **This is the row that says why the field could be added at all.** `Invitation.Intent` travels in
# an UNSIGNED invitation and `RosterHash` is a digest of the record copied into it, so editing the
# recital alone leaves that digest matching — and the signing path reads its recital from the
# invitation, because `internal/p2p` cannot import `internal/ceremony`. Without this comparison the
# field is exactly the "unsigned hint a tamperer sets to whatever makes the comparison pass" that
# `Record.DigestVersion`'s own comment warns about, and the completeness guard next door had
# recorded that as the reason not to carry it.
#
# Two checks go red under one patch, which is the point: the per-field test AND the structural
# completeness test that requires every Invitation field to be driven or excused by name.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAnInvitationWhoseRecitalDiffersIsRefusedByName"
EXPECT="an invitation whose recital differs from the record's was accepted"
