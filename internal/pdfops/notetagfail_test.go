package pdfops

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// annotElementPages returns, in /StructTreeRoot /K order, the page number each /Annot element's OBJR names.
func annotElementPages(t *testing.T, pdf []byte) []int {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pageOf := map[int]int{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			pageOf[ir.ObjectNumber.Value()] = p
		}
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.DereferenceDict(cat["StructTreeRoot"])
	if err != nil || root == nil {
		t.Fatalf("no /StructTreeRoot: %v", err)
	}
	kids, _ := ctx.DereferenceArray(root["K"])
	var pages []int
	for _, k := range kids {
		elem, eerr := ctx.DereferenceDict(k)
		if eerr != nil || elem == nil {
			continue
		}
		if s, _ := elem["S"].(types.Name); s != "Annot" {
			continue
		}
		ek, _ := ctx.DereferenceArray(elem["K"])
		for _, o := range ek {
			objr, oerr := ctx.DereferenceDict(o)
			if oerr != nil || objr == nil {
				continue
			}
			if pg, ok := objr["Pg"].(types.IndirectRef); ok {
				pages = append(pages, pageOf[pg.ObjectNumber.Value()])
			}
		}
	}
	return pages
}

// TestNotesAreDescribedInPageOrder — the review finding: iterating the notes' page map appended /Annot
// elements, and so reading order and output bytes, in a different order on every run. Many runs, because
// a random order of three comes out sorted by chance: probed with map iteration restored, eight runs
// went red on three executions of four and passed the fourth.
func TestNotesAreDescribedInPageOrder(t *testing.T) {
	src := labelReady(t, censusMarkdown())
	if n, err := PageCount(src); err != nil || n < 3 {
		t.Fatalf("setup: the document has %d page(s) (err %v); the order question needs three", n, err)
	}
	notes := []Note{
		{Page: 3, X: 100, Y: 700, Text: "on three"},
		{Page: 1, X: 100, Y: 700, Text: "on one"},
		{Page: 2, X: 100, Y: 700, Text: "on two"},
	}
	for run := 0; run < 32; run++ {
		out, err := AddNotes(src, notes)
		if err != nil {
			t.Fatal(err)
		}
		pages := annotElementPages(t, out)
		if len(pages) != 3 {
			t.Fatalf("run %d: %d /Annot element(s) found, want 3 — the order below would be vacuous", run, len(pages))
		}
		for i := 1; i < len(pages); i++ {
			if pages[i] < pages[i-1] {
				t.Fatalf("run %d: the notes' /Annot elements name pages %v — not page order, so a screen reader "+
					"meets them in an order that changes between identical runs", run, pages)
			}
		}
	}
}

// TestANoteOnATreeNibCannotModelStillBakes — the review finding: a tree nib's parser refuses used to fail
// AddNotes, which failed the bake and aborted the user's save. The note must land, undescribed.
func TestANoteOnATreeNibCannotModelStillBakes(t *testing.T) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(taggedFixture()), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.DereferenceDict(cat["StructTreeRoot"])
	if err != nil || root == nil {
		t.Fatal("setup: the tagged fixture has no /StructTreeRoot")
	}
	// An element with no /S: pdfcpu's relaxed validation reads it, nib's parser refuses it. (The other
	// refusals are out of reach — pdfcpu will not read a bare MCID or an MCR under the root, and its own
	// depth limit of 100 fires before nib's 200 — and a fixture pdfcpu refuses tests pdfcpu, not the
	// fallback.)
	root["K"] = types.Array{types.Dict{"K": types.Integer(0)}}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	broken := buf.Bytes()
	// Stimulus before response: nib really cannot model this tree, or "still bakes" proves nothing.
	check, err := api.ReadValidateAndOptimize(bytes.NewReader(broken), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := readStructTree(check, map[int]bool{}); rerr == nil || errors.Is(rerr, errNoStructTree) {
		t.Fatalf("setup: readStructTree accepted the tree (err %v), so this does not reach the refusal", rerr)
	}

	out, err := AddNotes(broken, []Note{{Page: 1, X: 100, Y: 700, Text: "a note"}})
	if err != nil {
		t.Fatalf("a note on a document whose tree nib cannot model failed the operation (%v) — the bake "+
			"answers 500 and the user's save is aborted", err)
	}
	notes, described, _, _ := noteFacts(t, out)
	if notes != 1 {
		t.Errorf("the note was not added (%d note annotations)", notes)
	}
	if described != 0 {
		t.Errorf("%d note(s) carry a /StructParent into a tree nib refused to model", described)
	}
}
