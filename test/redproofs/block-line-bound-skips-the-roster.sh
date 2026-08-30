# docs/red-proofs.md, tier 1: "A capacity too wide for the block it is drawn on" (P07.S07a, v1.117.216)
#
# The defect: `Convene` stops calling `checkRosterText`, so a party's label and capacity are never
# measured against the block line that will carry them.
#
# **This defect was INTRODUCED by the slice that closes it, and the diff-grill is what found it.**
# Before P07.S07a, `Label` and `Capacity` were carried, committed and never rendered, so their
# width could not matter. The slice puts both on a block that `renderAttestation` draws with
# `ctx.fillText` and no `maxWidth` — two fresh instances of the silent clipping `IntentFitsBlock`
# was written to refuse, on a repo whose law is refuse-not-clamp. Capacity is the one that
# matters: it is a claim about a party's authority, it is inside the signed commitment, and half
# of it on the page is a document that says something other than what the parties agreed.

# **RE-AIMED 2026-08-30 (/pending 308, sweep 18), and the reason is worth keeping.**
#
# The patch used to remove `checkRosterText`'s call from `Convene`. That stopped reintroducing the
# defect the moment `Record.Verify` gained the same check, because `Convene` calls `rec.Verify`
# (`internal/ceremony/convene.go:255`) — so the rule survived its own call site being deleted and
# this row replayed GREEN against a tree that still enforced it.
#
# That is ADR-009 working, visible from the outside: one predicate reached from two call sites means
# removing one site does not remove the rule. It also means the honest mutation is at the PREDICATE,
# not at a caller — a two-file patch removing both callers is what the one-file-per-patch rule
# forbids, for its own good reasons.

TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAnOverWideCapacityIsRefusedAtConveneRatherThanClippedOnTheBlock"
EXPECT="a capacity too wide for the block convened anyway"
