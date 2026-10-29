package pdfread_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// dupDoc is a one-page document whose page names fonts F1 and F2 (objects 8 and 9) and, when img is set, images I1
// and I2 (objects 6 and 7); more holds everything they name.
func dupDoc(f1, f2 string, img [2]string, more map[int]string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		8: f1, 9: f2,
	}
	xobj := ""
	if img[0] != "" {
		objs[6], objs[7] = img[0], img[1]
		xobj = " /XObject << /I1 6 0 R /I2 7 0 R >>"
	}
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources << /Font << /F1 8 0 R /F2 9 0 R >>" + xobj + " >> >>"
	for nr, body := range more {
		objs[nr] = body
	}
	return testpdf.Assemble(objs)
}

// passRuns reads pdf through ReadOptimized and reports how many times pdfcpu's pass was started.
func passRuns(t *testing.T, pdf []byte) (runs int, err error) {
	t.Helper()
	was := *pdfread.FreeReferencePass
	*pdfread.FreeReferencePass = func(ctx *model.Context) { runs++; was(ctx) }
	defer func() { *pdfread.FreeReferencePass = was }()
	_, err = pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	return runs, err
}

const (
	dupHelv  = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica %s >>"
	dupGlyph = "<< /Length 13 >>\nstream\n10 0 d0 0 0 m\nendstream"
	dupType3 = "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.001 0 0 0.001 0 0] /CharProcs << /a 30 0 R >> " +
		"/Encoding << /Type /Encoding /Differences [97 /a] >> /FirstChar 97 /LastChar 97 /Widths [%d] " +
		"/Resources << /Font << /Self %d 0 R >> >> >>"
	dupImage = "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 %s /Length 1 >>\nstream\n\x00\nendstream"
)

// TestALoopUnderAFontOrImagePdfcpuCouldMergeSkipsThePass — /pending 867. For every font or image pdfcpu's optimize
// pass finds equal to another it follows everything the duplicate names, remembering nothing (`traverse`,
// optimize.go:1019, v0.13.0; v0.16.2 depth-bounds it, optimize.go:1167), so a loop under a key the validator never reads was a fatal stack overflow — measured in
// a throwaway process on the first shape here. The pass is not run over such a document: a reader that only wants the
// document still gets it, and one whose answer depends on the pass is refused. **Each shape is asked of `Unaffordable`
// before the pass is ever started**, so a rule that stops refusing fails here as an assertion and not as a dead process.
func TestALoopUnderAFontOrImagePdfcpuCouldMergeSkipsThePass(t *testing.T) {
	f := func(extra string) string { return fmt.Sprintf(dupHelv, extra) }
	for _, c := range []struct {
		name string
		pdf  []byte
	}{
		{"two equal fonts over one object that names itself",
			dupDoc(f("/Foo 10 0 R"), f("/Foo 10 0 R"), [2]string{}, map[int]string{10: "[10 0 R]"})},
		{"two equal fonts, each over a ring of its own",
			dupDoc(f("/Foo 10 0 R"), f("/Foo 20 0 R"), [2]string{}, map[int]string{
				10: "[11 0 R]", 11: "[12 0 R]", 12: "[10 0 R]", 20: "[21 0 R]", 21: "[22 0 R]", 22: "[20 0 R]"})},
		{"two equal images over a loop",
			dupDoc(f(""), f("/Bar 1"), [2]string{fmt.Sprintf(dupImage, "/Foo 10 0 R"), fmt.Sprintf(dupImage, "/Foo 10 0 R")},
				map[int]string{10: "<< /Back 10 0 R >>"})},
		// pdfcpu's comparison calls a null on its left equal to anything (model/equal.go:81-83).
		{"a font over a loop beside one that holds a null where it differs",
			dupDoc(f("/Foo 10 0 R /Bar /X"), f("/Foo 10 0 R /Bar 40 0 R"), [2]string{}, map[int]string{10: "[10 0 R]", 40: "null"})},
		{"a font over a loop beside one that holds a null where the loop is",
			dupDoc(f("/Foo 10 0 R"), f("/Foo 40 0 R"), [2]string{}, map[int]string{10: "[10 0 R]", 40: "null"})},
		// The walk finishes the loop under font 5, which is nobody's duplicate, before it reaches fonts 8 and 9.
		{"two equal fonts over a loop a third font reached first",
			dupDoc(f("/Foo 10 0 R"), f("/Foo 10 0 R"), [2]string{}, map[int]string{5: f("/Foo 10 0 R /Bar /Z"), 10: "[[10 0 R]]"})},
		// pdfcpu compares two fonts' names without their subset prefixes (model/equal.go:153-181).
		{"two fonts over a loop, equal but for a subset prefix",
			dupDoc(strings.Replace(f("/Foo 10 0 R"), "/Helvetica", "/ABCDEF+Helvetica", 1),
				strings.Replace(f("/Foo 10 0 R"), "/Helvetica", "/GHIJKL+Helvetica", 1), [2]string{}, map[int]string{10: "[10 0 R]"})},
		{"two equal Type 3 fonts that each name themselves",
			dupDoc(fmt.Sprintf(dupType3, 10, 8), fmt.Sprintf(dupType3, 10, 9), [2]string{}, map[int]string{30: dupGlyph})},
	} {
		ctx, err := pdfread.Validated(c.pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Errorf("setup: %s does not validate: %v", c.name, err)
			continue
		}
		if why := pdfread.Unaffordable(ctx); !strings.Contains(why, "lead back to themselves") {
			t.Errorf("%s: Unaffordable = %q, want the loop named — pdfcpu's pass would follow it until the stack ran out", c.name, why)
			continue // never hand it to the pass
		}
		if runs, err := passRuns(t, c.pdf); err != nil || runs != 0 {
			t.Errorf("%s: read with the pass skipped: %d runs, %v; want the document and no pass", c.name, runs, err)
		}
		if _, err := pdfread.ReadOptimizedOrRefuse(c.pdf, model.NewDefaultConfiguration()); !errors.Is(err, pdfread.ErrUnaffordable) {
			t.Errorf("%s: a reader that depends on the pass got %v, want ErrUnaffordable", c.name, err)
		}
	}
}

