# docs/red-proofs.md, tier 1: "a role map chain truncated at ten hops" (/pending 507, 2026-09-16)
#
# The defect, verbatim as it stood from P07.S03 until /pending 507: `standardType` followed the
# `/RoleMap` for ten hops and then RETURNED THE INTERMEDIATE NAME. A producer-private type reached
# through a longer chain therefore read as an element of whatever the tenth name happened to be.
#
# Measured against veraPDF 1.30.2, on a document whose only heading is `/S /Alpha` chained through
# thirty private types: veraPDF resolves all thirty hops and FAILS ua1 7.4.2 t1 when the chain ends
# `/H3` and PASSES it when it ends `/H1`. nib answered `NotApplicable` — "the structure tree has no
# numbered heading" — for BOTH, so the bound cost a real failure and a real pass. NotApplicable is a
# conformant verdict (`Verdict.conformant`), which makes the first of those a false pass.
#
# This is the half of /pending 507 that a `CannotCheck` at the bound would still have got wrong: the
# oracle settles these documents, so a checker that answers "could not follow it" has less reach, not
# more honesty. The chain is followed to its end now and only a CYCLE is unresolvable.
TIER="tier 1 — go test"
PROVE="go test ./internal/uacheck/ -run TestARoleMapChainIsFollowedToItsEndAndVeraPDFSaysSo -count=1"
EXPECT="role map chain ending /H3 reports not applicable"
