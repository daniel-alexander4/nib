package pdfops

import (
	"runtime"
	"testing"
	"time"
)

// /pending 756: the single-page helpers the per-page loops call — the text-run reader, the page box, the OCR artifact
// pass, the annotation describer, the n-up note carry — each asked `PageDict` for their page, which walks a flat tree
// from the root, so a loop over the pages stayed quadratic after /pending 753 folded the sweeps themselves. The helpers
// now take a `pdfread.Page` resolved through `pageAt` from one walk. Doubling the pages must not much more than double
// a loop's cost; one `PageDict` per page on this tree quadruples it.
//
// prep builds an n-page document (untimed) and returns the loop to time over it.
func linearInPages(t *testing.T, what string, prep func(n int) func()) {
	t.Helper()
	best := func(n int) time.Duration {
		b := time.Duration(1 << 62)
		for i := 0; i < 5; i++ {
			loop := prep(n)
			runtime.GC()
			t0 := time.Now()
			loop()
			b = min(b, time.Since(t0))
		}
		return b
	}
	small, large := best(3000), best(6000)
	t.Logf("%s: 3,000 pages %v, 6,000 pages %v (×%.2f)", what, small, large, float64(large)/float64(small))
	if large > 3*small {
		t.Fatalf("%s: doubling the pages cost ×%.2f (%v → %v): a per-page helper walks the page tree per page",
			what, float64(large)/float64(small), small, large)
	}
}

// The text-run family: every whole-document reader of `readPageRuns` — the proposer here; `UnmarkedTextRuns`, the
// structure view and the commit's read share the shape and the door (`pageAt`).
func TestTheTextRunReadersAreLinearInThePageCount(t *testing.T) {
	linearInPages(t, "proposeStructure", func(n int) func() {
		ctx := flatPages(t, n)
		return func() {
			p, err := proposeStructure(ctx)
			if err != nil || len(p.noText) != n {
				t.Fatalf("proposeStructure: %d of %d pages read as empty, err %v", len(p.noText), n, err)
			}
		}
	})
}

// The structure-writer family: helpers resolved through the tree's own walk (`structTree.walkedPage`) — the annotation
// describer here; the OCR artifact pass, the element artifacter and the commit's resource read share the door.
func TestTheStructureWritersPerPageAreLinearInThePageCount(t *testing.T) {
	linearInPages(t, "describeAnnotationsOnPage", func(n int) func() {
		ctx := flatPages(t, n)
		tree, err := ensureStructTree(ctx, livePageObjects(ctx))
		if err != nil {
			t.Fatal(err)
		}
		return func() {
			for p := 1; p <= n; p++ {
				if _, err := describeAnnotationsOnPage(ctx, tree, p, "Text", "Annot"); err != nil {
					t.Fatalf("page %d: %v", p, err)
				}
			}
		}
	})
}
