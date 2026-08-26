# docs/red-proofs.md, tier 1: "L3 stops requiring the prefix to be cross-bound" (P07.S03, v1.117.159)
#
# The defect: the gate compares identities and stops asking whether each prefix signature attests
# to a real, valid co-signer on the same document. L3 and D23 both say "each one valid and
# cross-bound"; a check that only compared identities would satisfy the sentence while missing
# half of it — a signature accepting somebody who never signed attests to nothing.
#
# The fixture isolates exactly that: A signs accepting a stranger who never signs, B signs on top
# so A is no longer the last signature and its cross-binding is due, and the identities ARE the
# roster prefix — asserted before the refusal is graded, so a red here cannot be about order.
#
# The LAST signature is exempt by construction and that is measured, not conceded: the party who
# signed most recently accepted somebody who has not signed yet.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheGateRefusesEachThingByName -count=1"
EXPECT="prefix not cross-bound: admitted"
