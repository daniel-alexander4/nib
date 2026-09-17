# docs/red-proofs.md, tier 1: "a named reader that mentions no field of its shape" (/pending 558)
#
# `vault.ExternalSigner`'s reader list is put back to what it was before /pending 558 repaired it:
# `internal/server/keys.go`, which does not contain the string `ExternalSigner` at all. The three
# routes had moved to `internal/server/extsigner.go` (`internal/server/server.go:484-486`) and the
# table was never re-pointed.
#
# **This row is not cosmetic, and that is the whole reason it exists.** A dead OUT-OF-PACKAGE entry
# launders the outside-the-package arm added in the same change, which asks what the reader list
# SAYS rather than what the named files do. With this patch applied the shape's only live reader is
# `internal/vault/vault.go` — its own package's — and the outside-the-package arm stays silent,
# because `internal/server/keys.go` is outside `internal/vault` by path. So the two arms are not
# redundant: without this one the other can be satisfied by a file that reads nothing, and that is
# the state the tree was actually in when the item was worked.
#
# The failure names the count, which is what makes it diagnosable: "mentions none of its 3
# field(s)". Three, not zero, is the tell — the entry is not for a shape that lost its fields, it
# is for a file that never read them.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryPublishedObservableHasANamedReader -count=1"
EXPECT="mentions none of its"
