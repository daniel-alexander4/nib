# docs/red-proofs.md, tier 2: "a sign step claims a tick nothing observes" (ADR-027, v1.127.0)
#
# A step whose completion Nib genuinely cannot see — whether you ran a hidden-content scan —
# declares `done: () => true` instead of `null`, so it renders a permanent green tick.
#
# **This is the failure mode the feature is one edit away from at all times.** Every untracked row
# is a dash that somebody will eventually want to be a tick, and the tick is trivial to write and
# impossible to notice: it is green from the first render and it is always wrong. The honest state
# is `null`, which renders "—" and says on hover that Nib cannot tell.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/toolbargroups.test.mjs"
EXPECT="a tick that nothing observes"
