package pdfops

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure editor's dictionary edits — `PLAN-accessibility.md` P09.S02.

func viewOf(t *testing.T, pdf []byte) structureView {
	t.Helper()
	v, err := readStructureView(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// byID indexes a view by object number.
func byID(v structureView) map[int]viewElement {
	out := map[int]viewElement{}
	for _, e := range v.elements {
		if e.id != 0 {
			out[e.id] = e
		}
	}
	return out
}

// kidIDs is an element's kids by object number.
func kidIDs(v structureView, e viewElement) []int {
	var out []int
	for _, j := range e.kids {
		out = append(out, v.elements[j].id)
	}
	return out
}

// rawElement is an element's dictionary as the written document holds it.
func rawElement(t *testing.T, pdf []byte, objNr int) (*model.Context, types.Dict) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, err := ctx.DereferenceDict(*types.NewIndirectRef(objNr, 0))
	if err != nil || d == nil {
		t.Fatalf("object %d is not a dictionary: %v", objNr, err)
	}
	return ctx, d
}

// requireConsistent fails when the written tree contradicts itself.
func requireConsistent(t *testing.T, what string, pdf []byte) {
	t.Helper()
	if _, defects := checkTree(t, pdf); len(defects) > 0 {
		t.Errorf("%s left the tree inconsistent: %v", what, defects)
	}
}

// TestEachEditOnARealTreeDoesWhatItSaysAndNothingElse — every edit kind on LibreOffice's own trees:
// the edit is visible in the re-read view, every marked element keeps its type and text, and the
// consistency invariants hold.
func TestEachEditOnARealTreeDoesWhatItSaysAndNothingElse(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice is not installed, so there is no producer's tree to edit; the hand-built cases still run")
	}
	lists := truthCorpus(t)[0].pdf // Document › H1 P H2 P L P
	v := viewOf(t, lists)
	doc := v.elements[0]
	top := kidIDs(v, doc)
	if doc.kind != "Document" || len(top) != 6 {
		t.Fatalf("setup: the lists document's root reads %s with %d kids", doc.kind, len(top))
	}
	ids := byID(v)
	firstP, secondP, list, closing := top[1], top[3], top[4], top[5]
	if ids[list].standard != "L" || ids[firstP].standard != "P" {
		t.Fatalf("setup: kids read %s and %s", ids[list].standard, ids[firstP].standard)
	}

	// untouched asserts every marked element other than skip kept its type and text.
	untouched := func(what string, before, after structureView, skip int) {
		t.Helper()
		got := byID(after)
		for _, e := range before.elements {
			if !e.marked || e.id == skip {
				continue
			}
			if g := got[e.id]; g.kind != e.kind || g.text != e.text {
				t.Errorf("%s: element %d changed from %s %q to %s %q", what, e.id, e.kind, e.text, g.kind, g.text)
			}
		}
	}

	run := func(what string, src []byte, edits ...structEdit) structureView {
		t.Helper()
		out, err := applyStructEdits(src, edits)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		requireConsistent(t, what, out)
		return viewOf(t, out)
	}

	after := run("retype", lists, structEdit{kind: editRetype, elem: firstP, value: "H3"})
	if got := byID(after)[firstP]; got.kind != "H3" || got.text != ids[firstP].text {
		t.Errorf("retype: element reads %s %q", got.kind, got.text)
	}
	untouched("retype", v, after, firstP)

	after = run("alt", lists, structEdit{kind: editAlt, elem: top[0], value: "The document's title"})
	if got := byID(after)[top[0]]; !got.hasAlt || got.alt != "The document's title" {
		t.Errorf("alt: the heading reads alt %q (present %v)", got.alt, got.hasAlt)
	}
	untouched("alt", v, after, -1)

	after = run("reorder", lists, structEdit{kind: editMove, elem: closing, index: 0})
	want := append([]int{closing}, top[:5]...)
	if got := kidIDs(after, after.elements[0]); !equalInts(got, want) {
		t.Errorf("reorder: the root's kids read %v, want %v", got, want)
	}
	untouched("reorder", v, after, -1)

	after = run("re-parent", lists, structEdit{kind: editMove, elem: secondP, parent: list, index: -1})
	a := byID(after)
	if got := kidIDs(after, after.elements[0]); !equalInts(got, []int{top[0], top[1], top[2], list, closing}) {
		t.Errorf("re-parent: the root's kids read %v", got)
	}
	if got := kidIDs(after, a[list]); len(got) == 0 || got[len(got)-1] != secondP {
		t.Errorf("re-parent: the list's kids read %v, want %d last", got, secondP)
	}
	if moved := a[secondP]; after.elements[moved.parent].id != list {
		t.Errorf("re-parent: the paragraph's parent reads %d", after.elements[moved.parent].id)
	}
	if _, raw := rawElement(t, mustApply(t, lists, structEdit{kind: editMove, elem: secondP, parent: list, index: -1}), secondP); raw["P"].(types.IndirectRef).ObjectNumber.Value() != list {
		t.Errorf("re-parent: the paragraph's /P names %v, not the list", raw["P"])
	}
	untouched("re-parent", v, after, -1)

	after = run("a batch", lists,
		structEdit{kind: editRetype, elem: firstP, value: "H3"},
		structEdit{kind: editMove, elem: firstP, index: 0})
	if got := kidIDs(after, after.elements[0]); got[0] != firstP || byID(after)[firstP].kind != "H3" {
		t.Errorf("a batch: the root's kids read %v and the moved element %s", got, byID(after)[firstP].kind)
	}

	table := tableAndFigureODT(t)
	tv := viewOf(t, table)
	var headers []int
	for _, e := range tv.elements {
		if e.standard == "TH" {
			headers = append(headers, e.id)
		}
	}
	after = run("scope", table, structEdit{kind: editScope, elem: headers[1], value: "Row"})
	if got := byID(after); got[headers[1]].scope != "Row" || got[headers[0]].scope != "Column" {
		t.Errorf("scope: header cells read %q and %q, want Column and Row", got[headers[0]].scope, got[headers[1]].scope)
	}
	untouched("scope", tv, after, -1)
}

