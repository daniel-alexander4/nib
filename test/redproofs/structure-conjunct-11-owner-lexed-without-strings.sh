# docs/red-proofs.md, tier 1: /pending 736 — conjunct (11) lexes the owner forward, strings as strings
#
# The defect: the lex from a record's header to its gap reads a literal string's contents as tokens, so
# header-shaped text a third-party producer writes before /Contents (`/Reason(see 9 0 obj)`) refuses
# the genuine signer `contents-elsewhere` and removes it from the signers.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestTextBeforeContentsCannotMoveTheOwner"
EXPECT="text before its /Contents moved the owner"
