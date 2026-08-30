# docs/red-proofs.md, tier 1: "the text caps bind the convener and nobody else" (/pending 308, v1.117.264)
#
# The defect: `checkIntent` and `checkRosterText` go back to having callers only inside `Convene`,
# the convener's own door — so the caps bound the party who TYPES the recital, the labels and the
# capacities, and left unbounded every party who receives them.
#
# The precise mirror of the asymmetry `Verify`'s own roster-bound comment already describes: "a cap
# enforced only on the pasted-invitation path binds the recipients and leaves the emitter
# unbounded". Here it ran the other way and nobody had noticed, because the emitter is the only
# party any test had ever played.
#
# Measured with the defect applied: a 5000-rune intent, label and capacity all verify. D25
# allocates signature pages from this text's rendered height, and P08.S05 puts the record's intent
# into a delivered filename on every party's disk.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestVerifyBoundsTheTextEveryRecipientMustRender -count=1"
EXPECT="runes verifies"