func mustApply(t *testing.T, pdf []byte, edits ...structEdit) []byte {
	t.Helper()
	out, err := applyStructEdits(pdf, edits)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestRestoringAltAndScopeClearsTheirUA1Clauses — the phase-open measurement, closed by the editor:
// a figure with no `/Alt` fails 7.3 t1 and header cells with no `/Scope` fail 7.5 t1; restoring them
// through the edits clears both and adds nothing the producer's own document did not fail.
func TestRestoringAltAndScopeClearsTheirUA1Clauses(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so P09.S02's ua1 clauses are UNCHECKED in this run.")
	}
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is absent, so there is no table-and-figure document to strip.")
	}
	src := tableAndFigureODT(t)
	// A byte strip is valid only where the keys are not inside an object stream (never byte-count a
	// compressed PDF), so the setup proves it first.
	if n := bytes.Count(src, []byte("/ObjStm")); n != 0 {
		t.Fatalf("setup: the document has %d object stream(s), so a byte strip cannot be trusted", n)
	}
	if bytes.Count(src, []byte("/Alt")) != 1 || bytes.Count(src, []byte("/Scope")) != 2 {
		t.Fatalf("setup: %d /Alt and %d /Scope, want 1 and 2", bytes.Count(src, []byte("/Alt")), bytes.Count(src, []byte("/Scope")))
	}
	stripped := bytes.ReplaceAll(bytes.ReplaceAll(src, []byte("/Alt"), []byte("/Xlt")), []byte("/Scope"), []byte("/Xcope"))
	sv := viewOf(t, stripped)
	var edits []structEdit
	for _, e := range sv.elements {
		switch e.standard {
		case "Figure":
			if e.hasAlt {
				t.Fatal("setup: the stripped figure still reads an alt")
			}
			edits = append(edits, structEdit{kind: editAlt, elem: e.id, value: "A grey square"})
		case "TH":
			if e.scope != "" {
				t.Fatal("setup: a stripped header cell still reads a scope")
			}
			edits = append(edits, structEdit{kind: editScope, elem: e.id, value: "Column"})
		}
	}
	if len(edits) != 3 {
		t.Fatalf("setup: %d edit(s), want one alt and two scopes", len(edits))
	}
	fixed := mustApply(t, stripped, edits...)

	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"original.pdf": src, "stripped.pdf": stripped, "fixed.pdf": fixed} {
		f := filepath.Join(dir, n)
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	orig, bare, fix := cl["original.pdf"], cl["stripped.pdf"], cl["fixed.pdf"]
	if orig == nil || bare == nil || fix == nil {
		t.Fatal("veraPDF could not validate one of the three, so there is no differential")
	}
	for _, c := range []string{"7.3 t1", "7.5 t1"} {
		if !bare[c] {
			t.Errorf("setup: the stripped document does not fail %s, so restoring it proves nothing", c)
		}
		if fix[c] {
			t.Errorf("the edited document still fails %s", c)
		}
	}
	var added []string
	for c := range fix {
		if !orig[c] {
			added = append(added, c)
		}
	}
	sort.Strings(added)
	t.Logf("original fails %v; stripped %v; fixed %v", sortedClauses(orig), sortedClauses(bare), sortedClauses(fix))
	if len(added) > 0 {
		t.Errorf("the edits added ua1 clause(s) the producer's document did not fail: %v", added)
	}
}

