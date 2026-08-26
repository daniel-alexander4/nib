# docs/red-proofs.md, tier 1: "one of the two contribution doors stops stamping the commitment"
# (P07.S05b, v1.117.180)
#
# The defect: `buildCoSigned` — the INITIATING side's contribution — no longer calls
# `StampCommitment`, while `coSignExchange` still does. So half the signatures in a ceremony name
# it and half do not, `OneProceeding` goes false for the whole document, and a verifier reads a
# proceeding the parties did agree on as one they did not.
#
# **The ADR-009 shape, and its own row.** "A signature inside a ceremony carries that ceremony's
# commitment" holds at exactly two sites in two packages, so a behavioural test that drives ONE of
# them passes while the other rots — which is how the rule came to have a reader and no writer in
# the first place. This is the routing walk, and its stimulus is per DIRECTORY: a total of two is
# also what "read one package and found both there" looks like.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestEveryContributionEntryPointReachesTheGate -count=1"
EXPECT="without stamping the ceremony's commitment"
