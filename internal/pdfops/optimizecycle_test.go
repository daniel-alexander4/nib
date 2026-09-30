package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// formCycles is two cycles of forms of one /Length, of a and b forms, each form naming its successor twice — the
// comparison's cyclic shape (`/pending 715`). `EqualObjects` cuts only when a PAIR repeats on its path, so it walks
// the two cycles until lcm(a, b), doubling at every step.
func formCycles(a, b int) []byte {
	objs := map[int]string{}
	cycle := func(base, n int) {
		for i := 0; i < n; i++ {
			next := base + (i+1)%n
			objs[base+i] = optStream("/X Do /Y Do", fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 10 10] "+
				"/Resources << /XObject << /X %d 0 R /Y %d 0 R >> >>", next, next))
		}
	}
	cycle(100, a)
	cycle(200, b)
	return twoPages("<< /XObject << /X 100 0 R /Y 200 0 R >> >>", "/X Do /Y Do", objs)
}

// TestTwoCyclesOfFormsAreNotCalledCheap — the estimate cut a cycle where it met it, so its size was the cycle's
// length, while pdfcpu's comparison walks it until its pairs repeat. Measured before: eleven forms (cycles of 5 and
// 6) passed the estimate in 0.13 ms and `OptimizeContext` had not finished after 60 s; 4 and 5 took 1.2 s.
func TestTwoCyclesOfFormsAreNotCalledCheap(t *testing.T) {
	pdf := formCycles(5, 6)
	// The setup reads through Validated, never ReadOptimized: that would run the very pass under test.
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	forms := 0
	for _, e := range ctx.Table {
		if e != nil && !e.Free {
			if sd, ok := e.Object.(types.StreamDict); ok {
				if st := sd.Dict.NameEntry("Subtype"); st != nil && *st == "Form" {
					forms++
				}
			}
		}
	}
	if forms != 11 {
		t.Fatalf("setup: the fixture holds %d form XObjects, want 11", forms)
	}
	if why := pdfread.Unaffordable(ctx); !strings.Contains(why, "comparing its form XObjects") {
		t.Fatalf("the estimate over two cycles of 5 and 6 forms returned %q, want the comparison named — pdfcpu "+
			"walks them to lcm 30, doubling each step", why)
	}
	var out []byte
	finishesWithin(t, 20, "RemovePages over two cycles of forms", func() { out, err = RemovePages(pdf, []string{"2"}) })
	if err != nil {
		t.Fatal(err)
	}
	if got := formObjects(t, out); got != 11 {
		t.Errorf("RemovePages wrote %d form XObjects, want all 11 — a skipped pass keeps every form", got)
	}
}

// TestFormsSharingThePagesResourcesAreStillOptimized — the control for the change above: a cycle reached THROUGH
// an object both compared forms name (here the page's own /Resources, which every form's /Resources is too, a shape
// producers write) costs pdfcpu nothing, because `EqualObjects` answers equal references at once. Two identical
// forms there must still be merged.
func TestFormsSharingThePagesResourcesAreStillOptimized(t *testing.T) {
	pdf := twoPages("6 0 R", "/X Do /Y Do", map[int]string{
		6:   "<< /XObject << /X 100 0 R /Y 101 0 R >> >>",
		100: optStream("0 0 1 1 re f", "/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources 6 0 R"),
		101: optStream("0 0 1 1 re f", "/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources 6 0 R"),
	})
	if got := formObjects(t, pdf); got != 2 {
		t.Fatalf("setup: the fixture holds %d form XObjects, want 2", got)
	}
	out, err := RemovePages(pdf, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if got := formObjects(t, out); got != 1 {
		t.Errorf("RemovePages wrote %d form XObjects, want the two identical forms merged into 1 — a cycle through "+
			"a shared object is not the costly shape", got)
	}
}
