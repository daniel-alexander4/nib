# docs/red-proofs.md, tier 1: /pending 736 — a gap nobody's header precedes moves no floor
#
# The defect: a later party appends a record whose gap is a hex token inside an earlier signer's
# dictionary; that gap lifts the floor over the signer's header, and the signer is refused
# `contents-elsewhere` and leaves the signers.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestTextBeforeContentsCannotMoveTheOwner"
EXPECT="text before its /Contents moved the owner"
