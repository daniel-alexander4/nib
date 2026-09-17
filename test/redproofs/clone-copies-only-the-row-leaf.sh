# docs/red-proofs.md, tier 1: "A duplicated page carries its own grouping elements" (/pending 529)
#
# The defect: `copyOfLeaf` copies the element the `/ParentTree` row names and climbs no further,
# which is what the clone path did from P02.S04b until this item. A row is indexed by MCID, so it
# names LEAVES — a `/Lbl`, a `/TD` — and never the `/L` or `/Table` above them. The copies then hang
# under the ORIGINAL page's parent.
#
# Measured on nib's own Markdown conversion: each of page 1's three `/LI` came out with TWO `/Lbl`
# and TWO `/LBody`, the first reading "••first item of the list…", and page 2 had no `/L` and no
# `/LI` at all. On veraPDF's `7.5 Tables/7.5-t01-pass-a.pdf` the same defect gave a five-column
# table a header row of TEN cells. Both outputs were `carried` with zero completeness defects and
# zero orphan pages: nothing in the package compares a row's width to its table's.
#
# This row's EXPECT is the ORIGINAL-side clause. The copy-side clause has its own row
# (`clone-climbs-one-level-only`), because the two fail independently.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestADuplicatedPageCARRIESItsOwnGroupingElements -count=1"
EXPECT="holds 2 /Lbl and 2 /LBody"
