# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — G1, a signature nested under /Kids is seen
#
# The defect: the sweep skips every object /Fields does not list — the /Fields walk P01.S02 deleted. A
# signature whose field sits under a /Kids parent is listed by no top-level /Fields entry, so it vanished
# from the records: the same mutation as `the-cheap-walk-lets-the-attacker-pick-the-identity`, proved here
# against the /Kids test it also fails. It fails at `Revisions` itself — the library still counts the nested
# signer and the records no longer do, so the join's count check refuses the document before the test's own
# record assertion is reached: the defect is caught fail-closed, one step earlier than the test's message.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestAKidsNestedSignatureIsSeen"
EXPECT="the document holds 0 it would process"
