# docs/red-proofs.md, tier 1: "the hop that kept no copy only logs" (/pending 346, v1.117.302)
#
# `mirrorHop`'s failure branch keeps its `log.Printf` and drops the sticky notice — which is where
# this stood before P08.S08, and the state `cmd/nib/main.go` already argues against for its own
# hand-off notice: a double-clicked launch sends stderr nowhere.
#
# Of the three `noteFailure` kinds this is the one where the user has SIGNED and kept nothing, the
# state D24 exists to prevent. Until /pending 346 nothing observed the producer reaching this
# branch: the surface test called `noteFailure` directly and `l3_test.go` drove only the success
# path, so the notice was asserted about a call the product might never make. Its sibling
# `received-not-saved` got a real producer-side reader at P08.S05a; this one did not.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAFailedHopMirrorTellsTheSigner -count=1"
EXPECT="the user was told nothing"
