package pdfops

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// `/pending 715`: the residue of 664's walk budget — a budget per page where a loop reads many, and a depth cut
// that said nothing.

func readForWalk(t *testing.T, pdf []byte) *model.Context {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

// pagesDrawingOneFanOut is n pages that all draw one shared form fanning out ten per level for depth levels —
// 11,110 form walks a page at depth 4, under a page's budget and far under n pages' of them.
func pagesDrawingOneFanOut(n, depth int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		4: optStream("/X0 Do", ""),
		5: "<< /XObject << /X0 100 0 R >> >>",
		7: "<< /Type /StructTreeRoot >>",
		8: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var kids strings.Builder
	for p := 0; p < n; p++ {
		objs[10000+p] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources 5 0 R /Contents 4 0 R >>"
		fmt.Fprintf(&kids, "%d 0 R ", 10000+p)
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), n)
	for i := 0; i < depth; i++ {
		objs[100+i] = optStream(strings.Repeat(fmt.Sprintf("/X%d Do ", i+1), 10),
			fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /XObject << /X%d %d 0 R >> >>",
				i+1, 101+i))
	}
	objs[100+depth] = optStream("BT /F1 1 Tf (a) Tj ET 0 0 1 1 re f",
		"/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /Font << /F1 8 0 R >> >>")
	return assembleFixture(objs)
}

// TestAPageLoopPaysOneBudgetForTheDocument — `readPageRuns` took a budget per page, so a loop over the pages of a
// document whose every page stayed just under it paid a budget a page. Measured before: 64 pages sharing one
// fan-out form, `proposeStructure` 9.4 s, ~0.14 s a page for ~100 bytes of file a page.
func TestAPageLoopPaysOneBudgetForTheDocument(t *testing.T) {
	// Control: one such page is read in full, alone and through the loop.
	onePDF := pagesDrawingOneFanOut(1, 4)
	one := readForWalk(t, onePDF)
	if pr, err := readPageRuns(one, pageAt(one, nil, 1)); err != nil || len(pr.runs) != 10000 {
		t.Fatalf("control: one page read %d runs (%v), want 10000 — a page alone is under its budget", len(pr.runs), err)
	}
	if _, err := proposeStructure(one); err != nil {
		t.Fatalf("control: proposeStructure over one page: %v", err)
	}
	if _, err := readStructureView(onePDF); err != nil {
		t.Fatalf("control: the structure view over one page: %v", err)
	}
	// Stimulus: sixty-four pages are each under a page's budget and together far over the document's.
	const pages, perPage = 64, 11110
	if perPage > newFormWalkBudget(1).maxWalks || perPage*pages <= newFormWalkBudget(pages).maxWalks {
		t.Fatalf("setup: %d walks a page against a page budget of %d and a document budget of %d does not "+
			"isolate the per-page budget", perPage, newFormWalkBudget(1).maxWalks, newFormWalkBudget(pages).maxWalks)
	}
	pdf := pagesDrawingOneFanOut(pages, 4)
	ctx := readForWalk(t, pdf)
	const why = "enters form XObjects more than"
	var err error
	finishesWithin(t, 20, "proposeStructure over 64 pages", func() { _, err = proposeStructure(ctx) })
	if err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("proposeStructure over %d pages each drawing %d forms returned %v, want the document's walk "+
			"budget named — a budget per page lets the document cost one a page", pages, perPage, err)
	}
	finishesWithin(t, 20, "the structure view over 64 pages", func() { _, err = readStructureView(pdf) })
	if err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("the structure view over %d pages returned %v, want the document's walk budget named", pages, err)
	}
}

// formNest is a page drawing X0, each Xi drawing X(i+1) once, depth forms in all, the last showing a glyph and
// filling a rectangle — one path, so nothing but the depth is reached.
func formNest(depth int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X0 100 0 R >> >> " +
			"/Contents 4 0 R >>",
		4: optStream("/X0 Do", ""),
		7: "<< /Type /StructTreeRoot >>",
		8: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	for i := 0; i < depth-1; i++ {
		objs[100+i] = optStream(fmt.Sprintf("/X%d Do", i+1),
			fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /XObject << /X%d %d 0 R >> >>",
				i+1, 101+i))
	}
	objs[100+depth-1] = optStream("BT /F1 1 Tf (a) Tj ET 0 0 1 1 re f",
		"/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /Font << /F1 8 0 R >> >>")
	return assembleFixture(objs)
}

