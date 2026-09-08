# docs/red-proofs.md, tier 1: "the joined marker is written and never read" (/pending 377, v1.128.25)
#
# The defect: `ReadStored` stops populating `Stored.Joined`, so the `me` marker an accept writes
# reaches nobody. It fails in the safe-looking direction — every absent directory reads as one whose
# folder may have been removed — which is the state this row's fix exists to stop reporting.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestThePreHopPartyStillClassifiesAsAbsent -count=1"
EXPECT="does not read as joined"
