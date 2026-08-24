# docs/red-proofs.md, tier 1: "a Party field outside the commitment" (P07.S02, v1.117.153)
#
# The defect: Capacity is on Party and NOT in rosterPreimage, so Director and Witness hash
# identically and a convener can show the signers one roster and a verifier another.
#
# This row exists because the PREVIOUS guard could be silenced by a one-line map edit. It compared
# a hand-maintained inPreimage map against reflect.TypeOf(Party{}) and never against the preimage —
# measured on a pristine export: Capacity declared in the map ALONE shipped GREEN. The guard now
# DRIVES the preimage per field, so there is no map to edit.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestEveryPartyFieldIsInTheCommitment -count=1"
EXPECT="Party.Capacity varies"
