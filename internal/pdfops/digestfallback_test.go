package pdfops

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// flatWithKidsLeaf is n pages under one /Pages node, the page at position at carrying `/Kids` of two more pages:
// `collectLeaves` descends into them and counts one leaf more than `/Count`, so the digest takes its fallback, and
// pdfcpu, descending too, answers nothing at that page's number.
func flatWithKidsLeaf(n, at int) []byte {
	objs := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"}
	var kids strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", 3+i)
		extra := ""
		if i == at {
			extra = fmt.Sprintf(" /Kids [%d 0 R %d 0 R]", 3+n, 4+n)
			objs[3+n] = fmt.Sprintf("<< /Type /Page /Parent %d 0 R >>", 3+i)
			objs[4+n] = fmt.Sprintf("<< /Type /Page /Parent %d 0 R >>", 3+i)
		}
		objs[3+i] = "<< /Type /Page /Parent 2 0 R" + extra + " >>"
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>", kids.String(), n)
	return testpdf.Assemble(objs)
}

// TestTheDigestFallbackWalksThePageTreeOnce — /pending 818's sibling. The digest's fallback asked `PageDict` page by
// page, quadratic on a flat tree, and one leaf carrying `/Kids` reaches it: with that leaf last, every page before it
// is hashed through the fallback. It now asks `pdfread.Pages`, one walk, and answers what the per-page loop answered.
func TestTheDigestFallbackWalksThePageTreeOnce(t *testing.T) {
	small := flatWithKidsLeaf(40, 39)
	_, st, got := contentDigest(small)
	_, want := referenceContentDigest(small)
	// The reference is the loop as it stood before /pending 488 (its wording of this refusal was later repaired), so
	// the comparison is where each refuses: the same page.
	if st.fastPath || got == nil || want == nil || !strings.HasPrefix(got.Error(), "page 40 is unreadable") ||
		!strings.HasPrefix(want.Error(), "page 40 is unreadable") {
		t.Fatalf("fast path %v; the fallback answered %v, the per-page reference %v", st.fastPath, got, want)
	}
	// Read through `Validated`, not `ReadOptimized`: the optimize pass is itself quadratic on ANY flat tree (pdfcpu's
	// `optimizeResourceDicts` calls `PageDict(i, true)` per page — 4.1 s at 5,000 clean pages, measured), which is
	// not this door's cost and would hide it.
	const n = 20000
	ctx, err := pdfread.Validated(flatWithKidsLeaf(n, n-1), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	st = &digestStats{}
	t0 := time.Now()
	_, err = digestReadContext(ctx, st)
	el := time.Since(t0)
	if st.fastPath || err == nil || !strings.Contains(err.Error(), "no page at that number") {
		t.Fatalf("setup: the digest did not take its fallback to the last page (fast path %v, %v)", st.fastPath, err)
	}
	// A ceiling far from both ends: one walk measured well under a second here, PageDict per page about two minutes.
	if el > 20*time.Second {
		t.Fatalf("%d pages through the fallback took %v — it is asking PageDict page by page", n, el)
	}
	t.Logf("%d pages through the digest's fallback: %v", n, el)
}
