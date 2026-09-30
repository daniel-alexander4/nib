# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — G2: a multi-pair ByteRange resolves to its last pair
#
# The defect: G2: a multi-pair ByteRange resolves to its last pair is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestAnAbuttingSixElementByteRangeEndsAtItsLastPair"
EXPECT="want Index(4)+Index(5)"
