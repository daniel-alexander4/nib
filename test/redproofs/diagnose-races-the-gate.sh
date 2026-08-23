# docs/red-proofs.md, tier 1: "diagnose() reads the racy gate on the live path" (P05 graduation, v1.117.110)
#
# The defect: diagnose() read c.gate.Stats() directly. On the live arm-side path
# (sessionStatus.status -> diagnose) it runs WHILE feedCandidates is still writing the gate via
# gate.Accept, so the read is a data race. v1.117.106 introduced it; the graduation pass caught it.
#
# What it costs: a data race on every poll of an armed ceremony's status. -race misses it unless a
# test drives status() concurrently with an active feed, so a source guard asserts diagnose() calls
# no method on c.gate (the signals are snapshotted into atomics in the gate's only writer instead).
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestDiagnoseReadsGuardedSignalsNotTheGate"
EXPECT="no longer reads the c.recordRefused atomic"