// TestAFontThatNamesItselfIsStillOptimized is the other half of /pending 867: a loop under a font is a real document
// — a Type 3 font's `/Resources /Font` naming the font — and the pass is skipped only where pdfcpu could take the font
// for another's duplicate. Each of these runs pdfcpu's pass, which is also the evidence that the pass follows nothing
// under a font or image it merged into no other.
func TestAFontThatNamesItselfIsStillOptimized(t *testing.T) {
	f := func(extra string) string { return fmt.Sprintf(dupHelv, extra) }
	for _, c := range []struct {
		name string
		pdf  []byte
	}{
		{"a Type 3 font that names itself, beside an ordinary font",
			dupDoc(fmt.Sprintf(dupType3, 10, 8), f(""), [2]string{}, map[int]string{30: dupGlyph})},
		{"two Type 3 fonts that name themselves and differ in a width",
			dupDoc(fmt.Sprintf(dupType3, 10, 8), fmt.Sprintf(dupType3, 11, 9), [2]string{}, map[int]string{30: dupGlyph})},
		{"two fonts over one loop that differ in a name",
			dupDoc(f("/Foo 10 0 R /Bar /X"), f("/Foo 10 0 R /Bar /Y"), [2]string{}, map[int]string{10: "[10 0 R]"})},
		{"two equal fonts and two equal images, no loop",
			dupDoc(f("/Foo 10 0 R"), f("/Foo 10 0 R"), [2]string{fmt.Sprintf(dupImage, ""), fmt.Sprintf(dupImage, "")},
				map[int]string{10: "[]"})},
		{"an image over a loop, beside one with other bytes",
			dupDoc(f(""), f("/Bar 1"), [2]string{fmt.Sprintf(dupImage, "/Foo 10 0 R"),
				strings.Replace(fmt.Sprintf(dupImage, "/Foo 10 0 R"), "\x00", "\x01", 1)}, map[int]string{10: "[10 0 R]"})},
	} {
		ctx, err := pdfread.Validated(c.pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Errorf("setup: %s does not validate: %v", c.name, err)
			continue
		}
		if why := pdfread.Unaffordable(ctx); why != "" {
			t.Errorf("%s: Unaffordable = %q, want the pass run", c.name, why)
			continue
		}
		if runs, err := passRuns(t, c.pdf); err != nil || runs != 1 {
			t.Errorf("%s: %d runs of the pass, %v; want one", c.name, runs, err)
		}
	}
}

