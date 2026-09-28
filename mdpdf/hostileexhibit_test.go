package mdpdf

import (
	"errors"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// TestAnExhibitWhoseOptimizePassIsUnaffordableIsRefused — ADR-055, the review of /pending 675/714. The
// round-trip exists to prove an exhibit survives the merge, and the merge runs pdfcpu's optimize pass with no
// budget; skipping the pass here admitted the document into exactly that hang.
func TestAnExhibitWhoseOptimizePassIsUnaffordableIsRefused(t *testing.T) {
	pdf := testpdf.FormChain(400)
	// Stimulus first: the document is past the budget.
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if pdfread.Unaffordable(ctx) == "" {
		t.Fatal("setup: a 400-form chain is within the budget, so a refusal would prove nothing")
	}
	done := make(chan error, 1)
	go func() { _, err := normalizePDF(pdf); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, pdfread.ErrUnaffordable) {
			t.Fatalf("normalizePDF over an unaffordable exhibit returned %v, want ErrUnaffordable — a skipped pass "+
				"admits it into the merge's unbounded one", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("normalizePDF did not return within 20 s")
	}
	// The control: an ordinary chain still normalizes.
	if _, err := normalizePDF(testpdf.FormChain(3)); err != nil {
		t.Fatalf("a three-form chain no longer normalizes: %v", err)
	}
}
