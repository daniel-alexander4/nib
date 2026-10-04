package pdfread

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"nib/internal/scaling"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// flatInherited is n pages under ONE /Pages node, drawing a form they inherit from it — the shape whose resource
// step was quadratic (`/pending 825`), with something for the step to consolidate on every page.
func flatInherited(n int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		4: "<< /Length 6 >>\nstream\n/Fm Do\nendstream",
		5: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 14 >>\nstream\n0 0 10 10 re f\nendstream",
		6: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 14 >>\nstream\n0 0 20 20 re f\nendstream",
	}
	var kids strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", 10+i)
		objs[10+i] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>"
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /Resources << /XObject << /Fm 5 0 R /Unused 6 0 R >> >> >>", kids.String(), n)
	return assembleObjs(objs)
}

// assembleObjs is testpdf.Assemble, which this package's internal tests cannot import (testpdf imports pdfread).
func assembleObjs(objs map[int]string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	maxN := 0
	for n := range objs {
		maxN = max(maxN, n)
	}
	offs := map[int]int{}
	for n := 1; n <= maxN; n++ {
		if body, ok := objs[n]; ok {
			offs[n] = b.Len()
			fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
		}
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", maxN+1)
	for n := 1; n <= maxN; n++ {
		if off, ok := offs[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", maxN+1, xref)
	return b.Bytes()
}

// passReads is what the pass leaves on every page, rendered canonically: each page's dictionary with every reference
// replaced by what it names (keys sorted, streams by their dictionary and a digest of their bytes, `/Parent` left out
// so the render stays below the page). Object numbers are not in it, nor written bytes: pdfcpu keeps one of two equal
// fonts by map order, so which NUMBER survives is not stable run to run, and neither is what it writes (302 of 335
// corpus documents wrote different bytes twice under pdfcpu alone) — but what each page resolves to is.
func passReads(pdf []byte) (string, error) {
	ctx, err := ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return "", err
	}
	memo := map[int]string{}
	var render func(o types.Object, onPath map[int]bool, depth int) string
	render = func(o types.Object, onPath map[int]bool, depth int) string {
		if depth > 64 {
			return "…"
		}
		switch v := o.(type) {
		case types.IndirectRef:
			nr := v.ObjectNumber.Value()
			if onPath[nr] {
				return "↺"
			}
			if m, ok := memo[nr]; ok {
				return m
			}
			t, err := ctx.Dereference(v)
			if err != nil {
				return "!"
			}
			onPath[nr] = true
			out := render(t, onPath, depth+1)
			delete(onPath, nr)
			memo[nr] = out
			return out
		case types.Dict:
			keys := make([]string, 0, len(v))
			for k := range v {
				if k != "Parent" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			var b strings.Builder
			b.WriteString("<<")
			for _, k := range keys {
				b.WriteString("/" + k + " " + render(v[k], onPath, depth+1) + " ")
			}
			return b.String() + ">>"
		case types.StreamDict:
			return render(v.Dict, onPath, depth+1) + fmt.Sprintf("stream%x", sha256.Sum256(v.Raw))
		case types.Array:
			parts := make([]string, len(v))
			for i, e := range v {
				parts[i] = render(e, onPath, depth+1)
			}
			return "[" + strings.Join(parts, " ") + "]"
		case nil:
			return "null"
		default:
			return v.PDFString()
		}
	}
	var b strings.Builder
	for i, pg := range Pages(ctx) {
		fmt.Fprintf(&b, "page %d: %s\n", i+1, render(pg.Dict, map[int]bool{}, 0))
	}
	return b.String(), nil
}

// TestBalancingThePageTreeChangesNothingThePassLeaves — `/pending 825`: the reshaping is invisible in what the pass
// leaves on every page. Each document is read three ways: with the reshaping off (twice — a document whose reading is
// not stable under pdfcpu alone cannot be compared, and is counted), and with it forced onto every /Pages node of more
// than TWO kids, so it reshapes every multi-page document in the corpora rather than only the wide ones.
func TestBalancingThePageTreeChangesNothingThePassLeaves(t *testing.T) {
	docs := map[string][]byte{"flat 300": flatInherited(300), "flat 3": flatInherited(3)}
	home, _ := os.UserHomeDir()
	for _, root := range []string{filepath.Join(home, "nib", "producers"), filepath.Join(home, "nib", "verapdfs")} {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
				if b, rerr := os.ReadFile(p); rerr == nil {
					docs[p] = b
				}
			}
			return nil
		})
	}
	saved := pageNodeFanout
	defer func() { pageNodeFanout = saved }()
	compared, unstable, unreadable, reshaped := 0, 0, 0, 0
	for name, pdf := range docs {
		pageNodeFanout = 1 << 30
		off, err := passReads(pdf)
		if err != nil {
			unreadable++
			continue
		}
		again, _ := passReads(pdf)
		if off != again {
			unstable++
			continue
		}
		pageNodeFanout = 2
		on, err := passReads(pdf)
		if err != nil {
			t.Errorf("%s: reads with the tree as it is, and fails reshaped: %v", filepath.Base(name), err)
			continue
		}
		compared++
		if ctx, err := Validated(pdf, model.NewDefaultConfiguration()); err == nil && ctx.PageCount > 2 {
			reshaped++
		}
		if off != on {
			t.Errorf("%s: a page resolves differently after the pass over the reshaped tree", filepath.Base(name))
		}
	}
	t.Logf("%d documents: %d compared (%d with more than two pages, so reshaped), %d not stable under pdfcpu "+
		"alone, %d unreadable", len(docs), compared, reshaped, unstable, unreadable)
	if compared < 2 {
		t.Fatalf("only %d documents compared — the differential saw nothing", compared)
	}
}

