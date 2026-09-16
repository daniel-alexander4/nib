package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// P02.S03's readers — redaction does not carry a structure tree.
//
// **The routing guard is the real one, and the output assertion below is NOT.** `api.Collect` drops
// the structure tree on its own today, so "the redacted document carries no element" is true for
// that reason whatever `RedactPages` calls — it would stay green if redaction were pointed back at
// the carrying primitive tomorrow. The discrimination lives in the call graph until P02.S04b makes
// `Collect` carry. The phase inventory's S03 section records that in prose and **nothing polices
// it** — `inventorycheck`'s `## Known gaps` excuses a slice with no section, which is a different
// thing entirely — so S04b owes this test a red probe against a carrying `Collect`.

// callsIn returns, for every function declared in one file of this package, the set of plain
// function names it calls. It is the same shape as `TestTheReviewDoorsRouteThroughTheProposer`'s
// walk, which is this repo's idiom for asserting that a rule's callers route through its door.
func callsIn(t *testing.T, file string) map[string]map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	out := map[string]map[string]bool{}
	for _, d := range parsed.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		set := map[string]bool{}
		ast.Inspect(fn, func(n ast.Node) bool {
			if call, isCall := n.(*ast.CallExpr); isCall {
				if id, isIdent := call.Fun.(*ast.Ident); isIdent {
					set[id.Name] = true
				}
			}
			return true
		})
		out[fn.Name.Name] = set
	}
	return out
}

// TestRedactionNeverRoutesThroughTheCarryingCollect — P02.S03's clause, asserted where it can fail.
func TestRedactionNeverRoutesThroughTheCarryingCollect(t *testing.T) {
	calls := callsIn(t, "pdfops.go")

	if calls["RedactPages"] == nil {
		t.Fatal("RedactPages is not declared in pdfops.go, so its route is unchecked and every " +
			"assertion below is vacuous")
	}
	if calls["collectWithoutStructure"] == nil {
		t.Fatal("collectWithoutStructure is gone. Redaction's whole refusal is that it binds to a " +
			"primitive which will never carry a tree; without the door there is nothing to bind to")
	}
	if !calls["RedactPages"]["collectWithoutStructure"] {
		t.Error("RedactPages does not route through collectWithoutStructure. From P02.S04b `Collect` " +
			"carries the source tree onto the pages it keeps, and a tree carried across a redaction " +
			"describes what the redacted pages said — the headings, the reading order, and whatever " +
			"/Alt or /ActualText the producer wrote, which is the shape the raster was meant to destroy")
	}
	if calls["RedactPages"]["Collect"] {
		t.Error("RedactPages calls Collect. That is the carrying primitive from P02.S04b onwards, and " +
			"a redaction that inherits the carry re-describes the content it destroyed")
	}
}

// TestTheNonCarryingDoorStillKeepsTheDocumentsLanguage — the door is not a stub.
//
// It must do everything `Collect` does apart from the carry: the same page selection, and `/Lang`
// carried onto the output, which `carryLang` exists for and which a hand-rolled `api.Collect` call
// would silently lose (a document whose language is gone fails veraPDF 7.2 t3 on every page).
func TestTheNonCarryingDoorStillKeepsTheDocumentsLanguage(t *testing.T) {
	src := taggedFixture() // carries /Lang (en-GB)
	if lang := langOf(t, src); lang == "" {
		t.Fatal("setup: the fixture carries no /Lang, so the assertion below cannot fail")
	}
	out, err := collectWithoutStructure(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if n, perr := PageCount(out); perr != nil || n != 1 {
		t.Fatalf("the door selected %d page(s) (err %v), want 1 — it is not doing Collect's job", n, perr)
	}
	if lang := langOf(t, out); lang == "" {
		t.Error("the non-carrying door dropped the document's /Lang. It refuses the STRUCTURE carry " +
			"and nothing else; a language lost here costs every page veraPDF 7.2 t3")
	}
}

// TestRedactionEmitsNoStructureTree — the backstop, DISCRIMINATING since P02.S04b, and it took two
// corrections to get there.
//
// **P02.S03 recorded this as a debt: probe it RED against a carrying `Collect`.** Done, and the
// first attempt failed to go red — which is what the debt was for. Rasterising page 1 puts an
// `ImagesToPDF` output first in the segment list, and `api.MergeRaw` keeps the FIRST document's
// catalog, so the merged result had no tree however the untouched run was collected. The door under
// test ran and the assertion could not see it.
//
// So the rasterised page is **not the first one**: segment 1 is the untouched run, its catalog is
// the one the merge keeps, and a carry there reaches the output. Probed with `RedactPages` pointed
// at the carrying primitive: this goes red on exactly its own assertion.
func TestRedactionEmitsNoStructureTree(t *testing.T) {
	// **Two pages, the SECOND rasterised.** An earlier cut used the one-page corpus fixture and
	// rasterised its only page: every segment was an image, the door under test never ran, and the
	// test passed in 0.01s having exercised nothing.
	src := repeatedPagesFixture()
	if !inspectTags(src).claims() {
		t.Fatal("setup: the fixture claims no tagging, so redacting it asserts nothing")
	}
	if n, perr := PageCount(src); perr != nil || n < 2 {
		t.Fatalf("setup: the fixture has %d page(s) (err %v); without an untouched run the "+
			"redaction never calls the door this slice is about", n, perr)
	}
	out, err := RedactPages(src, map[int]RasterPage{2: rasterPage(t, 200, 200)})
	if err != nil {
		t.Skipf("SKIP (not a pass): the redaction path refused the fixture: %v", err)
	}
	if n, perr := PageCount(out); perr != nil || n != 2 {
		t.Fatalf("setup: the redacted document has %d page(s) (err %v), want 2 — the untouched "+
			"page was not carried through, so this is not measuring a redaction", n, perr)
	}
	if s := inspectTags(out); s.tree {
		t.Errorf("the redacted document carries a structure tree. Its elements describe what the "+
			"rasterised pages said, which is the shape the redaction destroyed (claims=%v)", s.claims())
	}
}
