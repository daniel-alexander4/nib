# docs/red-proofs.md, tier 1: "the re-delivery window is re-armed on every reconnect"
# (/pending 289, v1.117.187)
#
# The defect: `postSign` — the absolute deadline bounding the post-signing re-delivery window — is
# assigned again inside the accept loop, so each reconnect pushes the window out. That is a window
# anybody who can reach the listener holds open for free, which is the defect P05.S01 named for the
# ARM window, arriving one phase later under the other deadline's name.
#
# **It cannot be caught behaviourally at a sane cost**: the harm is a window that never closes, so
# a test would have to wait for a close that never comes and then conclude something from a
# timeout. The guard is structural for the same reason its sibling — the `timer.Reset` check in
# the same test — is: what must hold is that there is exactly ONE assignment, and no observation of
# a successful re-delivery distinguishes one from two.
#
# The two halves are separate rows because they fail independently: this one leaves every Reset a
# proper remainder, and `reset-to-a-fresh-period` leaves `postSign` assigned exactly once.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheArmWindowIsNotExtendedByConnectionsThatProduceNoSession -count=1"
EXPECT="the re-delivery window must be fixed once"
