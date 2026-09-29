package uacheck

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// checkWithin runs Check with a deadline taken from the clock, not from its return: the unbounded pass ran
// for minutes, and a check made after it would wait with it.
func checkWithin(t *testing.T, s int, what string, pdf []byte) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { _, err := Check(pdf); done <- err }()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Duration(s) * time.Second):
		t.Fatalf("Check over %s had not returned after %d s", what, s)
		return nil
	}
}

// TestTheCheckerRefusesAUseCMapLoop — `/pending 675`, through `nib ua`, `nib watch --do ua` and the GUI's
// accessibility report: pdfcpu's validator recursed on the loop until the process died. **Before the fix this
// kills the test binary.**
func TestTheCheckerRefusesAUseCMapLoop(t *testing.T) {
	for name, c := range testpdf.UseCMapLoops() {
		if len(c.Loop) < 2 {
			t.Fatalf("setup: %s carries no loop", name)
		}
		if err := checkWithin(t, 20, name, c.PDF); !errors.Is(err, pdfread.ErrUseCMapCycle) {
			t.Errorf("Check over %s returned %v, want the /UseCMap loop refused", name, err)
		}
	}
	if err := checkWithin(t, 20, "a chain with no loop", testpdf.UseCMapChain()); err != nil {
		t.Errorf("Check refused a /UseCMap chain with no loop: %v", err)
	}
}

// formChainDoc is depth form XObjects of one /Length, each drawing the next under its own /Resources —
// `/pending 706`'s comparison shape (pdfops' `formChain`), which cost the checker 90 s at 400.
func formChainDoc(depth int) []byte {
	form := func(extra string) string {
		return fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] %s /Length 5 >>\nstream\n/X Do\nendstream", extra)
	}
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X 100 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length 5 >>\nstream\n/X Do\nendstream",
	}
	for i := 0; i < depth; i++ {
		objs[100+i] = form(fmt.Sprintf("/Resources << /XObject << /X %d 0 R >> >>", 101+i))
	}
	objs[100+depth] = form("")
	return buildPDF(objs)
}

// TestTheCheckerRefusesWhereTheOptimizePassIsUnaffordable — `/pending 714`. The checker ran pdfcpu's optimize
// pass with no budget (90 s over this document); it now reads through pdfread's budget, and REFUSES rather than
// skipping, because its rules were measured against the optimized reading.
func TestTheCheckerRefusesWhereTheOptimizePassIsUnaffordable(t *testing.T) {
	pdf := formChainDoc(400)
	// Stimulus first: the document is past the budget the door applies.
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if why := pdfread.Unaffordable(ctx); why == "" {
		t.Fatal("setup: a chain of 400 forms is within the optimize budget — the fixture exercises nothing")
	}
	if err := checkWithin(t, 20, "a chain of 400 forms", pdf); !errors.Is(err, pdfread.ErrUnaffordable) {
		t.Errorf("Check over a chain of 400 forms returned %v, want the unaffordable pass refused", err)
	}
	// And a short chain is optimized and checked as before.
	if err := checkWithin(t, 20, "a chain of 3 forms", formChainDoc(3)); err != nil {
		t.Errorf("Check over a chain of 3 forms: %v", err)
	}
}
