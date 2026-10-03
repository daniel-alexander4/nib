package pdfops

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The page walk's budget bounds the WORK, not the nodes entered (`/pending 804`, R3). The shape is the review's:
// the root's `/Kids` is an array of K references to a Pages node whose own `/Kids` is that same array, so every
// visit to the node re-iterates K entries that are all on the path. Charged per node, ~budget visits × K
// iterations ran before the refusal; charged per kid, the walk stops within the budget.
func TestThePageWalkBudgetChargesEveryKid(t *testing.T) {
	const k = 20000
	refs := strings.Repeat("4 0 R ", k)
	doc := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids 3 0 R /Count 1 >>",
		3: "[" + refs + "]",
		4: "<< /Type /Pages /Kids 3 0 R /Count 1 >>",
	})
	ctx, err := api.ReadContext(bytes.NewReader(doc), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	const budget = 1000
	steps, err := walkPageTree(ctx.XRefTable, root, budget, func(_ types.Dict, _ int) {})
	if !errors.Is(err, errPageTreeTooLarge) {
		t.Fatalf("err = %v, want errPageTreeTooLarge", err)
	}
	if steps > budget+1 {
		t.Errorf("the walk took %d steps on a budget of %d — a node's /Kids is iterated uncharged, so a %d-entry array of on-path references costs budget × %d", steps, budget, k, k)
	}
}

// repeatedSlotDoc is a one-page document whose `/Annots` names ONE annotation k times.
func repeatedSlotDoc(k int) []byte {
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [" + strings.Repeat("4 0 R ", k) + "] >>",
		4: "<< /Type /Annot /Subtype /Link /Rect [10 10 50 50] /Border [0 0 0] /P 3 0 R >>",
	})
}

// linkObjects counts the link annotation objects a document holds.
func linkObjects(t *testing.T, pdf []byte) int {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Free {
			continue
		}
		if d, ok := e.Object.(types.Dict); ok {
			if st := d.NameEntry("Subtype"); st != nil && *st == "Link" {
				n++
			}
		}
	}
	return n
}

// A copied page gets ONE copy of each distinct annotation its `/Annots` names, however many slots name it
// (`/pending 804`, R3). `selectionCeiling` bounds how many pages a selection may produce, and `clonePage` then
// cloned every SLOT: page 1 × 100 of a page naming one annotation 4,000 times turned 24 KB into 4.2 MB and
// allocated 4.1 GiB. Counted as annotation objects in the output, not timed.
func TestACopiedPageClonesEachAnnotationOnce(t *testing.T) {
	const k, copies = 1000, 100
	sel := make([]string, copies)
	for i := range sel {
		sel[i] = "1"
	}
	out, err := Collect(repeatedSlotDoc(k), sel)
	if err != nil {
		t.Fatal(err)
	}
	got := linkObjects(t, out)
	if got == 0 {
		t.Fatal("setup: no link annotation in the output at all")
	}
	if got > copies {
		t.Errorf("%d copies of a page naming one annotation %d times wrote %d annotation objects, want at most %d — clonePage clones per slot, so the ceiling on pages does not bound the output", copies, k, got, copies)
	}
	t.Logf("%d bytes out, %d link objects", len(out), got)
}
