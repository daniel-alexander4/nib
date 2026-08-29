# docs/red-proofs.md, tier 1: "A script waved through a half-signed deed" (P07.S10, v1.117.231)
#
# The defect: `incomplete()` always returns false, so `nib verify` exits 0 on a ceremony an
# obliged party never signed.
#
# The README ships `nib verify contract.pdf && echo "signature intact"`. Every signature on a
# five-of-nine document is valid and `State` is Valid, so the machine-readable channel says "fine"
# about a document whose own human-readable report says INCOMPLETE.
#
# **That divergence is why `AddedAfter` joined this exit condition**, and the reasoning is recorded
# in the same function: "the CLI was the one surface where the machine-readable channel disagreed
# with the human one." This is the second instance of it, closed the same way.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestVerifyNamesWhoHasNotSignedAndExitsNonZero"
EXPECT="exited 0 on a ceremony four obliged parties never signed"
