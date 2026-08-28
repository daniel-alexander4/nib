# docs/red-proofs.md, tier 1: "concurrent first callers mint different signing identities"
# (/pending 285, v1.117.190)
#
# The defect: the if-absent check is removed, so the identity store overwrites unconditionally and
# two near-simultaneous first callers both mint — the second clobbering the first. Measured before
# the fix: **eight concurrent callers against one fresh vault produced 3 distinct identities, 6 of
# them holding a certificate the vault did not hold**; the mutation reproduces 5 of 8.
#
# **It needs only near-simultaneous FIRST calls**, which two browser tabs will do, and `identity()`
# has eight callers. For `finalize` a loser signs with an orphaned key — bad, and local. For a
# **ceremony record** it is durable and cross-party: the record names a convener whose key the
# machine discarded, so no later hop can act as convener and no re-convene can prove continuity.
#
# The check asserts two things and the second is the one with teeth. That every caller AGREES is
# necessary and not sufficient — they could all agree on a key the vault does not hold, which is the
# same orphan by another route — so it reads the vault back and requires the agreed identity to be
# the STORED one. Both assertions fire against this patch.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestConcurrentFirstCallersAllGetTheStoredIdentity -count=1"
EXPECT="DISTINCT identities"
