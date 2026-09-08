# docs/red-proofs.md, tier 1: "the announcement version is not bumped" (/pending 385, v1.128.30)
#
# The defect: the format gains the hop and keeps version 2, so a version-2 announcement — which
# carries no hop — parses and this Nib guesses which arm it names. `version`'s own doc: an
# announcement whose version is not this one is refused rather than best-guessed, because "a version
# field that is read and then ignored is decoration". ADR-010 made exactly this argument for the
# transport byte.
TIER="tier 1 — go test"
PROVE="go test ./internal/discovery/ -run TestAVersionTwoAnnouncementIsRefused -count=1"
EXPECT="a version-2 announcement parsed"
