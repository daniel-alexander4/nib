package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// ignoredChain is a one-page document whose catalog names, under /Foo — a key pdfcpu's validator never reads — a
// chain of links objects, each holding the next's reference nest arrays deep. ring > 0 closes the last link back on
// the first instead of ending it.
func ignoredChain(links, nest int, ring bool) map[int]string {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Foo 10 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] >>",
	}
	open, shut := strings.Repeat("[", nest), strings.Repeat("]", nest)
	for i := 0; i < links; i++ {
		next := 11 + i
		if ring && i == links-1 {
			next = 10
		}
		objs[10+i] = fmt.Sprintf("%s%d 0 R%s", open, next, shut)
	}
	if !ring {
		objs[10+links] = "[]"
	}
	return objs
}

func unvalidated(t *testing.T, pdf []byte) *model.Context {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: pdfcpu's unvalidated read: %v", err)
	}
	return ctx
}

// TestAChainUnderAKeyTheValidatorIgnoresIsRefused — /pending 840. pdfcpu's optimize pass follows every reference from
// the catalog, one recursion per link (`fixReferencesToFreeObjects`), and 1,500,000 links under a key the validator
// never reads were a fatal stack overflow that the door, which followed the validator's edges only, had passed. The
// door refuses it; the same chain under the bound reads.
func TestAChainUnderAKeyTheValidatorIgnoresIsRefused(t *testing.T) {
	const nest = 50 // levels a link: fifty arrays pdfcpu steps into on the way to the next reference
	under := pdfread.MaxObjectLevels/nest - 500
	if _, err := pdfread.Validated(testpdf.Assemble(ignoredChain(under, nest, false)), model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("control: %d links of %d levels, under the bound, do not read: %v", under, nest, err)
	}
	over := pdfread.MaxObjectLevels/nest + 100
	_, err := pdfread.Validated(testpdf.Assemble(ignoredChain(over, nest, false)), model.NewDefaultConfiguration())
	if !errors.Is(err, pdfread.ErrReferenceDepth) {
		t.Fatalf("%d links of %d levels under an ignored key: %v, want ErrReferenceDepth", over, nest, err)
	}
	if !strings.Contains(err.Error(), "under any key") {
		t.Fatalf("the refusal does not say which pass refused: %v", err)
	}
}

// TestLevelsCountEveryContainerOnTheWayToAReference pins what a level is: the object a reference names, each array or
// dictionary nested in it down to the next reference, and — for objects that name each other in a ring — every one of
// them, because a walk that enters an object once can cross them all.
func TestLevelsCountEveryContainerOnTheWayToAReference(t *testing.T) {
	for _, c := range []struct {
		name         string
		links, nest  int
		ring         bool
		want         int
		wantFromRoot int
	}{
		{"fifty links", 50, 1, false, 52, 1}, // the catalog, fifty links, the end
		{"fifty links, each three arrays deep", 50, 3, false, 152, 1},
		{"a ring of ten", 10, 1, true, 11, 1},                     // the catalog, then all ten
		{"no chain: the page tree's own ring", 0, 1, false, 4, 1}, // catalog, /Pages (its /Kids array is a level), page
	} {
		got, from := pdfread.ObjectLevels(unvalidated(t, testpdf.Assemble(ignoredChain(c.links, c.nest, c.ring))))
		if got != c.want || from != c.wantFromRoot {
			t.Errorf("%s: %d levels from object %d, want %d from object %d", c.name, got, from, c.want, c.wantFromRoot)
		}
	}
}

