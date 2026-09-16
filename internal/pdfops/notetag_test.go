package pdfops

import (
	"bytes"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// noteFacts reads, per page, how many /Text annotations there are, how many carry a /StructParent, and
// whether the page orders its tabs by structure; plus how many /Annot elements the tree holds.
func noteFacts(t *testing.T, pdf []byte) (notes, described, annotElems int, tabsS bool) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	tabsS = true
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, derr := ctx.PageDict(p, false)
		if derr != nil || d == nil {
			t.Fatal(derr)
		}
		annots, _ := ctx.DereferenceArray(d["Annots"])
		pageHasNote := false
		for _, a := range annots {
			ad, aerr := ctx.DereferenceDict(a)
			if aerr != nil || ad == nil {
				continue
			}
			if sub, _ := ad["Subtype"].(types.Name); sub != "Text" {
				continue
			}
			notes++
			pageHasNote = true
			if _, ok := ad["StructParent"]; ok {
				described++
			}
		}
		if pageHasNote {
			if tabs, _ := d["Tabs"].(types.Name); tabs != "S" {
				tabsS = false
			}
		}
	}
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Object == nil {
			continue
		}
		if d, ok := e.Object.(types.Dict); ok {
			if ty, _ := d["Type"].(types.Name); ty == "StructElem" {
				if s, _ := d["S"].(types.Name); s == "Annot" {
					annotElems++
				}
			}
		}
	}
	return notes, described, annotElems, tabsS
}

// TestANoteInATaggedDocumentIsAnAnnotElement — `PLAN-ua-coverage.md` P01.S02. The ua1 census asserts the
// clauses; this asserts the three writes, so a regression names which one went.
func TestANoteInATaggedDocumentIsAnAnnotElement(t *testing.T) {
	src := labelReady(t, "# Notes\n\nA paragraph to comment on.\n")
	if !ClaimsTagging(src) {
		t.Fatal("setup: the conversion is not tagged, so this asserts nothing about a tagged document")
	}
	out, err := AddNotes(src, []Note{{Page: 1, X: 100, Y: 700, Text: "first"}, {Page: 1, X: 200, Y: 600, Text: "second"}})
	if err != nil {
		t.Fatal(err)
	}
	notes, described, annotElems, tabsS := noteFacts(t, out)
	if notes != 2 {
		t.Fatalf("setup: %d note annotation(s) written, want 2", notes)
	}
	if described != notes {
		t.Errorf("%d of %d notes carry a /StructParent — a note nothing in the tree points at fails 7.18.1", described, notes)
	}
	if annotElems != notes {
		t.Errorf("the tree holds %d /Annot element(s) for %d notes", annotElems, notes)
	}
	if !tabsS {
		t.Error("a page with a note does not order its tabs by structure (7.18.3)")
	}
}

// TestANoteInAnUntaggedDocumentInventsNoTree — ADR-031 law 1: a note must not create a claim over pages
// nothing else describes.
func TestANoteInAnUntaggedDocumentInventsNoTree(t *testing.T) {
	src, err := testpdf.Text("an untagged page")
	if err != nil {
		t.Fatal(err)
	}
	if ClaimsTagging(src) {
		t.Fatal("setup: the untagged fixture claims tagging")
	}
	out, err := AddNotes(src, []Note{{Page: 1, X: 100, Y: 700, Text: "a note"}})
	if err != nil {
		t.Fatal(err)
	}
	if ClaimsTagging(out) {
		t.Error("a note gave an untagged document a tagging claim")
	}
	notes, described, _, tabsS := noteFacts(t, out)
	if notes != 1 {
		t.Fatalf("setup: %d note annotation(s) written, want 1", notes)
	}
	if described != 0 {
		t.Errorf("%d note(s) in an untagged document carry a /StructParent pointing into no tree", described)
	}
	if !tabsS {
		t.Error("7.18.3 does not depend on tagging, and the page with the note has no /Tabs /S")
	}
}