// TestAnXObjectWhoseSubtypeIsAReferenceSkipsThePass — /pending 867's second note. pdfcpu's relaxed validator reads
// `/Subtype 20 0 R` through the reference; its optimize pass asks the dictionary for a name and dereferences the nil it
// gets (optimize.go:697, :801, v0.13.0; v0.16.2's optimize.go has no `*sd.Subtype()` — grepped, not re-run), a runtime panic. Found on the two SHA-1 collision PDFs in the Go module cache.
func TestAnXObjectWhoseSubtypeIsAReferenceSkipsThePass(t *testing.T) {
	f := fmt.Sprintf(dupHelv, "")
	indirect := strings.Replace(fmt.Sprintf(dupImage, ""), "/Subtype /Image", "/Subtype 20 0 R", 1)
	pdf := dupDoc(f, f, [2]string{indirect, fmt.Sprintf(dupImage, "")}, map[int]string{20: "/Image"})
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: an image whose /Subtype is a reference does not validate: %v", err)
	}
	if why := pdfread.Unaffordable(ctx); !strings.Contains(why, "/Subtype of its XObject in object 6") {
		t.Fatalf("Unaffordable = %q, want the XObject named — pdfcpu's pass would panic on it", why)
	}
	if runs, err := passRuns(t, pdf); err != nil || runs != 0 {
		t.Fatalf("read with the pass skipped: %d runs, %v; want the document and no pass", runs, err)
	}
	if _, err := pdfread.ReadOptimizedOrRefuse(pdf, model.NewDefaultConfiguration()); !errors.Is(err, pdfread.ErrUnaffordable) {
		t.Fatalf("a reader that depends on the pass got %v, want ErrUnaffordable", err)
	}
	named := dupDoc(f, f, [2]string{fmt.Sprintf(dupImage, ""), fmt.Sprintf(dupImage, "")}, nil)
	if runs, err := passRuns(t, named); err != nil || runs != 1 {
		t.Fatalf("control: the same document with the name written in place: %d runs, %v; want one", runs, err)
	}
}

// TestASkippedPassIsRememberedAgainstItsContext — /pending 766 part 7. `optimize` returned nil for a pass it ran
// and for one it skipped, so nothing downstream of a write-path read could say the output was not compacted. The
// skip is now a fact kept against the context.
func TestASkippedPassIsRememberedAgainstItsContext(t *testing.T) {
	skipped, err := pdfread.ReadOptimized(testpdf.PassSkipped(), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: the document does not read: %v", err)
	}
	// STIMULUS: this is a document the pass is skipped for, and the read asked for the pass.
	if pdfread.Unaffordable(skipped) == "" {
		t.Fatal("setup: the fixture's pass is affordable; nothing below is about a skip")
	}
	if !pdfread.OptimizeSkipped(skipped) {
		t.Error("the pass was skipped and the context does not say so")
	}
	f := fmt.Sprintf(dupHelv, "")
	ran, err := pdfread.ReadOptimized(dupDoc(f, f, [2]string{}, nil), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if pdfread.OptimizeSkipped(ran) {
		t.Error("a document whose pass ran says it was skipped")
	}
	// Asked for and refused is not a skip: the strict door returns no context at all.
	if _, err := pdfread.ReadOptimizedOrRefuse(testpdf.PassSkipped(), model.NewDefaultConfiguration()); !errors.Is(err, pdfread.ErrUnaffordable) {
		t.Errorf("the strict read got %v, want ErrUnaffordable", err)
	}
	// And a context nobody asked to optimize says nothing.
	conf := model.NewDefaultConfiguration()
	conf.Optimize = false
	unasked, err := pdfread.ReadOptimized(testpdf.PassSkipped(), conf)
	if err != nil {
		t.Fatal(err)
	}
	if pdfread.OptimizeSkipped(unasked) {
		t.Error("a read with the pass turned off says the pass was skipped")
	}
}
