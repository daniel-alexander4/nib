# docs/red-proofs.md, tier 1: "a re-issued invitation carries no recital" (P08.S01, v1.117.240)
#
# The defect, and it was LIVE in the tree until this slice: `handleCeremonyInvites` rebuilds each
# party's invitation from the mirror's record plus the vault's stored secret, field by field — and
# it omitted `Intent`. `MatchesRecord` compares the recital and refuses on a mismatch, and
# `Contribute` refuses a ceremony signature whose attestation carries none. So every re-issued
# invitation parsed, armed, and was then refused at the recipient's arrival gate, AFTER the convener
# had been told the re-issue succeeded.
#
# It survived because the route had exactly one reference in the whole tree — its own registration.
# No Go test, no harness clause, no caller in the web client. It is nonetheless the only production
# reader of `ReadMirror`, and the convener's only disk-based recovery path for a lost invitation.
#
# The guard runs `MatchesRecord` itself rather than diffing fields, deliberately: a field-by-field
# comparison is the same shape that produced the defect — a constructor that forgets one field,
# checked by a test that forgets the same one.
#
# **RE-SITED at P08.S05g (2026-09-01).** The mint moved: `/api/ceremony/invites` and the delivery
# round both build a party's invitation from the record plus the convener's own secret, so that
# logic is one door (`convenerInvitationFor`) and the defect is expressed there. Same omission,
# same test, one function over — and now it is reachable from both callers rather than one.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAReIssuedInvitationStillMatchesItsRecord -count=1"
EXPECT="is refused against the record it was built from"
