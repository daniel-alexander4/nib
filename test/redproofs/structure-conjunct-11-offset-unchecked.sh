# docs/red-proofs.md, tier 1: P01 phase close of PLAN-returned-document.md — (11)'s offset arm
#
# The defect: the gap's owning header is not required to sit at the offset the xref gives the record, so a later
# revision that re-defines the signer's own object number passes and its unsigned /Reason is reported as the signer's.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestADictionaryRedefinedUnderTheSignersNumberIsRefused"
EXPECT="want conjunct 11 cause"
