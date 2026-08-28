# docs/red-proofs.md, tier 1: "A law figure that has left home without arriving anywhere" (P07.S09a, v1.117.223)
#
# The defect: `punchBudgetPerSide` stops being a CONST and becomes a `var`. It is still in
# `punch.go` and still reads 3000, so nothing behaves differently and the first arm of the guard is
# satisfied — which is exactly why the second arm exists. A figure deleted, moved or demoted passes
# "nothing is reachable from the tunable block", because nothing is reachable from anywhere when
# nothing is there.
#
# **And a `var` is a sharper defect than the move this row first used.** D33 makes this figure LAW
# on the ground that a bound an operator may raise is not a bound; a package-level variable is one
# any code in the package may raise at runtime, which is strictly worse than one an operator edits
# and recompiles. The single-file constraint `verify_test.go` imposes is what forced the search
# that found it.
#
# **This row also fixed the guard.** Its first form renamed the constant, which broke its callers
# and failed to compile — proving the compiler, not the check. Re-done as a move, it then exposed a
# false positive: the guard matched the substring `name + " = "`, which an ordinary typed
# declaration does not contain, so it would have failed a legal move it has no opinion about. The
# check is parsed now rather than grepped.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheLawFiguresLiveWithTheStructureTheyBound"
EXPECT="D33's split puts it with the STRUCTURE it bounds"
