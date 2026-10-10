package pdfread_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	_ "unsafe" // go:linkname, below

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// pdfcpuFixFree is pdfcpu's own, unexported `fixReferencesToFreeObjects` (optimize.go:1526, v0.13.0) — the recursive
// pass `pdfread.fixFreeReferences` restates. Reached by name so the two can be run side by side on one document; a
// pdfcpu that renames it fails this file's link, which is the moment to re-read the pass.
//
//go:linkname pdfcpuFixFree github.com/pdfcpu/pdfcpu/pkg/pdfcpu.fixReferencesToFreeObjects
func pdfcpuFixFree(ctx *model.Context) error

// contextState is everything the free-reference pass can change, written deterministically: every table entry in
// number order with each reference by its number, the pass's cache, and the null object it allocated.
func contextState(ctx *model.Context) string {
	var canon func(o types.Object) string
	canon = func(o types.Object) string {
		switch o := o.(type) {
		case nil:
			return "null"
		case types.IndirectRef:
			return fmt.Sprintf("%d %d R", o.ObjectNumber.Value(), o.GenerationNumber.Value())
		case types.Dict:
			keys := make([]string, 0, len(o))
			for k := range o {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, "/"+k+" "+canon(o[k]))
			}
			return "<<" + strings.Join(parts, " ") + ">>"
		case types.StreamDict:
			return "stream" + canon(o.Dict)
		case types.Array:
			parts := make([]string, 0, len(o))
			for _, v := range o {
				parts = append(parts, canon(v))
			}
			return "[" + strings.Join(parts, " ") + "]"
		}
		return fmt.Sprintf("%T:%s", o, o.PDFString())
	}
	nrs := make([]int, 0, len(ctx.Table))
	for nr := range ctx.Table {
		nrs = append(nrs, nr)
	}
	sort.Ints(nrs)
	var b strings.Builder
	for _, nr := range nrs {
		switch e := ctx.Table[nr]; {
		case e == nil:
			fmt.Fprintf(&b, "%d: no entry\n", nr)
		case e.Free:
			fmt.Fprintf(&b, "%d: free\n", nr)
		default:
			fmt.Fprintf(&b, "%d: %s\n", nr, canon(e.Object))
		}
	}
	cache := make([]int, 0, len(ctx.Optimize.Cache))
	for nr, in := range ctx.Optimize.Cache {
		if in {
			cache = append(cache, nr)
		}
	}
	sort.Ints(cache)
	fmt.Fprintf(&b, "cache: %v\nnull object: ", cache)
	if ctx.Optimize.NullObjNr == nil {
		b.WriteString("none\n")
	} else {
		fmt.Fprintf(&b, "%d\n", *ctx.Optimize.NullObjNr)
	}
	return b.String()
}

// bothPasses reads pdf twice through the door and validator, runs pdfcpu's pass on one context and nib's on the other,
// and returns each one's state.
func bothPasses(t *testing.T, pdf []byte) (theirs, ours string, rewrote bool) {
	t.Helper()
	read := func() *model.Context {
		ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("setup: the fixture does not read: %v", err)
		}
		return ctx
	}
	a, b := read(), read()
	if err := pdfcpuFixFree(a); err != nil {
		t.Fatalf("pdfcpu's pass: %v", err)
	}
	pdfread.FixFreeReferences(b)
	return contextState(a), contextState(b), b.Optimize.NullObjNr != nil
}

// TestTheFreeReferencePassLeavesWhatPdfcpusLeaves — nib makes pdfcpu's free-reference walk ahead of pdfcpu's own
// (/pending 840), so it must leave the context exactly as pdfcpu's would: the same references replaced by the null
// object, in the same holders, the same cache. Real documents do not name free objects — 0 of the 331
// files of nib's producer corpus and veraPDF's PDF/UA-1 corpus name a free or a missing one (measured; the two passes
// agree on all 331) — so the replacing branch is held by these shapes alone. Object 7 and 8 are free in each: a number the
// file's cross-reference table lists and no object has. Where one free object is named twice, the holders hang off an
// ARRAY, whose order pdfcpu walks in — a dictionary's order is Go's, in both passes.
func TestTheFreeReferencePassLeavesWhatPdfcpusLeaves(t *testing.T) {
	doc := func(catalog string, objs map[int]string) map[int]string {
		o := map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R " + catalog + " >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] >>",
		}
		for nr, body := range objs {
			o[nr] = body
		}
		return o
	}
	for _, c := range []struct {
		name     string
		objs     map[int]string
		packed   []int
		rewrites bool
	}{
		{"no free reference", doc("/Foo 10 0 R", map[int]string{10: "[11 0 R]", 11: "<< /A 3 0 R >>"}), nil, false},
		{"directly in a dictionary", doc("/Foo 10 0 R", map[int]string{10: "<< /A 7 0 R >>"}), nil, true},
		{"inside an array", doc("/Foo 10 0 R", map[int]string{10: "[1 7 0 R 2]"}), nil, true},
		{"inside nested direct containers", doc("/Foo 10 0 R", map[int]string{10: "<< /A << /B [<< /C 7 0 R >>] >> >>"}), nil, true},
		{"in a stream's dictionary", doc("/Foo 10 0 R", map[int]string{10: "<< /A 7 0 R /Length 0 >>\nstream\n\nendstream"}), nil, true},
		{"in the catalog itself", doc("/Foo 7 0 R", map[int]string{10: "[]"}), nil, true},
		{"in the catalog, inside a direct array", doc("/Foo [7 0 R]", map[int]string{10: "[]"}), nil, true},
		{"in an object held in an object stream", doc("/Foo 10 0 R", map[int]string{10: "<< /A 7 0 R >>", 11: "[]"}), []int{10, 11}, true},
		{"two free objects", doc("/Foo 10 0 R", map[int]string{10: "[7 0 R 8 0 R]"}), nil, true},
		{"one free object twice in one array", doc("/Foo 10 0 R", map[int]string{10: "[7 0 R 7 0 R]"}), nil, true},
		{"one free object from two holders", doc("/Foo [10 0 R 11 0 R]", map[int]string{10: "[7 0 R]", 11: "[7 0 R]"}), nil, true},
		// pdfcpu finishes object 11 before it returns to 10's second element, so the reference it replaces is 11's.
		{"one free object, met first below a sibling", doc("/Foo 10 0 R", map[int]string{10: "[11 0 R 7 0 R]", 11: "[7 0 R]"}), nil, true},
		{"one free object, met first beside a deeper holder", doc("/Foo 10 0 R", map[int]string{10: "[7 0 R 11 0 R]", 11: "[[7 0 R]]"}), nil, true},
		{"an object the file does not list", doc("/Foo 10 0 R", map[int]string{10: "[999 0 R]"}), nil, false},
		{"a holder named in a loop", doc("/Foo 10 0 R", map[int]string{10: "[11 0 R 7 0 R]", 11: "[10 0 R 7 0 R]"}), nil, true},
	} {
		pdf := testpdf.Assemble(c.objs)
		if c.packed != nil {
			pdf = testpdf.AssembleCompressed(c.objs, c.packed...)
		}
		theirs, ours, rewrote := bothPasses(t, pdf)
		if rewrote != c.rewrites {
			t.Errorf("%s: nib's pass replaced a reference = %v, want %v — the fixture does not hold the shape it names", c.name, rewrote, c.rewrites)
		}
		if theirs != ours {
			t.Errorf("%s: nib's pass leaves a different context than pdfcpu's.\npdfcpu:\n%s\nnib:\n%s", c.name, theirs, ours)
		}
	}
}
