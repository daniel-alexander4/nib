package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// ignoredChain is a one-page document whose page font names, under /Foo — a key pdfcpu's validator never reads — a
// chain of links objects, each holding the next's reference nest arrays deep. The chain hangs off a FONT because the
// depth pass walks from fonts and images (that is what pdfcpu's optimize pass follows every reference under). ring
// closes the last link back on the first instead of ending it.
func ignoredChain(links, nest int, ring bool) map[int]string {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources << /Font << /F 9 0 R >> >> >>",
		9: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Foo 10 0 R >>",
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
// them, because a walk that enters an object once can cross them all. The chain hangs off the page's font (object 9),
// where the walk starts.
func TestLevelsCountEveryContainerOnTheWayToAReference(t *testing.T) {
	for _, c := range []struct {
		name        string
		links, nest int
		ring        bool
		want        int
	}{
		{"fifty links", 50, 1, false, 52},                          // the font, fifty links, the end
		{"fifty links, each three arrays deep", 50, 3, false, 152}, // the font, then three levels a link
		{"a ring of ten", 10, 1, true, 11},                         // the font, then all ten
		{"no chain", 0, 1, false, 2},                               // the font and the one array it names
	} {
		got, from := pdfread.ObjectLevels(unvalidated(t, testpdf.Assemble(ignoredChain(c.links, c.nest, c.ring))))
		if got != c.want || from != 9 {
			t.Errorf("%s: %d levels from object %d, want %d from object 9", c.name, got, from, c.want)
		}
	}
}

// TestALargeHonestDocumentPassesTheDoor is the rework's acceptance (/pending 840, coordinator). A real document's
// size is in its pages, and pages name each other back and forth through /Parent — the shape that made a size-based
// bound refuse honest work. The depth pass walks from fonts and images, which do not reach the page tree, so 20,000
// pages — with /Parent links, one shared resource dictionary and every page in object streams — measure the same few
// levels a one-page document does, and the door passes them.
func TestALargeHonestDocumentPassesTheDoor(t *testing.T) {
	pdf := testpdf.ManyPages(20000)
	ctx := unvalidated(t, pdf)
	levels, _ := pdfread.ObjectLevels(ctx)
	if levels > 100 {
		t.Fatalf("20,000 honest pages measure %d levels — a size-based bound is back", levels)
	}
	start := time.Now()
	if err := pdfread.RefuseUnbounded(ctx); err != nil {
		t.Fatalf("20,000 honest pages (%d bytes) refused: %v", len(pdf), err)
	}
	t.Logf("20,000 pages: %d bytes, %d levels, door %v", len(pdf), levels, time.Since(start))

	// And a structure of 600,000 objects naming each other in a ring, off the catalog and not a font — larger than
	// maxObjectLevels if charged whole, which is what a page tree through /Parent is. The depth pass walks only fonts
	// and images, so it never enters this; a pass that walked everything (the fontOrImage scoping removed) would
	// refuse it. This is the red-proof that the scoping is what keeps an honest large document from being refused.
	ring := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Ring 10 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] >>",
	}
	const ringN = 600000 // > maxObjectLevels (2^19 = 524288)
	for i := 0; i < ringN; i++ {
		ring[10+i] = fmt.Sprintf("<< /Next %d 0 R >>", 10+(i+1)%ringN)
	}
	if err := pdfread.RefuseUnbounded(unvalidated(t, testpdf.Assemble(ring))); err != nil {
		t.Fatalf("a %d-object ring off the catalog, not a font, was refused: %v", ringN, err)
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

// TestTheFreeReferencePassLetsPdfcpuSkipItsOwnRecursion — /pending 840's free-reference half. pdfcpu's optimize pass
// walks every reference from the catalog recursively (`fixReferencesToFreeObjects`), one frame a link, and a chain
// 300,000 links deep under a key the validator ignores overflowed its stack — fatally. The depth pass does NOT cover
// this chain (it walks only fonts and images, and this hangs off the catalog), so what keeps pdfcpu from recursing is
// `fixFreeReferences` making the same walk iteratively first and pre-seeding pdfcpu's cache. With it, optimizing the
// document returns; the mutation that removes the pass (export `FreeReferencePass`) overflows the stack, so the
// red-proof for this one is a process death and lives in the mutation harness, not here.
func TestTheFreeReferencePassLetsPdfcpuSkipItsOwnRecursion(t *testing.T) {
	// 1,600,000 array links: past pdfcpu's own recursive overflow at the default 1 GB max stack (measured: 1,000,000
	// survives, 1,500,000 overflows), off the catalog so the depth pass — fonts and images only — never sees it.
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Foo 10 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] >>",
	}
	const links = 1600000
	for i := 0; i < links; i++ {
		objs[10+i] = fmt.Sprintf("[%d 0 R]", 11+i)
	}
	objs[10+links] = "[]"
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.OPTIMIZE
	done := make(chan error, 1)
	go func() { _, err := pdfread.ReadOptimized(testpdf.Assemble(objs), conf); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("optimizing a 300,000-link chain under an ignored key: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("optimizing a 300,000-link chain did not return")
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

// sharedAction is a one-page document with one action (object 50) over a chain of depth more, each naming the next
// twice in its /Next — 2^(depth+1)-1 paths — which items outline items and links link annotations name as their /A.
func sharedAction(items, links, depth int) []byte {
	o := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Outlines 4 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		4: fmt.Sprintf("<< /Type /Outlines /First 1000 0 R /Last %d 0 R /Count %d >>", 1000+items-1, items),
	}
	for j := 0; j < depth; j++ {
		o[50+j] = fmt.Sprintf("<< /S /GoTo /D [3 0 R /Fit] /Next [%d 0 R %d 0 R] >>", 51+j, 51+j)
	}
	o[50+depth] = "<< /S /GoTo /D [3 0 R /Fit] >>"
	for i := 0; i < items; i++ {
		s := "<< /Title (x) /Parent 4 0 R /A 50 0 R"
		if i > 0 {
			s += fmt.Sprintf(" /Prev %d 0 R", 1000+i-1)
		}
		if i < items-1 {
			s += fmt.Sprintf(" /Next %d 0 R", 1000+i+1)
		}
		o[1000+i] = s + " >>"
	}
	var annots []string
	for i := 0; i < links; i++ {
		o[5000+i] = "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /A 50 0 R >>"
		annots = append(annots, fmt.Sprintf("%d 0 R", 5000+i))
	}
	o[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Annots [" + strings.Join(annots, " ") + "] >>"
	return testpdf.Assemble(o)
}

// TestAnActionIsCountedOncePerNamingTheWalkDoesNotFollow — /pending 767 (5). pdfcpu validates an outline item's
// action, and every path under its /Next, once per item; the walk follows no outline, so sixty-four items over one
// action counted as one and validated for a minute (measured, 28 KB). Each naming no followed edge accounts for is
// charged the action's paths; a naming the walk does follow (a link's /A) is not charged twice, and an action with
// nothing under it is not charged at all.
func TestAnActionIsCountedOncePerNamingTheWalkDoesNotFollow(t *testing.T) {
	paths := func(items, links, depth int) int {
		n, err := pdfread.ValidatorPaths(unvalidated(t, sharedAction(items, links, depth)))
		if err != nil {
			t.Fatalf("%d items, %d links, depth %d: %v", items, links, depth, err)
		}
		return int(n)
	}
	const depth = 12
	under := 1<<(depth+1) - 1 // the paths from the action
	if one, many := paths(1, 0, depth), paths(64, 0, depth); many-one != 63*under {
		t.Errorf("64 outline items over one action count %d paths more than one item does, want 63 × %d", many-one, under)
	}
	if none, one := paths(0, 0, depth), paths(1, 0, depth); one != none {
		t.Errorf("one outline item over the action counts %d paths, the action alone %d: its one naming is the count it already has", one, none)
	}
	if one, many := paths(1, 1, depth), paths(1, 64, depth); many-one != 63*(under+1) {
		t.Errorf("64 links over one action count %d paths more than one link does, want 63 × %d — a followed naming is charged twice", many-one, under+1)
	}
	if links, both := paths(0, 2, depth), paths(3, 2, depth); both-links != 3*under {
		t.Errorf("3 outline items beside 2 links count %d paths more than the links alone, want 3 × %d", both-links, under)
	}
	if one, many := paths(1, 0, 0), paths(64, 0, 0); many != one {
		t.Errorf("64 outline items over an action that leads nowhere count %d paths, one item %d: that is linear in the file and not charged", many, one)
	}
	if _, err := pdfread.Validated(sharedAction(1, 0, depth), model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("control: one outline item over the action does not read: %v", err)
	}
	refusedAs(t, pdfread.ErrReferencePaths, "64 outline items over one action of 8,191 paths", func() error {
		_, err := pdfread.Validated(sharedAction(64, 0, depth), model.NewDefaultConfiguration())
		return err
	})
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
