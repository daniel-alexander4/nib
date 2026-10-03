package mdpdf

import (
	"slices"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// TestAPacketOfDocumentsWhoseOptimizePassIsUnaffordableFinishes — ADR-082, `/pending 716`. The packet's merge was
// `api.MergeRaw`, whose closing optimize pass ran over the MERGED context with no budget: the cover is only
// validated, so a 400-form chain as the cover reached that pass whole (measured before the restatement: still
// running at 30 s), and no exhibit's own budget bounds a pass over several documents at once. The merge is
// `pdfread.MergeRaw` now, budgeted over the merged context; and an exhibit past the budget is KEPT, its pass skipped
// — ADR-055 refused it, because the merge it was being admitted to was the unbudgeted one.
func TestAPacketOfDocumentsWhoseOptimizePassIsUnaffordableFinishes(t *testing.T) {
	chain := testpdf.FormChain(400)
	// Stimulus first: the document is past the budget.
	ctx, err := pdfread.Validated(chain, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if pdfread.Unaffordable(ctx) == "" {
		t.Fatal("setup: a 400-form chain is within the budget, so finishing would prove nothing")
	}
	type result struct {
		out     []byte
		skipped []int
		err     error
	}
	done := make(chan result, 1)
	go func() {
		out, skipped, err := AssemblePacket(chain, [][]byte{chain, testpdf.FormChain(3)})
		done <- result{out, skipped, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("AssemblePacket over unaffordable documents: %v", r.err)
		}
		if !slices.Equal(r.skipped, nil) {
			t.Fatalf("AssemblePacket left out exhibits %v — an exhibit too costly to deduplicate is kept, not dropped", r.skipped)
		}
		// Not checkPacket: its `api.Validate` runs pdfcpu's pass unbudgeted (validate.go:68) — the very cost under test.
		got, err := pdfread.Validated(r.out, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("the packet does not validate: %v", err)
		}
		if got.PageCount != 3 {
			t.Fatalf("the packet has %d pages, want 3 (cover, both exhibits)", got.PageCount)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("AssemblePacket over a cover and an exhibit of 400 forms did not return within 20 s")
	}
}
