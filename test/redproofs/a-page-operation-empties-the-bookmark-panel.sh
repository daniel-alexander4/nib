# docs/red-proofs.md, tier 1: "a page operation empties the bookmark panel" (/pending 524, 2026-09-16)
#
# The defect, verbatim as it stood until /pending 524: a page selection dropped `/Outlines`
# entirely. That was a recorded DECISION and it was right when it was made — `api.Collect` built a
# fresh context in which every page object number was new, so a copied destination "sends the reader
# to the wrong place — worse than not having one, because it is wrong rather than absent".
#
# P02.S04a moved the selection INTO the source context. The page tree is rewritten in place, so a
# kept page keeps its object number and a destination naming it is already correct after any
# permutation — there is nothing to remap, and the drop had outlived its reason.
#
# It is a user-facing loss and not housekeeping: the user AUTHORS bookmarks in nib, reads them in a
# sidebar tab called "Jump to Section", and exports through "Split by bookmarks". Measured before the
# fix, a four-bookmark document through `Collect("4","3","1")` came back with no `/Outlines` key at
# all and `Outline()` returning an empty list — and nothing anywhere told the user.
#
# The assertion grades the PAGES and not merely survival. `4,3,1` is not a rotation: source page 1
# lands at position 3, page 3 at position 2 and page 4 at position 1, so a carry that copied the
# destinations without re-resolving them, or one that remapped an index, disagrees with all three.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestAReorderedDocumentKeepsItsBookmarksPointingAtTheirOwnPages -count=1"
EXPECT="outline has 0 items, want 3"
