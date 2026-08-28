# docs/red-proofs.md, tier 1: "A ceremony block that names one neighbour" (P07.S07a, C09, v1.117.216)
#
# The defect: `AppearanceLines` always takes the two-party shape, so a ceremony block renders
# `Accepts: <label> [<short fp>]` — the predecessor, and nothing about the proceeding.
#
# C09's own words are that a nine-party document's blocks must name the ceremony rather than one
# neighbour. Nine blocks each naming their predecessor describe a CHAIN OF PAIRWISE CLAIMS, which
# is the thing the roster commitment exists to replace: signer 3 attesting only to signer 2 says
# nothing about what signer 1 agreed to.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestNineBlocksNameNinePartiesAndNotOneOfThemIsNibUser"
EXPECT="names one neighbour inside a ceremony"
