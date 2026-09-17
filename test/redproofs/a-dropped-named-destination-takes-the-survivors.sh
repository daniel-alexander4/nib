# docs/red-proofs.md, tier 1: "a dropped named destination takes the survivors" (/pending 524, 2026-09-16)
#
# The defect, verbatim as it stood from P02.S04a until /pending 524: `pruneNames` removed a doomed
# destination with `node.Remove(xt, k)`. pdfcpu's removal path calls `DeleteObjectGraph` on the
# removed entry's VALUE (`model/nameTree.go:381-535`), and a destination array's graph reaches the
# PAGE it names and whatever that page shares. Those object numbers go on the free list,
# `BindNameTrees` hands them straight back out to the kid dictionaries it mints while binding, and
# the SURVIVING entries end up pointing at the name-tree nodes that replaced them.
#
# Measured on a document with NO OUTLINE in it at all — six named destinations, of which the two on
# dropped pages shared a leaf: the output had no `/Dests` tree whatsoever, so all six went, including
# the four whose pages survived. That is every named destination in the document, which is what a
# link annotation written by Word or LaTeX uses.
#
# Passing `nil` unlinks and frees nothing: every freeing site in that path is guarded by
# `if xRefTable != nil` and nothing else there uses the table. Freeing was never needed, because
# `pageselect.go` turns on pdfcpu writing by REACHABILITY — an unreferenced destination array is not
# written, which is the same reason `unlinkDestinations` unlinks rather than deletes.
#
# **The fixture's SHAPE is the test.** pdfcpu splits a leaf at `maxEntries = 3`, so six names give
# three leaves of two, and the object-number reuse only happens when a leaf EMPTIES — that is what
# calls `removeKid`, which frees the leaf dictionary and then the intermediate above it. The two
# doomed names must therefore share a leaf. With one from each of two different leaves nothing
# empties, nothing is freed, and the same document comes through clean — which is exactly how this
# stayed invisible.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestDroppingOneNamedDestinationKeepsTheRest -count=1"
EXPECT="removing the two that died took the "
