# docs/red-proofs.md, tier 2: "no delivery control behind the lock" (/pending 353, v1.117.351)
#
# The defect: the `mayAct` gate is dropped, so the ceremony card offers a delivery round on the
# LOCK SCREEN.
#
# The card has two homes since P06.S07, and the lock screen's is possible only because both routes
# it reads are `requirePublicLoopback`. A delivery round is `requireUnlocked` and it mutates, so
# the control there can only ever earn a 401 — and it offers an action to somebody who has not
# proved they may act. The same gate also covers a non-primary Nib, which is unlocked and still
# must not drive one mirror beside another process.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/lockedpanel.test.mjs"
EXPECT="renders behind the lock"
