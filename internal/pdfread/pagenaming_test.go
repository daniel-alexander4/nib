package pdfread_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// /pending 816: two per-page multipliers the reference door did not count.
//
//   - pdfcpu validates a leaf `/Page` once per NAMING in `/Kids` (`processPagesKids`, validate/page.go:1043; its
//     PageTreeVisit refuses only a repeated `/Pages` node), and `validatePagesAnnotations` walks `/Annots` once per
//     naming too — while the door counted every page once, as a root. One page named 200 times over a 200-slot
//     `/Annots` with a 30-action chain, 5.2 KB, passed the door and validated for 4.8 s (measured).
//   - `rolePage` followed `/Resources`, `/Annots` and `/AA`; `validatePageDict` (page.go:808) also reaches a colour
//     space through `/Group /CS` and `/SeparationInfo /ColorSpace`, and a measure dictionary through every `/VP`
//     viewport. 100 pages sharing a depth-16 doubling chain behind `/Group /CS`, 16.6 KB, passed and took 15.9 s.

// namedPage is one page named n times in the root's /Kids, its /Annots k slots of one link whose action chain is
// chain long.
func namedPage(n, k, chain int) []byte { return namedPageUnder(n, k, chain, false) }

// namedPageUnder is namedPage, with the namings in an intermediate /Pages node under the root when nested.
func namedPageUnder(n, k, chain int, nested bool) []byte {
	parent := 2
	if nested {
		parent = 900
	}
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		3: fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 10 10] /Annots 4 0 R >>", parent),
		4: "[" + strings.Repeat("5 0 R ", k) + "]",
		5: "<< /Type /Annot /Subtype /Link /Rect [0 0 1 1] /A 6 0 R >>",
	}
	for i := 0; i < chain; i++ {
		act := fmt.Sprintf("<< /S /GoTo /D [3 0 R /Fit] /Next %d 0 R >>", 7+i)
		if i == chain-1 {
			act = "<< /S /GoTo /D [3 0 R /Fit] >>"
		}
		objs[6+i] = act
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Repeat("3 0 R ", n), n)
	if nested {
		objs[900] = fmt.Sprintf("<< /Type /Pages /Parent 2 0 R /Kids [%s] /Count %d >>", strings.Repeat("3 0 R ", n), n)
		objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [900 0 R] /Count %d >>", n)
	}
	return testpdf.Assemble(objs)
}

// namedContents is one page named n times whose /Contents array names one stream k times: pdfcpu dereferences every
// slot once per naming, a product of two arrays that share one file's bytes.
func namedContents(n, k int) []byte {
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Repeat("3 0 R ", n), n),
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R >>",
		4: "[" + strings.Repeat("5 0 R ", k) + "]",
		5: "<< /Length 0 >>\nstream\n\nendstream",
	})
}

// doublingChain is depth type 3 functions from object first, each naming the next twice, ending in a type 2.
func doublingChain(objs map[int]string, first, depth int) {
	for i := 0; i < depth; i++ {
		objs[first+i] = fmt.Sprintf("<< /FunctionType 3 /Domain [0 1] /Functions [%d 0 R %d 0 R] /Bounds [0.5] "+
			"/Encode [0 1 0 1] >>", first+i+1, first+i+1)
	}
	objs[first+depth] = "<< /FunctionType 2 /Domain [0 1] /C0 [0] /C1 [1] /N 1 >>"
}

// pagesReaching builds m pages that each reach one Separation colour space — and through its tint transform a
// doubling chain depth deep — by the page key given: entry is the page-dictionary text naming object 4.
func pagesReaching(m, depth int, entry string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		4: "[/Separation /Spot /DeviceGray 5 0 R]",
	}
	doublingChain(objs, 5, depth)
	first := 6 + depth
	var kids []string
	for i := 0; i < m; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", first+i))
		objs[first+i] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] " + entry + " >>"
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), m)
	return testpdf.Assemble(objs)
}