// TestTheDoorReadsObjectsHeldInAnObjectStream — pdfcpu's read leaves a compressed object undecoded, and the door read
// it as an object naming nothing: a tiling pattern whose /Resources is a compressed dictionary naming the pattern back
// passed, and the validator overflowed the stack on it (measured). Asked of the door alone, on the unvalidated read,
// so a door that misses it fails here rather than taking the test process with it.
func TestTheDoorReadsObjectsHeldInAnObjectStream(t *testing.T) {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources << /Pattern << /P0 5 0 R >> >> >>",
		5: "<< /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 1 1] /XStep 1 /YStep 1 /Resources 6 0 R /Length 0 >>\nstream\n\nendstream",
		6: "<< /Pattern << /P0 5 0 R >> >>",
	}
	if err := pdfread.RefuseUnbounded(unvalidated(t, testpdf.Assemble(objs))); !errors.Is(err, pdfread.ErrReferenceCycle) {
		t.Fatalf("control: the loop with every object in the open: %v, want ErrReferenceCycle", err)
	}
	if err := pdfread.RefuseUnbounded(unvalidated(t, testpdf.AssembleCompressed(objs, 6))); !errors.Is(err, pdfread.ErrReferenceCycle) {
		t.Fatalf("the same loop with its /Resources dictionary in an object stream: %v, want ErrReferenceCycle", err)
	}
	// The same file with the loop ended reads, so the fixture's object stream is one pdfcpu accepts.
	objs[6] = "<< >>"
	if _, err := pdfread.Validated(testpdf.AssembleCompressed(objs, 6), model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("control: the compressed fixture without its loop does not read: %v", err)
	}
	// And a chain under an ignored key is measured through compressed links too.
	chain := ignoredChain(40, 1, false)
	var packed []int
	for nr := 10; nr <= 50; nr++ {
		packed = append(packed, nr)
	}
	ctx := unvalidated(t, testpdf.AssembleCompressed(chain, packed...))
	if err := pdfread.RefuseUnbounded(ctx); err != nil {
		t.Fatalf("a forty-link compressed chain: %v", err)
	}
	if got, _ := pdfread.ObjectLevels(ctx); got != 42 {
		t.Fatalf("a forty-link chain held in an object stream measured %d levels, want 42", got)
	}
}

// sharedContainer is n pages whose /Resources all name one indirect /ExtGState dictionary of n entries.
func sharedContainer(n int) []byte {
	o := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"}
	var kids, entries []string
	for i := 0; i < n; i++ {
		o[10+2*i] = "<< /Type /ExtGState >>"
		entries = append(entries, fmt.Sprintf("/G%d %d 0 R", i, 10+2*i))
		o[11+2*i] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 9 9] /Resources << /ExtGState 5 0 R >> >>"
		kids = append(kids, fmt.Sprintf("%d 0 R", 11+2*i))
	}
	o[5] = "<< " + strings.Join(entries, " ") + " >>"
	o[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n)
	return testpdf.Assemble(o)
}

// TestThePathWalkStopsAtItsBudget — /pending 844. Every page reads a shared indirect container through afresh, so n
// pages over n entries were n² edges walked before the count refused them (8,000: 31 s on the door alone). The walk
// stops once it has followed more edges than the budget allows paths.
func TestThePathWalkStopsAtItsBudget(t *testing.T) {
	const n = 1500
	ctx := unvalidated(t, sharedContainer(n))
	budget := pdfread.PathBudget(ctx)
	_, followed, err := pdfread.CountPaths(ctx, budget)
	if !errors.Is(err, pdfread.ErrReferencePaths) {
		t.Fatalf("%d pages over one dictionary of %d entries: the walk returned %v after %d edges, want ErrReferencePaths", n, n, err, followed)
	}
	if followed > budget+n {
		t.Fatalf("the walk followed %d edges against a budget of %d: it stops within one object's entries of it", followed, budget)
	}
	// Unbudgeted, the same walk is the whole n²: what the stop saves.
	if paths, all, err := pdfread.CountPaths(unvalidated(t, sharedContainer(n)), 0); err != nil || all < n*n || paths < n*n {
		t.Fatalf("control: unbudgeted, %d edges and %d paths (%v), want at least %d of each", all, paths, err, n*n)
	}
	// A document inside the budget is counted exactly as before.
	small := unvalidated(t, sharedContainer(20))
	if paths, _, err := pdfread.CountPaths(small, pdfread.PathBudget(small)); err != nil || paths != 20*21 {
		t.Fatalf("twenty pages over twenty entries: %d paths (%v), want %d", paths, err, 20*21)
	}
}

// TestSharingIsCountedThroughTheEntryEdgesFanInCannotBuild — /pending 767: page /Annots, an ExtGState's /HT, an
// image's /ColorSpace and a Rendition action's /R fed the path count with nothing proving it. Once, shallowly, each
// reads; named four hundred times, each is past the budget.
func TestSharingIsCountedThroughTheEntryEdgesFanInCannotBuild(t *testing.T) {
	for _, s := range testpdf.FanInShapes() {
		if _, err := pdfread.Validated(s.Small, model.NewDefaultConfiguration()); err != nil {
			t.Errorf("setup: one naming through %s does not read: %v", s.Edge, err)
			continue
		}
		refusedAs(t, pdfread.ErrReferencePaths, "400 namings through "+s.Edge, func() error {
			_, err := pdfread.Validated(s.Large, model.NewDefaultConfiguration())
			return err
		})
	}
}