// TestTheReshapingIsGoneWhenThePassReturns: the temporary nodes are out of the table and every /Kids is the one the
// file wrote, so nothing after the pass — a write, `Pages`, a later PageDict — sees them.
func TestTheReshapingIsGoneWhenThePassReturns(t *testing.T) {
	pdf := flatInherited(300)
	ctx, err := Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	before := len(ctx.Table)
	root, _ := ctx.Pages()
	d, _ := ctx.DereferenceDict(*root)
	kids := d["Kids"]
	restore := balancePageTreeForPass(ctx)
	// Stimulus: the reshaping happened.
	if len(ctx.Table) == before || fmt.Sprint(d["Kids"]) == fmt.Sprint(kids) {
		t.Fatal("setup: a 300-kid /Pages node was not reshaped")
	}
	if pd, _, _, err := ctx.PageDict(300, false); err != nil || pd == nil {
		t.Fatalf("page 300 is not found through the reshaped tree: %v", err)
	}
	restore()
	if len(ctx.Table) != before {
		t.Errorf("the table holds %d entries after the restore, %d before", len(ctx.Table), before)
	}
	if fmt.Sprint(d["Kids"]) != fmt.Sprint(kids) {
		t.Error("the root's /Kids is not the one the file wrote after the restore")
	}
	// And through the real door: ReadOptimized leaves no node the file did not hold.
	ctx, err = ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	for nr, e := range ctx.Table {
		if dd, ok := e.Object.(types.Dict); ok && dd.Type() != nil && *dd.Type() == "Pages" && nr != root.ObjectNumber.Value() {
			t.Errorf("object %d is a /Pages node the file did not hold", nr)
		}
	}
}

// TestTheResourceStepIsLinearInPagesOnAFlatTree — `/pending 825`: measured before the reshaping, ReadOptimized over a
// clean flat tree cost 4.1 s at 5,000 pages, quadratic. Four times the pages must cost nowhere near sixteen times.
func TestTheResourceStepIsLinearInPagesOnAFlatTree(t *testing.T) {
	// Through the one clock door (`internal/scaling`): interleaved rounds, the least ratio — this test's own
	// best-of-three per size, measured one size after the other, went red at 8.6x under suite load.
	pdfs := map[int][]byte{}
	scaling.GrowsLinearly(t, "ReadOptimized over a flat tree", 1500, 6000, 8, func(n int) time.Duration {
		if pdfs[n] == nil {
			pdfs[n] = flatInherited(n)
		}
		return scaling.TimeOnce(func() {
			if _, err := ReadOptimized(pdfs[n], model.NewDefaultConfiguration()); err != nil {
				t.Fatal(err)
			}
		})
	})
}