// editFixture is a hand-built tree: a section holding an inline paragraph, a header cell and a
// paragraph that holds a span.
func editFixture() []byte {
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8:  "<< /Type /StructElem /S /Sect /P 7 0 R /K [<< /S /P >> 9 0 R 10 0 R] >>",
		9:  "<< /Type /StructElem /S /TH /P 8 0 R >>",
		10: "<< /Type /StructElem /S /P /P 8 0 R /K [11 0 R] >>",
		11: "<< /Type /StructElem /S /Span /P 10 0 R >>",
	})
}

// TestAnEditThatDoesNotDescribeTheTreeIsRefused — stale is ErrTagsStale (409 at the route), malformed
// is ErrTagsReview (400).
func TestAnEditThatDoesNotDescribeTheTreeIsRefused(t *testing.T) {
	src := editFixture()
	for _, c := range []struct {
		name  string
		edits []structEdit
		want  error
	}{
		{"no edit at all", nil, ErrTagsReview},
		{"an element the tree does not have", []structEdit{{kind: editRetype, elem: 99, value: "P"}}, ErrTagsStale},
		{"a parent the tree does not have", []structEdit{{kind: editMove, elem: 10, parent: 99}}, ErrTagsStale},
		{"an element written inline", []structEdit{{kind: editRetype, elem: 0, value: "H1"}}, ErrTagsReview},
		{"a type that is not standard", []structEdit{{kind: editRetype, elem: 10, value: "Bogus"}}, ErrTagsReview},
		{"a scope on a paragraph", []structEdit{{kind: editScope, elem: 10, value: "Row"}}, ErrTagsReview},
		{"a scope that is not one", []structEdit{{kind: editScope, elem: 9, value: "Diagonal"}}, ErrTagsReview},
		{"a move inside itself", []structEdit{{kind: editMove, elem: 10, parent: 10}}, ErrTagsReview},
		{"a move inside its own kid", []structEdit{{kind: editMove, elem: 10, parent: 11}}, ErrTagsReview},
		{"an unknown edit", []structEdit{{elem: 10}}, ErrTagsReview},
	} {
		if _, err := applyStructEdits(src, c.edits); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

// TestAScopeEditNeverWritesThroughASharedAttributeObject — two header cells share one indirect
// attribute object; correcting one leaves the other. A cell whose only attribute object is a Layout
// one keeps it beside the new Table one.
func TestAScopeEditNeverWritesThroughASharedAttributeObject(t *testing.T) {
	src := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8:  "<< /Type /StructElem /S /Table /P 7 0 R /K [9 0 R 10 0 R 12 0 R] >>",
		9:  "<< /Type /StructElem /S /TH /P 8 0 R /A 20 0 R >>",
		10: "<< /Type /StructElem /S /TH /P 8 0 R /A 20 0 R >>",
		12: "<< /Type /StructElem /S /TH /P 8 0 R /A << /O /Layout /Placement /Block >> >>",
		20: "<< /O /Table /Scope /Column >>",
	})
	out := mustApply(t, src,
		structEdit{kind: editScope, elem: 9, value: "Row"},
		structEdit{kind: editScope, elem: 12, value: "Both"})
	got := byID(viewOf(t, out))
	if got[9].scope != "Row" || got[10].scope != "Column" || got[12].scope != "Both" {
		t.Errorf("scopes read %q %q %q, want Row Column Both — a shared attribute object was written through", got[9].scope, got[10].scope, got[12].scope)
	}
	ctx, raw := rawElement(t, out, 12)
	arr, err := ctx.DereferenceArray(raw["A"])
	if err != nil || len(arr) != 2 {
		t.Fatalf("the Layout cell's /A reads %v, want the Layout object and a Table one", raw["A"])
	}
	if first, _ := ctx.DereferenceDict(arr[0]); first == nil || first.NameEntry("O") == nil || *first.NameEntry("O") != "Layout" {
		t.Errorf("the Layout attribute object did not survive: %v", arr[0])
	}
	cleared := mustApply(t, out, structEdit{kind: editScope, elem: 9, value: ""})
	if got := byID(viewOf(t, cleared)); got[9].scope != "" || got[10].scope != "Column" {
		t.Errorf("removing a scope reads %q, and the other cell %q", got[9].scope, got[10].scope)
	}
}

