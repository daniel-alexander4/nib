# docs/red-proofs.md, tier 1: "A duplicated page carries its own grouping elements" (/pending 529)
#
# The defect: `cloneRoot` returns after ONE step, so a `/Lbl`'s copy comes with its `/LI` and not
# with the `/L` above it.
#
# It exists to prove the copy-side clause is not a restatement of the original-side one. Under this
# patch the duplicate has all THREE of its `/LI`, so the `/Lbl`-count clause
# (`clone-copies-only-the-row-leaf`) stays green — the copy is missing structure while the original
# acquires none. Measured: "the duplicated page has 0 /L and 3 /LI of its own".
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestADuplicatedPageCARRIESItsOwnGroupingElements -count=1"
EXPECT="has 0 /L and 3 /LI of its own"
