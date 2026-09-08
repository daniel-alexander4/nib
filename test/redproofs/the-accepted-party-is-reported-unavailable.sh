# docs/red-proofs.md, tier 1: "the accepted party is reported unavailable" (/pending 377, v1.128.25)
#
# The defect: `handleCeremonyNext` answers `unavailable` for every non-OK class, so a party who has
# just accepted — the commonest invitee state there is — is told Nib could not read enough to say.
# It read everything there is: the directory holds this machine's own marker and no record, which is
# what waiting for the baton looks like.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAPartyWhoJustAcceptedIsNotToldTheirFolderMayBeGone -count=1"
EXPECT="want \"accepted\""
