# docs/red-proofs.md, tier 1: "the sweep drops the convener rule" (P02.S02, v1.128.7)
#
# The defect: `rearmCeremonies` stops comparing this machine against the invitation's convener.
#
# **This row exists to record that the guard is SCAN-ONLY, which is not what the slice assumed.**
# The first cut of `TestTheHopSweepLeavesTheConvenerAlone` was written as a behavioural test and was
# vacuous twice over: the sweep's `pinnedLabel` check refused first, and once that was removed by
# pinning self, `ceremonyFor` reached `hopBetween`, which refuses `a == b` with "was given as both
# ends" — under D22's hub the convener's only possible counterparty is itself. So no fixture can
# make the branch the deciding one, and the OUTCOME is unchanged with it gone.
#
# The mutation was also caught being a fake red: removing the branch outright left `strings` and
# `me` unused, so the check went red by failing to COMPILE — which `redproof.sh` refuses to accept
# as a proof, and which a hand-run `grep FAIL` had accepted. This patch compiles.
#
# What the scan buys is the topology rule surviving as a statement. What it does not buy is any
# evidence the line is load-bearing, and the test says so in its own header.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheHopSweepLeavesTheConvenerAlone -count=1"
EXPECT="no longer compares this machine against the invitation's convener"
