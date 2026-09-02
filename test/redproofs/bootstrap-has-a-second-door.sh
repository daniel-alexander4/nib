# docs/red-proofs.md, tier 1: "The DHT bootstrap gets a second door" (P07.S05d, v1.117.203)
#
# The defect: a second caller of `rz.Bootstrap` inside `internal/server`. This is not a
# hypothetical — it is what shipped. THREE sites bootstrapped eagerly (`dialerCeremony` at
# construction, and BOTH arm paths, which are different functions), and the plan that scoped the
# fix named two of them. A fix aimed at the two named sites would have left the TCP arm emitting
# and P03's exit criterion still false, with two of three paths fixed.
#
# The guard asserts ROUTING through the door rather than the text at each site, which is why it
# catches a fourth caller that looks nothing like this one. Eight copies checked for agreement say
# nothing about a ninth site added without one (ADR-009).
#
# **RE-SITED at P08.S05f (2026-09-01).** The patch used to insert its second caller INSIDE
# `startArmedRendezvous`, which that slice deleted as dead — so the row would have failed as
# "stale" for a reason that had nothing to do with the guard. It now inserts into
# `publishCandidates`, one of the two DHT verbs the door actually serves, which is a stronger
# site: the old one was in a function nothing could execute.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheDHTBootstrapHasExactlyOneDoor"
EXPECT="must be called from ensureBootstrapped and nowhere else"
