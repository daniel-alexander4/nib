# docs/red-proofs.md, tier 1: "the refresh is scheduled after the lease has expired" (/pending 253,
# v1.117.116)
#
# The defect: an unconditional 15 s refresh floor applied to a lease SHORTER than itself. The mapping
# expires before the first refresh, and the record Nib published names a dead port until the next
# cycle. Nothing detects it. Note the direction: /pending 253 asked for a CEILING, which is inert —
# the same test carries a standing row asserting a long grant is not clamped.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheRefreshCadenceNeverOutlivesTheLease -count=1"
EXPECT="before we renew it"