// TestAMoveHandlesEveryFormOfK — a `/K` that is a single reference, a direct array, and an indirect
// array object, on both ends of a move.
func TestAMoveHandlesEveryFormOfK(t *testing.T) {
	src := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] >>",
		8:  "<< /Type /StructElem /S /Div /P 7 0 R /K 9 0 R >>",
		9:  "<< /Type /StructElem /S /P /P 8 0 R >>",
		10: "<< /Type /StructElem /S /Div /P 7 0 R /K 30 0 R >>",
		11: "<< /Type /StructElem /S /P /P 10 0 R >>",
		30: "[11 0 R]",
	})
	out := mustApply(t, src, structEdit{kind: editMove, elem: 9, parent: 10, index: 0})
	v := viewOf(t, out)
	got := byID(v)
	if k := kidIDs(v, got[10]); !equalInts(k, []int{9, 11}) {
		t.Errorf("the indirect-array division's kids read %v, want [9 11]", k)
	}
	if k := kidIDs(v, got[8]); len(k) != 0 {
		t.Errorf("the single-reference division still lists %v", k)
	}
	ctx, raw := rawElement(t, out, 10)
	if _, still := raw["K"].(types.IndirectRef); !still {
		t.Errorf("the indirect /K was replaced rather than updated where it lives: %v", raw["K"])
	}
	if arr, _ := ctx.DereferenceArray(raw["K"]); len(arr) != 2 {
		t.Errorf("the /K object holds %v", arr)
	}
	_, moved := rawElement(t, out, 9)
	if p, ok := moved["P"].(types.IndirectRef); !ok || p.ObjectNumber.Value() != 10 {
		t.Errorf("the moved paragraph's /P reads %v", moved["P"])
	}

	back := mustApply(t, out, structEdit{kind: editMove, elem: 11, parent: 8, index: 5}, structEdit{kind: editMove, elem: 10, index: 0})
	bv := viewOf(t, back)
	bg := byID(bv)
	if k := kidIDs(bv, bg[8]); !equalInts(k, []int{11}) {
		t.Errorf("a move into an emptied /K, past its end, reads %v", k)
	}
	// This tree has no Document element, so the root's own order is the order of top-level elements.
	var roots []int
	for _, e := range bv.elements {
		if e.parent == -1 {
			roots = append(roots, e.id)
		}
	}
	if !equalInts(roots, []int{10, 8}) {
		t.Errorf("the root's elements read %v, want [10 8]", roots)
	}
}

