package pdfops

import (
	"bytes"
	"compress/zlib"
	"runtime"
	"strings"
	"testing"
)

// flatedSpaces is `size` spaces flate-compressed, written a MiB at a time so the fixture never holds them whole.
func flatedSpaces(size int) string {
	var z bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	chunk := bytes.Repeat([]byte{' '}, 1<<20)
	for left := size; left > 0; left -= len(chunk) {
		zw.Write(chunk[:min(left, len(chunk))])
	}
	zw.Close()
	return z.String()
}

// allocated is the heap f allocates, in bytes.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestAFormPastTheBudgetIsRefusedWithoutBeingInflated — `/pending 748`: the byte budget was charged with a form's
// DECODED size after the decode, so one ~600 KB form drawn once inflated to pdfcpu's own 512 MiB ceiling (1.8 GiB
// peak heap) before a 20 MiB budget could refuse it — and, past that ceiling, came back undecodable and was
// skipped with NO error, so each walker described a page it had not read. Now each walker is refused, naming the
// byte budget, having decoded about what the budget had left.
func TestAFormPastTheBudgetIsRefusedWithoutBeingInflated(t *testing.T) {
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm 9 0 R >> >> /Contents 4 0 R >>",
		4: optStream("/Fm Do ", ""),
		9: optStream(flatedSpaces(600<<20), "/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Filter /FlateDecode"),
	})
	ctx := optimizedCtx(t, pdf)
	src, res := pageOne(t, ctx)
	const most = 100 << 20 // the budget is 20 MiB; a decode grows its buffer to about twice what it holds
	check := func(walker string, walk func() error) {
		t.Helper()
		var err error
		if alloc := allocated(func() { err = walk() }); alloc > most {
			t.Errorf("%s allocated %d MiB to refuse a 600 MiB form under a %d-byte budget, want under %d MiB",
				walker, alloc>>20, newFormWalkBudget(1).maxBytes, most>>20)
		}
		if err == nil || !strings.Contains(err.Error(), "bytes of form XObjects") {
			t.Errorf("%s answered %v for a page drawing a 600 MiB form, want the byte budget named", walker, err)
		}
	}
	check("the run reader", func() error {
		w := newRunWalker(ctx.XRefTable)
		w.walk(src, res, newRunGState(), 0, map[int]bool{})
		return w.budget.err()
	})
	check("formDrawCounts", func() error {
		b := newFormWalkBudget(1)
		countFormDraws(ctx, src, res, map[int]formDraw{}, map[int]bool{}, 0, b)
		return b.err()
	})
	check("uncoveredDrawings", func() error {
		b := newFormWalkBudget(1)
		countDrawings(ctx, src, res, 0, map[int]bool{}, b)
		return b.err()
	})
}
