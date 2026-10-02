package pdfread_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// formOverChain is a page drawing one form whose /Foo names a chain `N 0 obj [N+1 0 R]` links long — a key pdfcpu's
// validator never follows, so the reference door does not see the chain, and the optimize estimate's `shape` does.
func formOverChain(links int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R /Resources << /XObject << /Fm0 5 0 R >> >> >>",
		4: "<< /Length 8 >>\nstream\n/Fm0 Do\nendstream",
		5: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Foo 10 0 R /Length 0 >>\nstream\n\nendstream",
	}
	for i := 0; i < links; i++ {
		objs[10+i] = fmt.Sprintf("[%d 0 R]", 11+i)
	}
	objs[10+links] = "[]"
	return testpdf.Assemble(objs)
}

// TestAChainDeeperThanTheShapeBoundIsUnaffordable — `shape` recursed once per link with no bound, and a stack overflow
// is fatal, not a panic: ~2,000,000 links took the process. Past `maxShapeDepth` the estimate refuses by name; a
// shallow chain still reads.
func TestAChainDeeperThanTheShapeBoundIsUnaffordable(t *testing.T) {
	read := func(links int) *model.Context {
		// Through the reference door, as every reader reaches the estimate: the chain is under a key it never follows.
		ctx, err := pdfread.Validated(formOverChain(links), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%d links: the door refused it (%v), so this does not reach the estimate", links, err)
		}
		return ctx
	}
	if why := pdfread.Unaffordable(read(100)); why != "" {
		t.Fatalf("control: a form over a 100-link chain is unaffordable (%s)", why)
	}
	if why := pdfread.Unaffordable(read(20000)); !strings.Contains(why, "chain of objects more than") {
		t.Fatalf("a form over a 20,000-link chain: Unaffordable %q, want the depth refusal", why)
	}
}

// TestANextThatIsNeitherDictNorArrayEndsTheWalk — the reference door's action role followed a `/Next` that was neither a
// dict nor an array (`0`, or an array of `0`s) back into itself until the stack died, fatally, inside `Validated` —
// every mutation's read. Each shape now returns, whatever pdfcpu then says of it.
func TestANextThatIsNeitherDictNorArrayEndsTheWalk(t *testing.T) {
	for _, next := range []string{"0", "[0 0 0]", "/Foo", "(x)", "[[0] [[0]]]"} {
		objs := map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Annots [5 0 R] >>",
			5: "<< /Type /Annot /Subtype /Link /Rect [0 0 1 1] /A 6 0 R >>",
			6: "<< /S /GoTo /D [3 0 R /Fit] /Next " + next + " >>",
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = pdfread.Validated(testpdf.Assemble(objs), model.NewDefaultConfiguration())
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("/Next %s: Validated did not return", next)
		}
	}
}
