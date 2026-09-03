# docs/red-proofs.md, tier 1: "the ceremonies listing is behind the vault lock"
# (P06.S01, C12, D29, v1.117.334)
#
# The shipped state until now, and the plan has said it was wrong since 2026-08-18: *"the ceremonies
# listing moves off `requireUnlocked` and locked becomes a fifth degradation class, because the vault
# lock protects nothing there: the mirror is unsealed by D29's own design."* Six of P06's exit
# criteria are about a panel that renders while locked, and behind the gate not one of them is
# buildable. Nothing in the listing is sealed: `ListStored` and `ReadStored` read `record.json` and
# nothing else.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheCeremoniesListingAnswersWithTheVaultLocked -count=1"
EXPECT="returned 401, want 200"
