# docs/red-proofs.md, tier 1: "a convener can leave and strand the roster" (P05.S01, v1.128.10)
#
# The defect: the convener refusal is bypassed. A convener holds the record and the document and is
# one end of every hop under D22's hub — walking away leaves every other party waiting, with no way
# to finish and nothing saying why. Tearing down a ceremony you convened is `unconvene`, which
# exists and takes the four vault stores.
#
# The refusal's SENTENCE is asserted as well as its code, because "409" alone does not tell a
# convener which of the route's three refusals they hit or what to do instead.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run 'TestLeavingIsRefusedWhereItWouldOnlyCostTheUser/a_ceremony_this_machine_convened' -count=1"
EXPECT="a convener leaving their own ceremony returned"
