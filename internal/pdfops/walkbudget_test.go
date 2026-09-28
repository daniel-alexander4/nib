package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// `/pending 664`: pdfops' three form walkers bounded depth only, so a form drawing the next ten times cost
// ×10 per level from a file of two kilobytes — measured at five levels 1.4 s (`formDrawCounts`), 0.9 s
// (`uncoveredDrawings`), 1.2 s (`readPageRuns`). Each must now stop at `formWalkBudget` and SAY so.

// walkFanOut is a tagged page drawing X0, each Xi drawing X(i+1) ten times, the last showing one glyph and
// filling one rectangle: 10^depth leaf walks.
func walkFanOut(depth int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X0 100 0 R >> >> " +
			"/Contents 4 0 R >>",
		4: optStream("/X0 Do", ""),
		7: "<< /Type /StructTreeRoot >>",
		8: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	for i := 0; i < depth; i++ {
		objs[100+i] = optStream(strings.Repeat(fmt.Sprintf("/X%d Do ", i+1), 10),
			fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /XObject << /X%d %d 0 R >> >>",
				i+1, 101+i))
	}
	objs[100+depth] = optStream("BT /F1 1 Tf (a) Tj ET 0 0 1 1 re f",
		"/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /Font << /F1 8 0 R >> >>")
	return assembleFixture(objs)
}

func TestEveryFormWalkerStopsAtTheWalkBudgetAndSaysSo(t *testing.T) {
	read := func(pdf []byte) *model.Context {
		ctx, err := readOptimized(pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		return ctx
	}
	// Stimulus first: the fan-out multiplies. Three levels are read in full by every walker, and each sees
	// the leaf 10^3 times — so seven levels is 10^7, far past any budget.
	control := walkFanOut(3)
	ctx := read(control)
	draws, err := formDrawCounts(ctx)
	if err != nil || draws[103].count != 1000 {
		t.Fatalf("control: the leaf form is counted drawn %d times (%v), want 1000", draws[103].count, err)
	}
	if n, err := uncoveredDrawings(control); err != nil || n != 1000 {
		t.Fatalf("control: %d uncovered drawings (%v), want 1000", n, err)
	}
	if pr, err := readPageRuns(ctx, 1); err != nil || len(pr.runs) != 1000 {
		t.Fatalf("control: %d runs (%v), want 1000", len(pr.runs), err)
	}
	if !carryIsComplete(control) {
		t.Fatal("control: three levels of forms carrying no marked content read as an incomplete carry")
	}

	hostile := walkFanOut(7)
	ctx = read(hostile)
	const budget = "enters form XObjects more than"
	finishesWithin(t, 10, "formDrawCounts over seven levels", func() { _, err = formDrawCounts(ctx) })
	if err == nil || !strings.Contains(err.Error(), budget) {
		t.Errorf("formDrawCounts over seven levels returned %v, want the walk budget named", err)
	}
	finishesWithin(t, 10, "uncoveredDrawings over seven levels", func() { _, err = uncoveredDrawings(hostile) })
	if err == nil || !strings.Contains(err.Error(), budget) {
		t.Errorf("uncoveredDrawings over seven levels returned %v, want the walk budget named", err)
	}
	finishesWithin(t, 10, "readPageRuns over seven levels", func() { _, err = readPageRuns(ctx, 1) })
	if err == nil || !strings.Contains(err.Error(), budget) {
		t.Errorf("readPageRuns over seven levels returned %v, want the walk budget named", err)
	}
	// The carry gate's condition 4 cannot be answered from partial counts, so it is a defect, not a pass.
	var complete bool
	finishesWithin(t, 10, "carryIsComplete over seven levels", func() { complete = carryIsComplete(hostile) })
	if complete {
		t.Error("carryIsComplete over seven levels of forms answered complete from counts it never finished")
	}
}

// TestNestedSequencesCostTheRunReaderLinearTime — `/pending 664`'s R3-4: every show asked whether an
// `/Artifact` was open by scanning the whole marked-content stack, and every `Do` marked every open
// sequence, so N nested sequences and N operators cost N². Measured before: 80,000 of each, 8.2 s.
func TestNestedSequencesCostTheRunReaderLinearTime(t *testing.T) {
	const n = 200_000
	page := func(content string, xobj string) []byte {
		return assembleFixture(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 8 0 R >> " +
				xobj + " >> /Contents 4 0 R >>",
			4: optStream(content, ""),
			8: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
			9: optStream("", "/Type /XObject /Subtype /Form /BBox [0 0 10 10]"),
		})
	}
	// n shows inside n plain sequences and one artifact, which sits at the BOTTOM (so the question "what
	// MCID is in force" searches down through every plain one) and then at the TOP (so "is an artifact open"
	// searches up through every plain one). Every run is inside the artifact either way.
	shows := strings.Repeat("(a) Tj ", n)
	for _, where := range []struct{ name, content string }{
		{"under", "/Artifact BMC " + strings.Repeat("/P BMC ", n) + "BT /F1 1 Tf " + shows + "ET"},
		{"over", strings.Repeat("/P BMC ", n) + "/Artifact BMC BT /F1 1 Tf " + shows + "ET"},
	} {
		ctx, err := readOptimized(page(where.content, ""), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		var pr pageRuns
		finishesWithin(t, 10, fmt.Sprintf("%d shows with the artifact %s %d nested sequences", n, where.name, n),
			func() { pr, err = readPageRuns(ctx, 1) })
		if err != nil || len(pr.runs) != n {
			t.Fatalf("%d runs (%v), want %d", len(pr.runs), err, n)
		}
		for _, r := range []textRun{pr.runs[0], pr.runs[n-1]} {
			if !r.artifact || r.mcid != -1 {
				t.Fatalf("a run inside an /Artifact sequence (%s the rest) reads artifact=%v mcid=%d, want true and -1",
					where.name, r.artifact, r.mcid)
			}
		}
	}

	// n nested sequences carrying MCIDs, then n draws of a form: every sequence draws a form, and the
	// draws past the walk budget are refused, but they must be refused in linear time.
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "/P <</MCID %d>> BDC ", i)
	}
	b.WriteString(strings.Repeat("/Fm Do ", n))
	draws := page(b.String(), "/XObject << /Fm 9 0 R >>")
	ctx, err := readOptimized(draws, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	finishesWithin(t, 10, fmt.Sprintf("%d form draws inside %d nested sequences", n, n), func() { _, err = readPageRuns(ctx, 1) })
	if err == nil || !strings.Contains(err.Error(), "enters form XObjects more than") {
		t.Fatalf("%d form draws on one page returned %v, want the walk budget named", n, err)
	}
}

