# docs/red-proofs.md, tier 1: "The loopback rule removed from `startAnnouncing`"
#
# The defect: an armed listener bound to loopback is announced on every joined interface
# every 500 ms. A peer on the link resolves it to <our LAN address>:port and cannot connect,
# and a user who deliberately bound loopback has their six-word name — a stable function of
# a never-rotating fingerprint — put on every attached segment anyway.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestALoopbackBindIsNotAnnouncedOnTheLink"
# EXPECT is the token the real assertion prints, not the exit status: a deleted test file
# also exits non-zero, and that is what this harness used to accept as proof.
EXPECT="is loopback and was announced"
