# docs/red-proofs.md, tier 1: "a commitment is written without its version" (P07.S04, v1.117.169)
#
# The defect: `reason()` emits the token whenever a hash is present, version or not. A reader
# handed a bare hash cannot tell a different ceremony from a different record format — the exact
# ambiguity the version exists to remove — and the client renders the second as the first.
#
# **Both, or neither.** Emitting nothing is the fail-CLOSED direction: `markOneProceeding` treats a
# missing commitment as disqualifying, so the signature reads as "not part of this proceeding"
# rather than as a commitment somebody might compare. The reader refuses version 0 for the same
# reason, so the pair cannot be produced through one door and not the other.
#
# Three test fixtures failed loudly on this rule when it landed rather than silently carrying no
# commitment, which is what the rule is for.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestARosterHashWithoutAVersionCarriesNoToken -count=1"
EXPECT="was written into the signature"
