package pdfops

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// flatPages is n pages under one /Pages node — the tree nib's Markdown conversion writes, and the one on which
// `ctx.PageDict(p)` per page is quadratic: pdfcpu dereferences every kid before the one it wants.
func flatPages(t *testing.T, n int) *model.Context {
	t.Helper()
	objs := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"}
	var kids strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", 3+i)
		objs[3+i] = "<< /Type /Page /Parent 2 0 R /Resources << >> >>"
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>", kids.String(), n)
	ctx, err := pdfread.Validated(testpdf.Assemble(objs), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

// /pending 753: preparing a 7,059-page document for a ceremony took ~50 s, most of it in per-page sweeps that
// each asked `PageDict` for every page — `scanPages` (the structure gate, `readHostTree`) and the `live` set
// seven doors build. Doubling the pages must not much more than double their cost; per-page `PageDict` on this
// tree quadruples it.
func TestThePerPageSweepsAreLinearInThePageCount(t *testing.T) {
	best := func(n int) time.Duration {
		ctx := flatPages(t, n)
		b := time.Duration(1 << 62)
		for i := 0; i < 5; i++ {
			runtime.GC()
			t0 := time.Now()
			if got := len(scanPages(ctx)); got != n {
				t.Fatalf("scanPages found %d of %d pages", got, n)
			}
			if got := len(livePageObjects(ctx)); got != n {
				t.Fatalf("livePageObjects found %d of %d pages", got, n)
			}
			b = min(b, time.Since(t0))
		}
		return b
	}
	small, large := best(3000), best(6000)
	t.Logf("3,000 pages %v, 6,000 pages %v (×%.2f)", small, large, float64(large)/float64(small))
	if large > 3*small {
		t.Fatalf("doubling the pages cost ×%.2f (%v → %v): a per-page sweep walks the page tree per page",
			float64(large)/float64(small), small, large)
	}
}

// The fold must not move what the sweeps see: every record is exactly PageDict's and PageDictIndRef's answer.
func TestScanPagesIsWhatPageDictAndPageDictIndRefAnswer(t *testing.T) {
	ctx := flatPages(t, 50)
	recs := scanPages(ctx)
	live := livePageObjects(ctx)
	for i, r := range recs {
		d, _, _, err := ctx.PageDict(i+1, false)
		ir, ierr := ctx.PageDictIndRef(i + 1)
		if err != nil || ierr != nil || r.nr != i+1 || fmt.Sprintf("%p", d) != fmt.Sprintf("%p", r.dict) || *ir != *r.ref {
			t.Fatalf("page %d: record %+v is not PageDict's answer", i+1, r)
		}
		if !live[ir.ObjectNumber.Value()] {
			t.Fatalf("page %d (object %d) missing from the live set", i+1, ir.ObjectNumber.Value())
		}
	}
	if len(live) != 50 {
		t.Fatalf("live set holds %d pages, want 50", len(live))
	}
}
