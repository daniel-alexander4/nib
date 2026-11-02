package pdfops

import (
	"context"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"strings"
	"testing"

	"nib/internal/pdfread"
)

// TestAMarkdownTableIsTaggedAsATable — I2. Table > TR > TH/TD from the document's own structure, header cells scoped to
// their column, every row the same length even with an empty cell, and the text still found.
func TestAMarkdownTableIsTaggedAsATable(t *testing.T) {
	md := "Intro.\n\n| Item | Qty | Price |\n|:-----|:---:|------:|\n| Pens | 12 | 3.50 |\n| Paper |  | 1.00 |\n\nAfter.\n"
	out, err := ConvertDocToPDF(context.Background(), []byte(md), ".md")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.ReadForInspection(out)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := readStructTree(ctx, livePageObjects(ctx))
	if err != nil {
		t.Fatalf("no structure tree: %v", err)
	}
	kinds := map[string]int{}
	var rows []int
	for _, e := range tree.elems {
		kinds[e.kind]++
		if e.kind == "TR" {
			rows = append(rows, len(e.kids))
		}
		if e.kind == "TH" {
			a := derefAttrScope(ctx, e)
			if a != "Column" {
				t.Errorf("a TH has Scope %q, want Column", a)
			}
		}
	}
	if kinds["Table"] != 1 || kinds["TR"] != 3 || kinds["TH"] != 3 || kinds["TD"] != 6 {
		t.Errorf("elements %v, want 1 Table, 3 TR, 3 TH and 6 TD", kinds)
	}
	for i, n := range rows {
		if n != 3 {
			t.Errorf("row %d has %d cells, want 3", i, n)
		}
	}
	pages, err := DocumentText(out)
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, p := range pages {
		for _, r := range p.Runs {
			all.WriteString(r.Text + " ")
		}
	}
	for _, want := range []string{"Item", "Pens", "3.50", "Paper", "1.00", "After."} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("%q is not in the converted text %q", want, all.String())
		}
	}
	if strings.Contains(all.String(), "|") {
		t.Errorf("the bars are still in the text: %q", all.String())
	}
}

// derefAttrScope is the /Scope name in an element's Table attribute object ("" when it has none).
func derefAttrScope(ctx *model.Context, e *structElem) string {
	var objs []types.Object
	if d, _ := ctx.DereferenceDict(e.dict["A"]); d != nil {
		objs = append(objs, d)
	} else if arr, _ := ctx.DereferenceArray(e.dict["A"]); arr != nil {
		objs = append(objs, arr...)
	}
	for _, o := range objs {
		if d, _ := ctx.DereferenceDict(o); d != nil {
			if n := d.NameEntry("Scope"); n != nil {
				return *n
			}
		}
	}
	return ""
}
