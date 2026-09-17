# docs/red-proofs.md, tier 1: "rolemap cycle reported as cannotcheck not fail" (/pending 548, 2026-09-16)
#
# The defect: the rule holds the cycle and answers `CannotCheck` over it — the exact state
# `/pending 507` left and `/pending 548` closed. A document nib KNOWS is non-conformant is reported
# as one nib could not check, and `CheckForUA` refuses it for the wrong reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestACircularRoleMapFailsTheClauseThatIsAboutIt -count=1"
EXPECT="nib holds the cycle and this is the clause it breaks"
