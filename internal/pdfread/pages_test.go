package pdfread_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// /pending 753: `pdfread.Pages` answers `ctx.PageDict(p, false)` for every page from one walk. These hold it to
// that answer — the same dictionary (by identity: callers write through it), reference, inherited attributes
// and error — on every tree shape where the walk and pdfcpu could part, and to linear cost.

// samePage fails unless got is exactly PageDict(p, false).
func samePages(t *testing.T, name string, ctx *model.Context) {
	t.Helper()
	got := pdfread.Pages(ctx)
	if len(got) != ctx.PageCount {
		t.Fatalf("%s: Pages returned %d pages for a %d-page document", name, len(got), ctx.PageCount)
	}
	for i, g := range got {
		p := i + 1
		d, ref, attrs, err := ctx.PageDict(p, false)
		if g.Nr != p {
			t.Fatalf("%s: page %d reported as number %d", name, p, g.Nr)
		}
		if (err == nil) != (g.Err == nil) {
			t.Fatalf("%s page %d: PageDict err %v, Pages err %v", name, p, err, g.Err)
		}
		if reflect.ValueOf(d).Pointer() != reflect.ValueOf(g.Dict).Pointer() {
			t.Fatalf("%s page %d: Pages returned a different dictionary from PageDict's", name, p)
		}
		if !reflect.DeepEqual(ref, g.Ref) {
			t.Fatalf("%s page %d: reference %v, PageDict's %v", name, p, g.Ref, ref)
		}
		if !reflect.DeepEqual(attrs, g.Attrs) {
			t.Fatalf("%s page %d: inherited attributes %+v, PageDict's %+v", name, p, g.Attrs, attrs)
		}
		if attrs != nil && g.Attrs != nil && attrs.MediaBox != nil && attrs.MediaBox == g.Attrs.MediaBox {
			t.Fatalf("%s page %d: a rectangle is shared with PageDict's answer", name, p)
		}
	}
}

func page(parent int, extra string) string {
	return fmt.Sprintf("<< /Type /Page /Parent %d 0 R %s >>", parent, extra)
}

// treeShapes are page trees built by hand. fast says whether the one walk may answer (true) or must hand the
// document to PageDict (false) — each false is a shape where one walk cannot reproduce pdfcpu.
func treeShapes() map[string]struct {
	objs map[int]string
	fast bool
} {
	res := "/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >>"
	return map[string]struct {
		objs map[int]string
		fast bool
	}{
		// Inherited /MediaBox, /Rotate and /Resources at the root; one page overrides each.
		"flat, inheriting": {map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 /MediaBox [0 0 612 792] /Rotate 90 " + res + " >>",
			3: page(2, ""),
			4: page(2, "/MediaBox [0 0 100 200] /Rotate 180 /CropBox [10 10 90 190] /Resources << >>"),
			5: page(2, ""),
		}, true},
		// Attributes set at an intermediate node reach its subtree and not its siblings.
		"nested, attributes per subtree": {map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R 6 0 R 7 0 R] /Count 5 /MediaBox [0 0 612 792] >>",
			3:  "<< /Type /Pages /Parent 2 0 R /Kids [4 0 R 5 0 R] /Count 2 /MediaBox [0 0 300 300] /Rotate 270 >>",
			4:  page(3, ""),
			5:  page(3, "/Rotate 0"),
			6:  page(2, ""),
			7:  "<< /Type /Pages /Parent 2 0 R /Kids [8 0 R 9 0 R] /Count 2 /CropBox [1 2 3 4] >>",
			8:  "<< /Type /Pages /Parent 7 0 R /Kids [10 0 R] /Count 1 " + res + " >>",
			9:  page(7, ""),
			10: page(8, ""),
		}, true},
		// An empty intermediate node holds nothing and shifts nothing.
		"empty intermediate node": {map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 2 >>",
			3: page(2, "/MediaBox [0 0 1 1]"),
			4: "<< /Type /Pages /Parent 2 0 R /Kids [] /Count 0 /MediaBox [0 0 9 9] >>",
			5: page(2, "/MediaBox [0 0 2 2]"),
		}, true},
		// pdfcpu ENTERS a subtree with no /Count, even one not holding the page asked for, and keeps what it set.
		"a subtree without a count": {map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
			3: "<< /Type /Pages /Parent 2 0 R /Kids [4 0 R] /Rotate 90 /MediaBox [0 0 5 5] >>",
			4: page(3, ""),
			5: page(2, ""),
		}, false},
		// A subtree whose count understates it: pdfcpu skips by the count.
		"a subtree whose count is wrong": {map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 3 /MediaBox [0 0 612 792] >>",
			3: "<< /Type /Pages /Parent 2 0 R /Kids [4 0 R 5 0 R] /Count 1 >>",
			4: page(3, ""),
			5: page(3, "/Rotate 90"),
			6: page(2, ""),
		}, false},
		// A /Page carrying /Kids: pdfcpu counts it and descends past it.
		"a page with kids": {map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
			3: "<< /Type /Page /Parent 2 0 R /Kids [4 0 R] >>",
			4: page(3, ""),
			5: page(2, ""),
		}, false},
	}
}

