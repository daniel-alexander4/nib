package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
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
