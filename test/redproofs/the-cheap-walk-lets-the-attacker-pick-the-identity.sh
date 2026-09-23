# docs/red-proofs.md, tier 1: "both sides of the join key come from ONE enumeration" (ADR-051)
#
# The defect: keying the signer map from an AcroForm/Fields walk instead of the library's own xref
# sweep for /Filter /Adobe.PPKLite. It is two orders of magnitude cheaper and it reinstates
# /pending 613 inside its own fix — the bag is unsigned bytes, so with two walks the attacker writes
# both sides of the key: the real signature reachable only through the xref, plus a decoy /Fields
# entry carrying the same certificate bag and a SignerInfo naming the victim.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestASignatureAbsentFromFieldsIsStillAttributed"
EXPECT="a walk the attacker can empty"
