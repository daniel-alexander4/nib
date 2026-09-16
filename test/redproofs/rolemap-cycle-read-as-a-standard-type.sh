# docs/red-proofs.md, tier 1: "a role map cycle read as a standard type" (/pending 507, 2026-09-16)
#
# The defect: a `/RoleMap` that sends a name around a loop resolves to nothing, and every rule that
# asks an element's standard type treated the arbitrary name the walk stopped on as the answer. With
# the cycle detection removed, `7.3 t1` and `7.5 t1` answer **Pass** and `7.4.2 t1` answers
# **NotApplicable** over an element nib could not type at all — three conformant verdicts, which is
# ADR-032's law broken three ways in one document — and `7.18.4 t1` answers Fail naming a type it
# never established.
#
# It is the sibling of the row above: the ten-hop bound was wrong because the chain IS resolvable,
# and a cycle is the one case where it is not. Only the second earns `CannotCheck`.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run 'TestACyclicRoleMapIsCannotCheckNeverAPass|TestAWidgetWhoseElementCannotBeTypedIsCannotCheck' -count=1"
EXPECT="over an element nib cannot type, want CannotCheck"
