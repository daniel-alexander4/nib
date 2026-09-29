# docs/red-proofs.md, tier 1: "the records come from the library's OWN enumeration" (ADR-051, ADR-058)
#
# The defect: `sign.sweep` building its records from the AcroForm/Fields walk instead of the
# library's own xref sweep for /Filter /Adobe.PPKLite. It is two orders of magnitude cheaper and it
# reinstates /pending 613 inside its own fix: a signature reachable only through the xref gets no
# record, so the positional join (ADR-058) no longer lines the library's signers up with the
# records it came from, and the identity is whatever the attacker's listed object supplies.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestASignatureAbsentFromFieldsIsStillAttributed"
EXPECT="a walk the attacker can empty"