// TestAFormNestedPastTheDepthIsNamedNotDropped — the three walkers stopped SILENTLY at their depth bound, so what
// the deeper forms drew was never read while the runs and counts built above it read as the whole page, the
// defect the walk budget refuses. The cut is now the budget's (`deeper`) and says so.
func TestAFormNestedPastTheDepthIsNamedNotDropped(t *testing.T) {
	// Control: at each walker's own bound the leaf is reached and nothing is refused.
	atRuns := formNest(maxFormDepth)
	ctx := readForWalk(t, atRuns)
	if pr, err := readPageRuns(ctx, pageAt(ctx, nil, 1)); err != nil || len(pr.runs) != 1 {
		t.Fatalf("control: %d forms deep read %d runs (%v), want the leaf's 1", maxFormDepth, len(pr.runs), err)
	}
	if n, err := uncoveredDrawings(atRuns); err != nil || n != 1 {
		t.Fatalf("control: %d forms deep, %d uncovered drawings (%v), want 1", maxFormDepth, n, err)
	}
	ctx = readForWalk(t, formNest(maxFormDrawDepth))
	if draws, err := formDrawCounts(ctx); err != nil || draws[100+maxFormDrawDepth-1].count != 1 {
		t.Fatalf("control: %d forms deep, the leaf is counted %d (%v), want 1", maxFormDrawDepth,
			draws[100+maxFormDrawDepth-1].count, err)
	}

	const why = "nests form XObjects more than"
	past := formNest(maxFormDepth + 1)
	ctx = readForWalk(t, past)
	if pr, err := readPageRuns(ctx, pageAt(ctx, nil, 1)); err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("readPageRuns %d forms deep returned %d runs and %v, want the depth named — the leaf's text was "+
			"never read", maxFormDepth+1, len(pr.runs), err)
	}
	if n, err := uncoveredDrawings(past); err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("uncoveredDrawings %d forms deep returned %d and %v, want the depth named", maxFormDepth+1, n, err)
	}
	ctx = readForWalk(t, formNest(maxFormDrawDepth+1))
	if _, err := formDrawCounts(ctx); err == nil || !strings.Contains(err.Error(), why) {
		t.Errorf("formDrawCounts %d forms deep returned %v, want the depth named", maxFormDrawDepth+1, err)
	}
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

// TestEveryPageLoopSharesOneWalkBudget — the guard: a call to the run reader inside a loop passes the loop's one
// budget (`readPageRuns`' shared argument), because a call without it is a budget per page, the measured defect.
// ADR-009: this checks the ROUTING, so a loop added later fails here rather than in a timing.
func TestEveryPageLoopSharesOneWalkBudget(t *testing.T) {
	readers := map[string]bool{"readPageRuns": true, "readPageLayout": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	inLoop := 0
	var visit func(n ast.Node, loop bool)
	visit = func(n ast.Node, loop bool) {
		ast.Inspect(n, func(c ast.Node) bool {
			if c == n || c == nil {
				return true
			}
			switch v := c.(type) {
			case *ast.ForStmt:
				visit(v.Body, true)
				return false
			case *ast.RangeStmt:
				visit(v.Body, true)
				return false
			case *ast.FuncLit:
				visit(v.Body, false) // a closure is its own scope of iteration
				return false
			case *ast.CallExpr:
				id, ok := v.Fun.(*ast.Ident)
				if !ok || !readers[id.Name] || !loop {
					return true
				}
				inLoop++
				if len(v.Args) < 3 || isNilIdent(v.Args[2]) {
					t.Errorf("%s calls %s inside a loop without the loop's budget — each page then gets a budget of "+
						"its own; make one `newFormWalkBudget(pages)` before the loop and pass it",
						fset.Position(v.Pos()), id.Name)
				}
			}
			return true
		})
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, f, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		visit(file, false)
	}
	// Stimulus: the census sees the five loops it exists for (proposer, structure view, commit, artifact, claim).
	if inLoop < 5 {
		t.Fatalf("setup: the census found %d run-reader calls inside loops, want at least 5 — it is not seeing them", inLoop)
	}
}
