# docs/red-proofs.md, tier 1: "A roster entry carries a field outside the commitment" (D21)
#
# The defect: `Party` regains a `Name` field — a six-word display name, JSON-serialized, and
# outside `rosterPreimage`. This is the tree's own history, not a hypothetical: the field was
# there from P01.S06 until v1.117.41, written by nobody and read by nobody, and D21's
# refusal ("an invitation whose name and fingerprint disagree must be refused") could not
# happen because `MatchesRecord` compares Fingerprint and Signs and never Name.
#
# What it lets through: a roster whose copy the signers read and whose copy a verifier reads
# differ in the name and hash identically — one party's six words beside another party's
# fingerprint. Two people read the same words aloud, the verification "succeeds", and the pin
# is the attacker's.
#
# The guard this replays is the GENERAL one. The test it replaced asserted that one named
# field was excluded, which says nothing about the next field somebody adds; this walks every
# field of Party and demands each be in the preimage or carry a written reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestEveryPartyFieldIsInTheCommitment"
# **EXPECT was re-recorded 2026-08-25 (v1.117.156).** P07.S02 rewrote this guard "from a claim
# into a measurement" — it used to compare a hand-maintained `inPreimage` map against
# `reflect.TypeOf(Party{})`, and it now drives the preimage per field — so its sentence changed
# with it. The row still went red against its own defect and the harness said "not for its own
# reason", which is the third failure mode working exactly as designed: a stale token cannot be
# told from a deleted check by an exit status alone.
EXPECT="Party.Name varies (alpha vs beta) and RosterHash does NOT move"
