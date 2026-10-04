package pdfops

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/scaling"
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
	// 4× the pages: linear ×4, a per-page root walk ×16; ×8 is the log midpoint. (It was 2× against ×3, and
	// tipped ×4.07 once in a loaded parallel run; measured one size after the other it still ranged ×2.1-×6.8 at
	// load ~15, so it is counted first and the clock's sizes are interleaved through `scaling` — /pending 799.)
	prep := func(n int) func() {
		ctx := flatPages(t, n)
		return func() {
			if got := len(scanPages(ctx)); got != n {
				t.Fatalf("scanPages found %d of %d pages", got, n)
			}
			if got := len(livePageObjects(ctx)); got != n {
				t.Fatalf("livePageObjects found %d of %d pages", got, n)
			}
		}
	}
	scaling.AllocsGrowLinearly(t, "scanPages+livePageObjects", 3000, 12000, 8, prep)
	scaling.GrowsLinearly(t, "scanPages+livePageObjects", 3000, 12000, 8, func(n int) time.Duration {
		return scaling.TimeOnce(prep(n))
	})
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
