# docs/red-proofs.md, tier 1: "a disagreement never reaches the exit code" (/pending 324, v1.117.278)
#
# The defect: `refuses()` drops `disagrees()`, so a document whose signatures name DIFFERENT
# ceremonies prints exactly that in the text output and exits 0 — the CLI being the one surface
# where the machine-readable channel disagrees with the human one.
#
# **This row first replayed GREEN, and the reason is worth keeping.** The fixture used an off-roster
# stranger as the second signer, so `refuses()` still fired through `hasUnrostered()` and the
# mutation was invisible. An arm that two predicates can satisfy cannot say which one is missing.
# Both signers are now ON the roster and the check asserts it, so only `disagrees()` can carry it.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestTheCeremonyVerdictRefusesWhatItUsedToCallComplete -count=1"
EXPECT="and it exited 0"
