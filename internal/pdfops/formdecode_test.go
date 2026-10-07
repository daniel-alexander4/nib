package pdfops

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"nib/internal/pdfread"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 742`: pdfops' form walkers decoded a form afresh at every `Do` — pdfcpu's `DereferenceStreamDict`
// hands back a copy, so nothing cached the decode — and `formWalkBudget` is charged the DECODED size, so the
// decode ran before any refusal and on after it. Measured on the old code: one page drawing a 64 MiB flate
// form of spaces 80 times (a 66 KB file) took 6.6 s in `readPageRuns` and 13.4 s in `formDrawCounts` to
// refuse; and `formDrawCounts` re-read a form's whole body for its MCIDs at every draw, including the draws
// of a form drawing itself, which are charged nothing — a 140 KB form of 20,000 `/Fm Do` took 92 s and
// returned no error. `formWalkBudget.formContent` is now the one decode door, once per object per walk.

// flated is body flate-compressed.
func flated(body []byte) string {
	var z bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	zw.Write(body)
	zw.Close()
	return z.String()
}

// formDrawnTimes is one page drawing the flate-compressed form `body` `draws` times.
func formDrawnTimes(body []byte, draws int) []byte {
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm 9 0 R >> " +
			"/Font << /F1 8 0 R >> >> /Contents 4 0 R >>",
		4: optStream(strings.Repeat("/Fm Do ", draws), ""),
		8: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		9: optStream(flated(body), "/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Filter /FlateDecode"),
	})
}

func optimizedCtx(t *testing.T, pdf []byte) *model.Context {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

// pageOne is page 1's content and resources, as each walker is entered with them.
func pageOne(t *testing.T, ctx *model.Context) ([]byte, types.Dict) {
	t.Helper()
	d, _, attrs, err := ctx.PageDict(1, false)
	if err != nil || d == nil {
		t.Fatalf("page 1: %v", err)
	}
	src, err := pdfread.PageContent(ctx, d, 1)
	if err != nil {
		t.Fatalf("page 1 content: %v", err)
	}
	return src, attrs.Resources
}

// TestAFormDrawnRepeatedlyIsDecodedOnceByEveryWalker is the item's own case with the time taken out: 50 draws
// of one form cost ONE decode in each of the three walkers, and every draw is still walked.
func TestAFormDrawnRepeatedlyIsDecodedOnceByEveryWalker(t *testing.T) {
	const draws = 50
	ctx := optimizedCtx(t, formDrawnTimes([]byte("BT /F1 1 Tf (a) Tj ET 0 0 1 1 re f"), draws))
	src, res := pageOne(t, ctx)

	w := newRunWalker(ctx.XRefTable)
	w.walk(src, res, newRunGState(), 0, map[int]bool{})
	if err := w.budget.err(); err != nil || len(w.runs) != draws {
		t.Fatalf("control: the run reader read %d runs (%v), want %d — every draw walked", len(w.runs), err, draws)
	}
	if w.budget.decodes != 1 {
		t.Errorf("the run reader decoded a form drawn %d times %d times, want once", draws, w.budget.decodes)
	}

	counts := map[int]formDraw{}
	b := newFormWalkBudget(1)
	countFormDraws(ctx, src, res, counts, map[int]bool{}, 0, b)
	if err := b.err(); err != nil || counts[9].count != draws {
		t.Fatalf("control: formDrawCounts counted %d draws (%v), want %d", counts[9].count, err, draws)
	}
	if b.decodes != 1 {
		t.Errorf("formDrawCounts decoded a form drawn %d times %d times, want once", draws, b.decodes)
	}

	b = newFormWalkBudget(1)
	if n := countDrawings(ctx, src, res, 0, map[int]bool{}, b); n != draws || b.err() != nil {
		t.Fatalf("control: uncoveredDrawings counted %d (%v), want %d", n, b.err(), draws)
	}
	if b.decodes != 1 {
		t.Errorf("uncoveredDrawings decoded a form drawn %d times %d times, want once", draws, b.decodes)
	}
}

// TestFormWalkersDecodeThroughTheOneDoor is ADR-009's guard for `formContent`: outside it, the only production
// caller of `streamContent` is the run reader's `/ToUnicode` read, which is a font's, cached per font, and no form.
// A walker added later that decodes a form itself would re-decode it at every draw and past the refusal.
func TestFormWalkersDecodeThroughTheOneDoor(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// The door decodes through pdfread's capped decode (/pending 748), and no other pdfops file does.
	allowed := map[string]int{"textrun.go": 1}
	// fontink.go's is a font's embedded program, read once per font for its glyphs' reach (ADR-098) — no form.
	capped := map[string]int{"walkbudget.go": 1, "fontink.go": 1}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		n := strings.Count(string(src), "streamContent(") - strings.Count(string(src), "func streamContent(")
		if n != allowed[f] {
			t.Errorf("%s calls streamContent %d times, want %d — a form's bytes are read through formWalkBudget.formContent", f, n, allowed[f])
		}
		if n := strings.Count(string(src), "pdfread.DecodeWithin("); n != capped[f] {
			t.Errorf("%s calls pdfread.DecodeWithin %d times, want %d — formWalkBudget.formContent is the door", f, n, capped[f])
		}
	}
}

// TestNothingIsDecodedOnceTheBudgetHasRefused: 40 DISTINCT 1 MiB forms, so the cache cannot help — the walk
// is refused a little past 20 MiB, and the forms after the refusal are never decoded, only named.
func TestNothingIsDecodedOnceTheBudgetHasRefused(t *testing.T) {
	const forms = 40
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		8: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var names, page strings.Builder
	for i := 0; i < forms; i++ {
		fmt.Fprintf(&names, "/F%d %d 0 R ", i, 100+i)
		fmt.Fprintf(&page, "/F%d Do ", i)
		body := append(bytes.Repeat([]byte{' '}, 1<<20), fmt.Sprintf("%% form %d", i)...)
		objs[100+i] = optStream(flated(body), "/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Filter /FlateDecode")
	}
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << " + names.String() +
		">> >> /Contents 4 0 R >>"
	objs[4] = optStream(page.String(), "")
	ctx := optimizedCtx(t, assembleFixture(objs))
	src, res := pageOne(t, ctx)

	budget := newFormWalkBudget(1)
	limit := budget.maxBytes/(1<<20) + 1 // the decode that crosses the budget is the last one
	if limit >= forms {
		t.Fatalf("setup: %d forms of 1 MiB do not cross a %d-byte budget", forms, budget.maxBytes)
	}
	w := newRunWalker(ctx.XRefTable)
	w.walk(src, res, newRunGState(), 0, map[int]bool{})
	if w.budget.err() == nil {
		t.Fatal("control: 40 MiB of forms on one page was read in full, want the byte budget to refuse it")
	}
	if w.budget.decodes > limit {
		t.Errorf("the run reader decoded %d forms, want at most %d — nothing past the refusal", w.budget.decodes, limit)
	}
	counts := map[int]formDraw{}
	b := newFormWalkBudget(1)
	countFormDraws(ctx, src, res, counts, map[int]bool{}, 0, b)
	if b.err() == nil || b.decodes > limit {
		t.Errorf("formDrawCounts decoded %d forms (%v), want the byte budget named and at most %d decodes", b.decodes, b.err(), limit)
	}
}

// TestAHugeFormDrawnRepeatedlyIsRefusedPromptly is the measured case: 6.6 s and 13.4 s before, ~0.1 s after.
// The refusal still names the byte budget, since every draw is still charged.
func TestAHugeFormDrawnRepeatedlyIsRefusedPromptly(t *testing.T) {
	pdf := formDrawnTimes(bytes.Repeat([]byte{' '}, 64<<20), 80)
	ctx := optimizedCtx(t, pdf)
	const why = "bytes of form XObjects"
	var err error
	finishesWithin(t, 3, "readPageRuns over 80 draws of a 64 MiB form", func() { _, err = readPageRuns(ctx, pageAt(ctx, nil, 1)) })
	if err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("readPageRuns returned %v, want the byte budget named", err)
	}
	finishesWithin(t, 3, "formDrawCounts over 80 draws of a 64 MiB form", func() { _, err = formDrawCounts(ctx) })
	if err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("formDrawCounts returned %v, want the byte budget named", err)
	}
	finishesWithin(t, 3, "uncoveredDrawings over 80 draws of a 64 MiB form", func() { _, err = uncoveredDrawings(pdf) })
	if err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("uncoveredDrawings returned %v, want the byte budget named", err)
	}
}

// TestAFormDrawingItselfIsCountedInLinearTime: a form's draws of itself are counted and not followed, so they
// are charged nothing — and each one re-read the whole body for its MCIDs. 92 s before, milliseconds after,
// with every draw still counted.
func TestAFormDrawingItselfIsCountedInLinearTime(t *testing.T) {
	const n = 20000
	ctx := optimizedCtx(t, assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm 9 0 R >> >> /Contents 4 0 R >>",
		4: optStream("/Fm Do", ""),
		9: optStream(strings.Repeat("/Fm Do ", n), "/Type /XObject /Subtype /Form /BBox [0 0 10 10]"),
	}))
	var counts map[int]formDraw
	var err error
	finishesWithin(t, 5, "formDrawCounts over a form drawing itself 20,000 times", func() { counts, err = formDrawCounts(ctx) })
	if err != nil || counts[9].count != n+1 {
		t.Fatalf("formDrawCounts counted %d draws (%v), want %d — the page's and each of the form's own", counts[9].count, err, n+1)
	}
}