// TestAMoveCountsOnlyElementsAndABatchSeesItsOwnMoves — a position is among a parent's ELEMENT kids,
// with marked-content ids between them not counted; and the second edit of a batch resolves the element
// where the first one put it.
func TestAMoveCountsOnlyElementsAndABatchSeesItsOwnMoves(t *testing.T) {
	src := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 12 0 R] >>",
		8:  "<< /Type /StructElem /S /Div /P 7 0 R /Pg 3 0 R /K [0 9 0 R 1 10 0 R] >>",
		9:  "<< /Type /StructElem /S /P /P 8 0 R >>",
		10: "<< /Type /StructElem /S /P /P 8 0 R >>",
		11: "<< /Type /StructElem /S /Span /P 12 0 R >>",
		12: "<< /Type /StructElem /S /Div /P 7 0 R /K [11 0 R] >>",
	})
	out := mustApply(t, src, structEdit{kind: editMove, elem: 11, parent: 8, index: 1})
	v := viewOf(t, out)
	if got := kidIDs(v, byID(v)[8]); !equalInts(got, []int{9, 11, 10}) {
		t.Errorf("a move to the second element place, among marked content, reads %v — want [9 11 10]", got)
	}
	ctx, raw := rawElement(t, out, 8)
	k, _ := ctx.DereferenceArray(raw["K"])
	if len(k) != 5 {
		t.Fatalf("the division's /K holds %d entries, want its two MCIDs and three elements", len(k))
	}
	if _, isInt := k[2].(types.Integer); !isInt {
		t.Errorf("the MCID between the first two elements moved: /K is %v", k)
	}

	// Moved under 12, then moved again with no parent named: "its current parent" is 12 by then.
	batch := mustApply(t, src,
		structEdit{kind: editMove, elem: 9, parent: 12, index: 0},
		structEdit{kind: editMove, elem: 9, index: -1})
	bv := viewOf(t, batch)
	if got := kidIDs(bv, byID(bv)[12]); !equalInts(got, []int{11, 9}) {
		t.Errorf("a batch's second move reads %v under the new parent, want [11 9]", got)
	}
}

// TestADefectTheTreeAlreadyHadDoesNotBlockAnEdit — a producer's tree that already contradicts itself
// can still be corrected; the edit neither refuses because of what it did not do nor claims to have
// repaired it. And a stale edit says so in a tree's terms, not a proposal's.
func TestADefectTheTreeAlreadyHadDoesNotBlockAnEdit(t *testing.T) {
	src := danglingStructParentsFixture()
	if _, defects := checkTree(t, src); len(defects) == 0 {
		t.Fatal("setup: the fixture has no defect, so this proves nothing")
	}
	target := 0
	for _, e := range viewOf(t, src).elements {
		if e.id != 0 {
			target = e.id
			break
		}
	}
	if target == 0 {
		t.Fatal("setup: the fixture has no addressable element")
	}
	out, err := applyStructEdits(src, []structEdit{{kind: editRetype, elem: target, value: "Div"}})
	if err != nil {
		t.Fatalf("an edit to a tree that was already inconsistent was refused: %v", err)
	}
	if got := byID(viewOf(t, out))[target]; got.kind != "Div" {
		t.Errorf("the retype reads %s", got.kind)
	}
	if _, defects := checkTree(t, out); len(defects) == 0 {
		t.Error("the pre-existing defect disappeared — an edit that repairs what nobody asked it to is a second change")
	}

	_, stale := applyStructEdits(editFixture(), []structEdit{{kind: editRetype, elem: 99, value: "P"}})
	if !errors.Is(stale, ErrTagsStale) || strings.Contains(stale.Error(), "propos") {
		t.Errorf("a stale edit reads %v — want ErrTagsStale in a tree's terms", stale)
	}
}

// TestAltTextKeepsEveryCharacter — parentheses and backslashes are string delimiters in a PDF, and a
// non-Latin character needs UTF-16.
func TestAltTextKeepsEveryCharacter(t *testing.T) {
	alt := `Chart (2024) — ünits \ in 日本 )(`
	out := mustApply(t, editFixture(), structEdit{kind: editAlt, elem: 10, value: alt})
	if got := byID(viewOf(t, out))[10]; got.alt != alt {
		t.Errorf("alt reads %q, want %q", got.alt, alt)
	}
	cleared := mustApply(t, out, structEdit{kind: editAlt, elem: 10, value: ""})
	if got := byID(viewOf(t, cleared))[10]; got.hasAlt {
		t.Errorf("removing the alt left %q", got.alt)
	}
	if !strings.Contains(alt, "(") {
		t.Fatal("setup: the alt must carry a delimiter")
	}
}