func TestPagesAnswersWhatPageDictAnswersOnEveryTreeShape(t *testing.T) {
	for name, c := range treeShapes() {
		ctx, err := pdfread.Validated(testpdf.Assemble(c.objs), model.NewDefaultConfiguration())
		if err != nil {
			// A shape validation refuses cannot reach any caller; say so rather than pass it silently.
			if c.fast {
				t.Fatalf("%s: a well-formed tree was refused: %v", name, err)
			}
			t.Logf("%s: refused by validation (%v) — unreachable, nothing to compare", name, err)
			continue
		}
		if got := pdfread.WalkedInOnePass(ctx); got != c.fast {
			t.Errorf("%s: answered in one pass = %v, want %v", name, got, c.fast)
		}
		samePages(t, name, ctx)
	}
}

// Every real document within reach: the producer corpus and veraPDF's. Skipped where neither is installed.
func TestPagesAnswersWhatPageDictAnswersOnTheCorpora(t *testing.T) {
	home, _ := os.UserHomeDir()
	var files []string
	for _, root := range []string{filepath.Join(home, "nib", "producers"), filepath.Join(home, "nib", "verapdfs")} {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Skip("no corpus under ~/nib/producers or ~/nib/verapdfs")
	}
	read, fast := 0, 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		ctx, err := pdfread.Validated(b, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		read++
		if pdfread.WalkedInOnePass(ctx) {
			fast++
		}
		samePages(t, f, ctx)
	}
	t.Logf("%d of %d documents read; %d answered in one pass", read, len(files), fast)
	if read > 0 && fast == 0 {
		t.Fatalf("no corpus document took the one walk — the fast path is dead")
	}
}

// flatDoc is n pages under one /Pages node — the shape nib's own Markdown conversion writes.
func flatDoc(n int) []byte {
	objs := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"}
	var kids strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", 3+i)
		objs[3+i] = page(2, "")
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>", kids.String(), n)
	return testpdf.Assemble(objs)
}

// The cost: doubling the pages must not much more than double the time. On `PageDict` per page it quadruples.
func TestPagesIsLinearInThePageCount(t *testing.T) {
	best := func(n int) time.Duration {
		ctx, err := pdfread.Validated(flatDoc(n), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		b := time.Duration(1 << 62)
		for i := 0; i < 5; i++ {
			runtime.GC()
			t0 := time.Now()
			if got := len(pdfread.Pages(ctx)); got != n {
				t.Fatalf("%d pages answered for %d", got, n)
			}
			b = min(b, time.Since(t0))
		}
		return b
	}
	// 4× the pages: linear is ×4, the per-page root walk ×16; ×8 is the midpoint on a log scale. (It was 2×
	// against ×3 — alone ×2.1, but millisecond timings tipped ×3.66 once under a loaded full suite.)
	small, large := best(3000), best(12000)
	t.Logf("3,000 pages %v, 12,000 pages %v (×%.2f)", small, large, float64(large)/float64(small))
	if large > 8*small {
		t.Fatalf("4x the pages cost ×%.2f (%v → %v): the walk is not linear", float64(large)/float64(small), small, large)
	}
}
