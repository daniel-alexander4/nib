package pdfops

import (
	"testing"
	"time"

	"nib/internal/scaling"
)

// /pending 756: the single-page helpers the per-page loops call — the text-run reader, the page box, the OCR artifact
// pass, the annotation describer, the n-up note carry — each asked `PageDict` for their page, which walks a flat tree
// from the root, so a loop over the pages stayed quadratic after /pending 753 folded the sweeps themselves. The helpers
// now take a `pdfread.Page` resolved through `pageAt` from one walk.
//
// prep builds an n-page document (untimed) and returns the loop to time over it.
//
// 4× the pages: linear ×4, a `PageDict` per page ×16; ×8 is the log midpoint. It was 2× against ×3, measured to
// completion one size after the other, and went red against good code under another repo's load — ×3.86, and ×3.21
// at load ~15 (/pending 799) — so it is COUNTED first (a `PageDict` per page allocates per kid it passes: the count
// is ×4 or ×16 whatever the machine is doing) and then timed through `scaling`'s interleaved rounds, for a
// quadratic that does not allocate.
func linearInPages(t *testing.T, what string, prep func(n int) func()) {
	t.Helper()
	scaling.AllocsGrowLinearly(t, what, 3000, 12000, 8, prep)
	scaling.GrowsLinearly(t, what, 3000, 12000, 8, func(n int) time.Duration { return scaling.TimeOnce(prep(n)) })
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
