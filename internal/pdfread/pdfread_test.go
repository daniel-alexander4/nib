package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 675`: a `/UseCMap` loop recursed pdfcpu's validator into a fatal stack overflow — the nib process
// died, not the request. **On the code before this package these tests do not fail, they kill the test
// binary** (~5 s, "goroutine stack exceeds 1000000000-byte limit"), which is the defect itself.

// chainFrom follows `/UseCMap` references from nr in the RAW read — the stimulus, read without the door.
func chainFrom(t *testing.T, pdf []byte, nr, steps int) []int {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: the unvalidated read failed: %v", err)
	}
	out := []int{nr}
	for len(out) <= steps {
		sd, _, err := ctx.DereferenceStreamDict(*types.NewIndirectRef(nr, 0))
		if err != nil || sd == nil {
			break
		}
		ir, ok := sd.Dict["UseCMap"].(types.IndirectRef)
		if !ok {
			break
		}
		nr = ir.ObjectNumber.Value()
		out = append(out, nr)
	}
	return out
}

func refusedWithin(t *testing.T, what string, f func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		if !errors.Is(err, pdfread.ErrReferenceCycle) {
			t.Errorf("%s returned %v, want the /UseCMap loop refused (pdfread.ErrReferenceCycle)", what, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("%s had not returned after 10 s", what)
	}
}

func TestAUseCMapLoopIsRefusedBeforeTheValidator(t *testing.T) {
	for name, c := range testpdf.UseCMapLoops() {
		// Stimulus first: the raw object graph really loops, starting from the font's /Encoding.
		if got := chainFrom(t, c.PDF, 20, len(c.Loop)-1); fmt.Sprint(got) != fmt.Sprint(c.Loop) {
			t.Fatalf("setup: %s's /UseCMap chain from 20 is %v, want %v", name, got, c.Loop)
		}
		raw, err := api.ReadContext(bytes.NewReader(c.PDF), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("setup: %s — the unvalidated read failed: %v", name, err)
		}
		if _, err := pdfread.ValidatorPaths(raw); !errors.Is(err, pdfread.ErrReferenceCycle) {
			t.Errorf("%s: the reference walk returned %v, want the loop found", name, err)
			continue
		}
		conf := model.NewDefaultConfiguration()
		refusedWithin(t, "Validated over "+name, func() error { _, err := pdfread.Validated(c.PDF, conf); return err })
		refusedWithin(t, "ReadOptimized over "+name, func() error { _, err := pdfread.ReadOptimized(c.PDF, conf); return err })
		refusedWithin(t, "Reader over "+name, func() error { _, err := pdfread.Reader(c.PDF, nil); return err })
	}
}

// A chain that ENDS — a CMap using another that uses a predefined one — is what /UseCMap is for, and must read.
func TestAUseCMapChainThatEndsStillReads(t *testing.T) {
	pdf := testpdf.UseCMapChain()
	if got := chainFrom(t, pdf, 20, 5); fmt.Sprint(got) != "[20 21]" {
		t.Fatalf("setup: the chain from 20 is %v, want [20 21] (21 names a predefined CMap)", got)
	}
	if _, err := pdfread.Validated(pdf, model.NewDefaultConfiguration()); err != nil {
		t.Errorf("Validated refused a /UseCMap chain with no loop: %v", err)
	}
	rs, err := pdfread.Reader(pdf, nil)
	if err != nil {
		t.Fatalf("Reader refused a /UseCMap chain with no loop: %v", err)
	}
	if n, err := api.PageCount(rs, model.NewDefaultConfiguration()); err != nil || n != 1 {
		t.Errorf("pdfcpu over Reader's reader: %d pages, %v; want 1", n, err)
	}
}

// `/pending 764`: every edge pdfcpu's validator follows without a guard (the table in refgraph.go). Each fixture's
// Ends must VALIDATE — the setup check that the pair differs only in the loop, and that pdfcpu reads the chain it
// is built on — and each Loop must be refused before the validator is reached. As with the `/UseCMap` tests
// above, on a door that misses an edge this does not fail: it kills the test binary.
func TestEveryUnguardedReferenceLoopIsRefusedBeforeTheValidator(t *testing.T) {
	for _, c := range testpdf.RefLoops() {
		conf := model.NewDefaultConfiguration()
		if _, err := pdfread.Validated(c.Ends, conf); err != nil {
			t.Errorf("setup: %s — the chain that ends does not validate: %v", c.Edge, err)
			continue
		}
		// The walk first, on the unvalidated read: a door missing this edge fails here, and the loop is never handed
		// to the validator that would kill the binary.
		raw, err := api.ReadContext(bytes.NewReader(c.Loop), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("setup: %s — the unvalidated read failed: %v", c.Edge, err)
		}
		if _, err := pdfread.ValidatorPaths(raw); !errors.Is(err, pdfread.ErrReferenceCycle) {
			t.Errorf("%s: the reference walk returned %v, want the loop found (pdfread.ErrReferenceCycle)", c.Edge, err)
			continue
		}
		refusedAs(t, pdfread.ErrReferenceCycle, "Validated over "+c.Edge, func() error { _, err := pdfread.Validated(c.Loop, conf); return err })
		refusedAs(t, pdfread.ErrReferenceCycle, "Reader over "+c.Edge, func() error { _, err := pdfread.Reader(c.Loop, nil); return err })
	}
}

// Sharing is walked once per path by pdfcpu's validator — measured 2.8 s at depth 20 from 4.8 KB, doubling per
// level — so it is refused past the path budget, and quickly. Sharing within it reads.
func TestSharedReferencesAreRefusedOnlyPastThePathBudget(t *testing.T) {
	if _, err := pdfread.Validated(testpdf.SharedPatterns(3), model.NewDefaultConfiguration()); err != nil {
		t.Errorf("a pattern shared by two names three levels deep was refused: %v", err)
	}
	if _, err := pdfread.Validated(testpdf.SharedNameTree(3), model.NewDefaultConfiguration()); err != nil {
		t.Errorf("a name tree sharing its kids three levels deep was refused: %v", err)
	}
	refusedAs(t, pdfread.ErrReferencePaths, "Validated over a name tree sharing its kids 30 deep", func() error {
		_, err := pdfread.Validated(testpdf.SharedNameTree(30), model.NewDefaultConfiguration())
		return err
	})
	for _, depth := range []int{25, 60} {
		start := time.Now()
		refusedAs(t, pdfread.ErrReferencePaths, fmt.Sprintf("Validated over shared patterns %d deep", depth), func() error {
			_, err := pdfread.Validated(testpdf.SharedPatterns(depth), model.NewDefaultConfiguration())
			return err
		})
		if d := time.Since(start); d > time.Second {
			t.Errorf("refusing shared patterns %d deep took %v; the door must answer before the validator's cost, not after", depth, d)
		}
	}
}

// A chain with no loop and no sharing still costs pdfcpu a set of stack frames per level.
func TestAReferenceChainDeeperThanTheBoundIsRefused(t *testing.T) {
	// A chain is walked once by pdfcpu, so it is counted once: 5,000 links are 5,000 paths, not one set of paths per
	// link (~12.5 million, which the count once charged and refused).
	if _, err := pdfread.Validated(testpdf.ChainedPatterns(5000), model.NewDefaultConfiguration()); err != nil {
		t.Errorf("a chain of 5000 patterns was refused: %v", err)
	}
	refusedAs(t, pdfread.ErrReferenceDepth, "Validated over a chain of 9000 patterns", func() error {
		_, err := pdfread.Validated(testpdf.ChainedPatterns(9000), model.NewDefaultConfiguration())
		return err
	})
}

func refusedAs(t *testing.T, want error, what string, f func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Errorf("%s returned %v, want %v", what, err, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("%s had not returned after 10 s", what)
	}
}

// pdfcpu validates each page's resources afresh, so a structure every page shares is walked once per page: the
// door's count must follow each way a page's /Resources reaches a function, or it counts the sharing once. One
// page reads; four hundred pages multiply the same 2^12 paths past the budget.
func TestSharingAcrossPagesIsCountedThroughEveryResourceEntry(t *testing.T) {
	for entry := range testpdf.FanInEntries {
		pdf := testpdf.FanInEntries[entry]
		if _, err := pdfread.Validated(testpdf.FanIn(pdf, 1, 12), model.NewDefaultConfiguration()); err != nil {
			t.Errorf("setup: one page through %s does not read: %v", entry, err)
			continue
		}
		refusedAs(t, pdfread.ErrReferencePaths, "400 pages through "+entry, func() error {
			_, err := pdfread.Validated(testpdf.FanIn(pdf, 400, 12), model.NewDefaultConfiguration())
			return err
		})
	}
}

// sparseDoc is a one-page document whose content stream is object number nr, written with a sparse xref (two
// subsections), so the file stays a few hundred bytes however large nr is.
func sparseDoc(nr int) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	off := map[int]int{}
	for _, o := range []struct {
		n int
		s string
	}{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents %d 0 R >>", nr)},
		{nr, "<< /Length 0 >>\nstream\n\nendstream"},
	} {
		off[o.n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.n, o.s)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 4\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%d 1\n%010d 00000 n \n",
		off[1], off[2], off[3], nr, off[nr])
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", nr+1, x)
	return b.Bytes()
}

// The reference door's working memory follows the number of objects, not the highest object NUMBER (the P01
// phase-close review): its slots were sized by the largest number, so a 400-byte file naming object 2^25 cost 6.1 GB.
func TestTheReferenceWalkIsSizedByObjectsNotByTheirNumbers(t *testing.T) {
	pdf := sparseDoc(1 << 20)
	raw, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: the sparse document does not read: %v", err)
	}
	if _, ok := raw.Table[1<<20]; !ok || len(raw.Table) > 16 {
		t.Fatalf("setup: the table holds %d entries and object 2^20 is present=%v; want a handful, including it", len(raw.Table), ok)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, err := pdfread.ValidatorPaths(raw); err != nil {
		t.Fatalf("the walk refused a sparse, loop-free document: %v", err)
	}
	runtime.ReadMemStats(&after)
	if got := after.TotalAlloc - before.TotalAlloc; got > 4<<20 {
		t.Errorf("the walk allocated %d bytes over %d objects — it is sized by the highest object number", got, len(raw.Table))
	}
}

// pagesSharing builds m pages that all name one indirect `/Annots` array of k slots, each naming annotation 4, whose
// action chain is n long — and, when aa, give every page one shared `/AA` whose open action is that chain's head
// instead. resources puts `/Resources << >>` on each page: the old door walked a page only when it had them.
func pagesSharing(m, k, n int, aa, resources bool) []byte {
	first := 5 + n
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		3: "[" + strings.Repeat("4 0 R ", k) + "]",
		4: "<< /Type /Annot /Subtype /Link /Rect [0 0 1 1] /A 5 0 R >>",
	}
	res := ""
	if resources {
		res = " /Resources << >>"
	}
	var kids []string
	for i := 0; i < m; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", first+i))
		if aa {
			objs[first+i] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10]" + res + " /AA << /O 5 0 R >> >>"
		} else {
			objs[first+i] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10]" + res + " /Annots 3 0 R >>"
		}
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), m)
	for i := 0; i < n; i++ {
		act := fmt.Sprintf("<< /S /GoTo /D [%d 0 R /Fit] /Next %d 0 R >>", first, 6+i)
		if i == n-1 {
			act = fmt.Sprintf("<< /S /GoTo /D [%d 0 R /Fit] >>", first)
		}
		objs[5+i] = act
	}
	return testpdf.Assemble(objs)
}

// TestAPageIsCountedWhetherOrNotItHasResources — /pending 800. pdfcpu validates every page's `/Annots` and `/AA` once
// per page; the door counted a page only when it carried `/Resources`, so 200 pages sharing one `/Annots` array that
// names one annotation 200 times, its action chain 30 long, validated for 5.4 s and passed (measured), while the same
// shape with `/Resources << >>` on each page was refused in milliseconds.
func TestAPageIsCountedWhetherOrNotItHasResources(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	// Stimulus: WITH resources the shape is refused — the door can see it at all, so the difference below is the gate.
	if _, err := pdfread.Validated(pagesSharing(200, 200, 30, false, true), conf); !errors.Is(err, pdfread.ErrReferencePaths) {
		t.Fatalf("setup: the shared-slot shape WITH /Resources was not refused (%v), so the door cannot see it at all", err)
	}
	for _, c := range []struct {
		name string
		pdf  []byte
	}{
		{"200 pages without /Resources sharing one /Annots of 200 slots over a 30-action chain", pagesSharing(200, 200, 30, false, false)},
		{"1,200 pages without /Resources sharing one /AA whose open action heads a 300-action chain", pagesSharing(1200, 0, 300, true, false)},
	} {
		start := time.Now()
		refusedAs(t, pdfread.ErrReferencePaths, c.name, func() error {
			_, err := pdfread.Validated(c.pdf, conf)
			return err
		})
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s: refusing took %v — the door must answer before the validator's cost", c.name, d)
		}
	}
	// The same shapes at a size pdfcpu validates quickly still read: a page is counted, not refused for being a page.
	for _, pdf := range [][]byte{pagesSharing(20, 20, 30, false, false), pagesSharing(20, 0, 30, true, false)} {
		if _, err := pdfread.Validated(pdf, conf); err != nil {
			t.Errorf("a small shared shape was refused: %v", err)
		}
	}
}

// freeEntriesDoc is a three-object document whose classic xref also declares n FREE entries — about 20 bytes each.
func freeEntriesDoc(n int) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	var off []int
	for _, s := range []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] >>",
	} {
		off = append(off, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(off), s)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", n+4)
	for _, o := range off {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	for i := 0; i < n; i++ {
		b.WriteString("0000000000 00001 f \n")
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", n+4, x)
	return b.Bytes()
}

// TestFreeXrefEntriesDoNotRaiseThePathBudget — /pending 811. The budget grows per LIVE object, the population the
// walk counts; counting every table entry let a file buy itself budget with free entries (1,000,000 of them took a
// 3-object document from 262,208 paths to 16,262,208 — 62×, measured).
func TestFreeXrefEntriesDoNotRaiseThePathBudget(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	plain, err := api.ReadContext(bytes.NewReader(freeEntriesDoc(0)), conf)
	if err != nil {
		t.Fatal(err)
	}
	padded, err := api.ReadContext(bytes.NewReader(freeEntriesDoc(100000)), conf)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the padded file really carries the entries, or the budgets below agree for the wrong reason.
	if len(padded.Table) < 100000 {
		t.Fatalf("setup: the padded table holds %d entries, not the 100,000 free ones declared", len(padded.Table))
	}
	if a, b := pdfread.PathBudget(plain), pdfread.PathBudget(padded); a != b {
		t.Errorf("100,000 free xref entries moved the path budget from %d to %d — a file can buy itself validator time", a, b)
	}
}