// TestAFormDrawnManyTimesStopsAtTheByteBudget — the other way to amplify: few walks of one large form.
// 2,000 draws of a 16 KiB form is under the walk budget and 32 MiB of content read.
func TestAFormDrawnManyTimesStopsAtTheByteBudget(t *testing.T) {
	body := strings.Repeat("0 0 1 1 re f ", 16<<10/13)
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm 9 0 R >> >> /Contents 4 0 R >>",
		4: optStream(strings.Repeat("/Fm Do ", 2000), ""),
		9: optStream(body, "/Type /XObject /Subtype /Form /BBox [0 0 10 10]"),
	})
	ctx, err := readOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: under the walk budget, over the byte budget.
	if b := newFormWalkBudget(1); 2000 > b.maxWalks || 2000*len(body) <= b.maxBytes {
		t.Fatalf("setup: 2000 walks of %d bytes against budgets of %d walks and %d bytes does not isolate the byte budget",
			len(body), b.maxWalks, b.maxBytes)
	}
	const why = "bytes of form XObjects"
	if _, err := formDrawCounts(ctx); err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("formDrawCounts returned %v, want the byte budget named", err)
	}
	if _, err := uncoveredDrawings(pdf); err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("uncoveredDrawings returned %v, want the byte budget named", err)
	}
	if _, err := readPageRuns(ctx, 1); err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("readPageRuns returned %v, want the byte budget named", err)
	}
}
