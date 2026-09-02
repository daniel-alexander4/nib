# docs/red-proofs.md, tier 1: "the close-out prune deletes the ceremony directory"
# (P08.S06, C11, v1.117.330)
#
# `RemoveMirror`'s own doc comment invites exactly this: *"D29's close-out prune is P08.S06's, and
# this function is what it will call"* — and it is an `os.RemoveAll`. On every machine but the
# convener's, `~/nib/ceremonies/<id>/document.pdf` is the ONLY place that party's own signed
# contribution exists, and on declined, expired and abandoned there is no delivery round to have
# carried it anywhere. The mutation is one call, and the cost is a user's signature destroyed by
# their own software's tidying.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheCloseOutPrunePreservesThisMachinesOwnContribution -count=1"
EXPECT="own signed contribution is GONE"
