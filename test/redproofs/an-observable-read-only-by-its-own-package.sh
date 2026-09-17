# docs/red-proofs.md, tier 1: "a shape whose every declared reader is in its own package"
# (/pending 558)
#
# `ceremony.Party`'s reader list is put back to `record.go` + `invitation.go` — both inside
# `internal/ceremony` — which is the state this file's own prose has described since P07.S02 and
# never asserted: *"both inside the DEFINING package, so any new field satisfies it the moment the
# producer mentions it once"*. The out-of-package reader added in the same change
# (`internal/server/convene.go:535-537`, the re-issue loop walking `rec.Roster`) is removed.
#
# **What makes the defect invisible to every other arm is an in-package CONSUMER**, and that is the
# correction this row carries. The /pending 558 entry said a defining file satisfies the scan
# because the struct DEFINITION mentions every field. It does not — the per-field match needs a
# SELECTOR (`.Fingerprint`), and a declaration contains none. Measured on the tree this row was
# written against, `internal/udpmux/mux.go` was named a reader of `udpmux.Stats` and satisfied 0 of
# its 10 fields, because `Stats()` builds the struct with keyed literals. What satisfies the match
# is `record.go`'s preimage builder and `invitation.go`, which read the fields for real — so the
# per-field arm is green, the dead-reader arm is green, and only this one fires.
#
# The neighbouring hole it does NOT close is recorded at `internalShapes`: `udpmux.Stats` itself had
# a real out-of-package reader all along (`internal/cli/rendezvous.go` mentions every field), so no
# package-scoped rule would have caught /pending 512. That needs a reader tied to the mux INSTANCE,
# and `nib rendezvous` builds its own.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryPublishedObservableHasANamedReader -count=1"
EXPECT="every one of them is inside"
