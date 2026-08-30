# docs/red-proofs.md, tier 1: "an off-roster signature passes as complete" (/pending 324, v1.117.278)
#
# The defect: `markUnrostered` becomes a no-op. A valid signature copying the document's own roster
# token, from an identity the roster does not name, is then invisible to every check —
# `Completeness` counts the OBLIGED signers and can never exceed the roster, so the document reads
# "3 of 3 — Complete" while carrying a fourth signature nobody agreed to. Measured before the fix:
# `valid (4 signer(s))`, `"complete":true`, exit 0, and the web panel drew the intruder as
# `✓ First signer`.
#
# Copying the token is what defeats `markOneProceeding`, which is why this is the residue and not a
# second net: a plain appended co-signature with no token is already caught.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestTheCeremonyVerdictRefusesWhatItUsedToCallComplete -count=1"
EXPECT="was not flagged"
