package pdfops

import (
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 795` I3 and W4: two decodes that inflated whatever a stream declared, and a reflow cascade that took a
// fresh walk budget for every page it went on to.

// decodeBomb is the decoded size of the fixtures' one large stream: ten times a page's byte budget, and under
// pdfcpu's own 512 MiB ceiling, so an uncapped decode SUCCEEDS and is seen only by what it allocates.
const decodeBomb = 200 << 20

// mostToRefuse is what a refusal may allocate: a capped decode grows its buffer to about twice what it holds.
const mostToRefuse = 100 << 20

// TestThePrunerDecodesAStreamOnlyWithinItsBudget — `streamNames` called `sd.Decode()`, so a kept page whose content
// inflates to 200 MiB was inflated whole to collect its names.
func TestThePrunerDecodesAStreamOnlyWithinItsBudget(t *testing.T) {
	if decodeBomb <= newFormWalkBudget(1).maxBytes {
		t.Fatalf("setup: a %d-byte stream is inside a page's budget of %d", decodeBomb, newFormWalkBudget(1).maxBytes)
	}
	page := func(content string) (*resourcePruner, types.Dict) {
		ctx := optimizedCtx(t, assembleFixture(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 8 0 R >> >> /Contents 4 0 R >>",
			4: content,
			8: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		}))
		return &resourcePruner{xt: ctx.XRefTable, policy: refuseUnreadable, names: map[int]map[string]bool{},
			owned: map[int]bool{}, budget: newFormWalkBudget(1)}, pageAt(ctx, nil, 1).Dict
	}
	// Control: an ordinary page's names are read, filtered or not.
	p, d := page(optStream("BT /F1 12 Tf (a) Tj ET", ""))
	if names, ok := p.pageContentNames(d); !ok || !names["F1"] {
		t.Fatalf("control: an ordinary page read names %v (ok %v), want F1", names, ok)
	}
	// Stimulus.
	p, d = page(optStream(flatedSpaces(decodeBomb), "/Filter /FlateDecode"))
	var err error
	alloc := allocated(func() { err = p.page(d, 1) })
	if alloc > mostToRefuse {
		t.Errorf("the pruner allocated %d MiB reading a page whose content inflates to %d MiB, want under %d MiB — "+
			"it decodes through formWalkBudget.formContent", alloc>>20, decodeBomb>>20, mostToRefuse>>20)
	}
	if err == nil || !strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("the pruner answered %v for a page it did not finish reading, want it refused as unreadable", err)
	}
}

// TestTheNUpCarryDecodesAFormOnlyWithinItsBudget — the carry matched each form on a sheet against the source pages
// after `sd.Decode()`, so a composed document carrying one form that inflates to 200 MiB was inflated to be compared.
func TestTheNUpCarryDecodesAFormOnlyWithinItsBudget(t *testing.T) {
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Contents 4 0 R >>",
		4: optStream("0 0 1 1 re f", ""),
		7: "<< /Type /StructTreeRoot >>",
	})
	if sources, ok := capturePageSources(src); !ok || len(sources) != 1 {
		t.Fatalf("setup: the source's pages were captured as %d (ok %v), want 1 — the carry never reaches the forms", len(sources), ok)
	}
	composed := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm1 9 0 R >> >> /Contents 4 0 R >>",
		4: optStream("/Fm1 Do", ""),
		7: "<< /Type /StructTreeRoot >>",
		9: optStream(flatedSpaces(decodeBomb), "/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Filter /FlateDecode"),
	})
	var ok bool
	alloc := allocated(func() { _, ok = carryTagsThroughNUp(src, composed) })
	if ok {
		t.Fatalf("the carry claimed a document whose one form it could not have matched")
	}
	if alloc > mostToRefuse {
		t.Errorf("the carry allocated %d MiB over a form that inflates to %d MiB, want under %d MiB — it decodes "+
			"through formWalkBudget.formContent", alloc>>20, decodeBomb>>20, mostToRefuse>>20)
	}
}

// TestAReflowCascadeReadsTheNextPageAgainstItsOneBudget — `nextPageFor` read each page a flow went on to with a
// budget of its own (`readPageGlyphLayout`), so a cascade paid a budget a page; `pushDown` now hands its one budget on.
func TestAReflowCascadeReadsTheNextPageAgainstItsOneBudget(t *testing.T) {
	ctx := readForWalk(t, pagesDrawingOneFanOut(2, 2)) // 110 form walks a page
	first := pageAt(ctx, nil, 1)
	// Control: with room in the budget the next page is read.
	if _, _, _, err := nextPageFor(ctx, first, newFormWalkBudget(ctx.PageCount)); err != nil {
		t.Fatalf("control: the next page under a fresh budget: %v", err)
	}
	// Stimulus: a cascade that has already spent its walks, all but ten.
	spent := newFormWalkBudget(ctx.PageCount)
	spent.walks = spent.maxWalks - 10
	_, _, _, err := nextPageFor(ctx, first, spent)
	if err == nil || !strings.Contains(err.Error(), "enters form XObjects more than") {
		t.Errorf("the next page of a cascade ten walks from its budget read 110 forms and returned %v, want the "+
			"cascade's walk budget named — a budget per page lets a cascade cost one a page", err)
	}
}
