# docs/red-proofs.md, tier 1: "the verdict checks Sent before Own" (/pending 23, v1.117.352)
#
# The defect: `Stats.Verdict()` tests `Own == 0` before `Sent == 0`.
#
# **Its sibling `discover-verdict-order` is the SAME mutation proved through the CLI's test.** One
# mutation, two rows, because each proves a different caller ROUTES through the shared door
# (ADR-009) rather than keeping a private copy that happens to agree.
#
# **The ordering IS the rule.** A machine that sent nothing also heard nothing of its own, so
# `Own == 0` holds there too — classifying on it first tells a user with no working interface that
# a firewall is dropping their multicast, which sends them to the wrong place entirely. The two
# remedies are opposite: one is "your network is not connected", the other is "your firewall is".
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheVerdictSeparatesTheThreeWaysOfFindingNothing"
EXPECT="want 1"
