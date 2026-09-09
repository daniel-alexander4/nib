# docs/red-proofs.md, tier 1: "a stopped ceremony finishes its round" (P01.S02c, /pending 428, v1.128.60)
#
# The defect: `roundIsFinished`'s signer branch goes back to `ended == StateDeclined`, so any end
# state that is not declined falls through to `alreadyDelivered` — a stat on `~/nib/signed/<name>`,
# the FINISHED DOCUMENT, which a stopped ceremony never produces. Permanently false, so a signer who
# has been told the proceeding is over holds the directory and its pins until the three-day grace.
#
# **That is not a hypothetical: this function's own doc records it having shipped once already**, for
# `declined`, and says it was "found by trying to drive the clause at tier 4, not by reading this
# function." Adding a third end state to a two-state literal reintroduces it with every existing
# test green — `closeout_test.go` covers exactly declined and completed.
#
# The remedy is one predicate, `ceremony.DeliversDocument`, asked at all four sites that read the end
# state as a binary. Three of the four produced a false statement to a user for a third value.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAStoppedCeremonyCarriesItsAttestationAndFinishesItsRound -count=1"
EXPECT="a signer on a STOPPED ceremony is not finished"