// pagesViewing builds m pages whose /VP holds v viewports, all naming one measure with f number formats in /X.
func pagesViewing(m, v, f int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Version /1.7 >>",
		3: "<< /Type /NumberFormat /U (mi) /C 1 >>",
		4: "<< /Type /Measure /Subtype /RL /R (1in = 1mi) /X [" + strings.Repeat("3 0 R ", f) + "] /D [3 0 R] /A [3 0 R] >>",
		5: "[" + strings.Repeat("<< /Type /Viewport /BBox [0 0 1 1] /Measure 4 0 R >> ", v) + "]",
	}
	var kids []string
	for i := 0; i < m; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", 6+i))
		objs[6+i] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /VP 5 0 R >>"
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), m)
	return testpdf.Assemble(objs)
}

// TestALeafNamedManyTimesIsCountedPerNaming — the first multiplier. The stimulus: the same objects with the page
// named ONCE read, so the difference below is the naming count and nothing else.
func TestALeafNamedManyTimesIsCountedPerNaming(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	if _, err := pdfread.Validated(namedPage(1, 200, 30), conf); err != nil {
		t.Fatalf("setup: the page named once was refused (%v)", err)
	}
	start := time.Now()
	refusedAs(t, pdfread.ErrReferencePaths, "one page named 200 times over a 200-slot /Annots, 30-action chain", func() error {
		_, err := pdfread.Validated(namedPage(200, 200, 30), conf)
		return err
	})
	if d := time.Since(start); d > time.Second {
		t.Errorf("refusing took %v — the door must answer before the validator's cost", d)
	}
	// The namings are counted wherever in the tree they are, not only under the root.
	if _, err := pdfread.Validated(namedPageUnder(1, 200, 30, true), conf); err != nil {
		t.Fatalf("setup: the nested page named once was refused (%v)", err)
	}
	refusedAs(t, pdfread.ErrReferencePaths, "the same, the namings in an intermediate /Pages node", func() error {
		_, err := pdfread.Validated(namedPageUnder(200, 200, 30, true), conf)
		return err
	})
	// The count is per naming, not a refusal of naming: a page named twice still reads.
	if _, err := pdfread.Validated(namedPage(2, 20, 30), conf); err != nil {
		t.Errorf("a page named twice was refused: %v", err)
	}
	// A repeat also re-walks the page's own flat arrays, which reach no other object and so carry no path: one page
	// named 3,000 times over a 3,000-slot /Contents, 36 KB, validated for 5.9 s (measured under load) and passed.
	if _, err := pdfread.Validated(namedContents(1, 3000), conf); err != nil {
		t.Fatalf("setup: the page named once was refused (%v)", err)
	}
	refusedAs(t, pdfread.ErrReferencePaths, "one page named 3,000 times over a 3,000-slot /Contents", func() error {
		_, err := pdfread.Validated(namedContents(3000, 3000), conf)
		return err
	})
}

// TestEveryPageKeyThatReachesAColourSpaceIsCounted — the second multiplier, through each page key pdfcpu follows to
// a colour space; and the viewport measure, which pdfcpu re-validates once per viewport.
func TestEveryPageKeyThatReachesAColourSpaceIsCounted(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	for _, c := range []struct {
		name  string
		entry string
	}{
		{"/Group /CS", "/Group << /S /Transparency /CS 4 0 R >>"},
		{"/SeparationInfo /ColorSpace", "/SeparationInfo << /Pages [] /DeviceColorant /Spot /ColorSpace 4 0 R >>"},
	} {
		if _, err := pdfread.Validated(pagesReaching(1, 4, c.entry), conf); err != nil {
			t.Errorf("%s: setup: one page over a short chain was refused (%v)", c.name, err)
			continue
		}
		start := time.Now()
		refusedAs(t, pdfread.ErrReferencePaths, "100 pages over a depth-16 chain through "+c.name, func() error {
			_, err := pdfread.Validated(pagesReaching(100, 16, c.entry), conf)
			return err
		})
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s: refusing took %v", c.name, d)
		}
	}
	if _, err := pdfread.Validated(pagesViewing(1, 4, 4), conf); err != nil {
		t.Errorf("/VP: setup: a small viewport array was refused (%v)", err)
	}
	refusedAs(t, pdfread.ErrReferencePaths, "1,000 viewports naming one measure of 1,000 number formats", func() error {
		_, err := pdfread.Validated(pagesViewing(1, 1000, 1000), conf)
		return err
	})
}
