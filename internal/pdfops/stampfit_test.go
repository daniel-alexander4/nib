package pdfops

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"nib/internal/testpdf"
	"nib/mdpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// emittedBBoxWidth is the width of the form XObject pdfcpu actually wrote for a
// stamped field — the number that decides where the glyphs land on the page.
//
// This is the oracle stampWidth is held to, and it is deliberately NOT the AFM
// table. A test that measures with the same function the code measures with is
// circular and cannot tell a correct width rule from a wrong one — `internal/p2p`'s
// column guard says so about itself in as many words. Reading the emitted BBox
// escapes that: it is pdfcpu's own answer, produced by the write path, and it goes
// red if pdfcpu ever changes how it measures.
func emittedBBoxWidth(t *testing.T, pdf []byte, f Field) float64 {
	t.Helper()
	out, _, err := StampFields(pdf, []Field{f})
	if err != nil {
		t.Fatalf("StampFields(%q): %v", f.Text, err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read stamped PDF: %v", err)
	}
	d, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatalf("PageDict: %v", err)
	}
	res, err := ctx.DereferenceDict(d["Resources"])
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}
	xo, err := ctx.DereferenceDict(res["XObject"])
	if err != nil {
		t.Fatalf("XObject: %v", err)
	}
	for _, ref := range xo {
		sd, _, err := ctx.DereferenceStreamDict(ref)
		if err != nil || sd == nil || sd.Dict["BBox"] == nil {
			continue
		}
		arr, err := ctx.DereferenceArray(sd.Dict["BBox"])
		if err != nil || len(arr) < 3 {
			continue
		}
		if v, ok := arr[2].(interface{ Value() float64 }); ok {
			return v.Value()
		}
	}
	t.Fatalf("no form XObject with a BBox found for %q — nothing was measured", f.Text)
	return 0
}

// trapStrings are the strings that separate a correct width rule from the three
// wrong ones that look right. Each was measured on 2026-09-09; the comments give
// the wrong answers so a future reader can tell WHICH rule broke from the number.
var trapStrings = []struct {
	name, text string
	why        string
}{
	{"ascii", "Plain ASCII", "the baseline — every candidate rule agrees here"},
	{"percent", "50% of the time", "stampText doubles %; measuring the escaped form reads 94.04 against 83.38"},
	{"newline", "one\ntwo", "CoreWidth counts \\n as a character: 50.69 against an emitted 20.02"},
	{"newline-second-wider", "a\nlonger second line", "the widest line is the second: 116.05 against 97.38"},
	{"multibyte", "café naïve", "encodedWidth's byte-vs-rune rule; a rune-wise count under-reads"},
	// pdfcpu breaks on "\n" ONLY: a lone "\r" is an ordinary glyph and a "\r\n" leaves the
	// "\r" on the end of the first line. Measured 2026-09-09 — 20.02 / 32.02 / 50.69 for
	// "one\ntwo", "one\r\ntwo", "one\rtwo" — so splitting on "\n" alone is not an
	// approximation of pdfcpu's rule, it IS pdfcpu's rule. A tidier-looking split on all
	// line endings would be wrong on both of these.
	{"crlf", "one\r\ntwo", "the \\r stays on line one; a split on all line endings under-reads"},
	{"bare-cr", "one\rtwo", "no break at all — a split on \\r would read 20.02 against an emitted 50.69"},
}

// C1 — the width StampFields REPORTS is the width pdfcpu emits, across the traps.
//
// The assertion is on Fit.WidthPt and not on stampWidth, and the difference is not
// cosmetic: an earlier cut of this test called the helper directly, and a mutation
// that made fitFor measure stampText's escaped output instead of the raw text
// stayed GREEN through the whole suite. The door was proven correct while the
// production path that calls it was not — every other test that reached fitFor
// happened to use %-free text. Driving the traps through StampFields is what makes
// this a claim about what ships.
func TestStampWidthMatchesEmittedBBox(t *testing.T) {
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	const eps = 0.01
	n := 0
	for _, tc := range trapStrings {
		t.Run(tc.name, func(t *testing.T) {
			f := Field{Page: 1, Rect: [4]float64{50, 400, 150, 420}, Text: tc.text, Font: "Helvetica", Size: 12}
			_, fits, err := StampFields(pdf, []Field{f})
			if err != nil {
				t.Fatalf("StampFields: %v", err)
			}
			if len(fits) != 1 {
				t.Fatalf("got %d fits, want 1 — nothing was measured", len(fits))
			}
			want := emittedBBoxWidth(t, pdf, f)
			if got := fits[0].WidthPt; math.Abs(got-want) > eps {
				t.Errorf("StampFields reported %.2fpt, pdfcpu emitted a %.2fpt box (%s)", got, want, tc.why)
			}
			// The door itself, held to the same oracle: a divergence here and a
			// clean Fit above would mean fitFor is not using it.
			fontName, pts := stampStyle(f)
			if got := stampWidth(tc.text, fontName, pts); math.Abs(got-want) > eps {
				t.Errorf("stampWidth = %.2fpt, pdfcpu emitted a %.2fpt box (%s)", got, want, tc.why)
			}
			n++
		})
	}
	if n != len(trapStrings) {
		t.Fatalf("measured %d of %d trap strings — the rest asserted nothing", n, len(trapStrings))
	}
}

// The stimulus floor for C1: the traps must actually DISCRIMINATE. If the three
// wrong rules all agree with the right one on this corpus, the corpus is decoration
// and C1 would pass against a naive implementation.
//
// This is the assertion that would have caught the plan's own prescription.
func TestTrapStringsSeparateTheWrongRulesFromTheRight(t *testing.T) {
	naive := func(s string) float64 { return mdpdf.CoreWidth(s, "Helvetica", 12) }
	escaped := func(s string) float64 { return mdpdf.CoreWidth(strings.ReplaceAll(s, "%", "%%"), "Helvetica", 12) }
	naiveCaught, escapedCaught := 0, 0
	for _, tc := range trapStrings {
		right := stampWidth(tc.text, "Helvetica", 12)
		if math.Abs(naive(tc.text)-right) > 0.01 {
			naiveCaught++
		}
		if math.Abs(escaped(tc.text)-right) > 0.01 {
			escapedCaught++
		}
	}
	if naiveCaught == 0 {
		t.Error("no trap string distinguishes a bare CoreWidth from stampWidth — the newline traps are inert")
	}
	if escapedCaught == 0 {
		t.Error("no trap string distinguishes measuring stampText's output — the % trap is inert")
	}
	t.Logf("traps: %d/%d catch a bare CoreWidth, %d/%d catch measuring the escaped form",
		naiveCaught, len(trapStrings), escapedCaught, len(trapStrings))
}

// C2 — an arbitrary font name neither panics nor measures the wrong face.
//
// Field.Font is whatever pdf.js read out of the document, so unlisted names are
// the ordinary case, not the edge one. mdpdf.CoreWidth PANICS on a name pdfcpu's
// metrics table does not carry ("pdfcpu: user font not loaded: Arial"), which
// would put a panic on /api/bake — StampFields is called from handleBake.
func TestStampWidthMeasuresTheFaceThatIsActuallyStamped(t *testing.T) {
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	const text = "Plain ASCII"
	helvetica := stampWidth(text, "Helvetica", 12)
	for _, name := range []string{"Arial", "", "NotAFont", "helvetica", "Comic Sans MS"} {
		f := Field{Page: 1, Rect: [4]float64{50, 400, 150, 420}, Text: text, Font: name, Size: 12}
		fontName, pts := stampStyle(f)
		if fontName != "Helvetica" {
			t.Errorf("stampStyle(%q) = %q, want the Helvetica the stamp will actually use", name, fontName)
		}
		// Does not panic, and agrees with the face pdfcpu emits.
		got := stampWidth(f.Text, fontName, pts)
		if math.Abs(got-helvetica) > 0.01 {
			t.Errorf("font %q measured %.2fpt, want the stamped face's %.2fpt", name, got, helvetica)
		}
		if want := emittedBBoxWidth(t, pdf, f); math.Abs(got-want) > 0.01 {
			t.Errorf("font %q: measured %.2fpt, pdfcpu emitted %.2fpt", name, got, want)
		}
	}
	// A non-core name really does panic one level down — so the coercion above is
	// load-bearing, not belt-and-braces. Without this, the test cannot tell a
	// working guard from an unnecessary one.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("mdpdf.CoreWidth no longer panics on an unlisted font — " +
					"stampStyle's coercion may no longer be what keeps /api/bake up")
			}
		}()
		_ = mdpdf.CoreWidth(text, "Arial", 12)
	}()
}

// emittedAnchorX is the x the stamped glyphs are actually translated to on the
// page — the "cm" that precedes the form XObject's Do operator.
//
// The fit subtracts stampInsetPt from the box because the stamp description ADDS
// it to the rectangle. Reading the emitted anchor is what holds those two uses of
// one constant together; a mutation that dropped the inset from the description
// alone passed every other assertion in this file.
func emittedAnchorX(t *testing.T, pdf []byte, f Field) float64 {
	t.Helper()
	out, _, err := StampFields(pdf, []Field{f})
	if err != nil {
		t.Fatalf("StampFields: %v", err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	d, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatalf("PageDict: %v", err)
	}
	c, err := ctx.PageContent(d, 1)
	if err != nil {
		t.Fatalf("PageContent: %v", err)
	}
	m := anchorCmRE.FindSubmatch(c)
	if m == nil {
		t.Fatalf("no \"... cm ... Do\" found in the page stream — nothing was located:\n%s", c)
	}
	var x float64
	if _, err := fmt.Sscanf(string(m[1]), "%g", &x); err != nil {
		t.Fatalf("unparseable anchor %q: %v", m[1], err)
	}
	return x
}

// The translate immediately before the watermark form's Do.
var anchorCmRE = regexp.MustCompile(`([0-9.]+) [0-9.]+ cm[^D]*/Fm[0-9]+ Do`)

// C3 — the fit is measured against the same inset the stamp anchors at.
//
// Driven so that 2pt is the whole verdict: the text is sized to fall between the
// raw rectangle width and the inset width, so a fit computed against the wrong one
// reports the opposite answer.
func TestFitAccountsForTheAnchorInset(t *testing.T) {
	const text = "Plain ASCII"
	w := stampWidth(text, "Helvetica", 12) // 61.36pt
	// Box chosen so that: w <= rectWidth, and w > rectWidth - stampInsetPt.
	rectW := w + stampInsetPt/2
	f := Field{Page: 1, Rect: [4]float64{50, 400, 50 + rectW, 420}, Text: text, Font: "Helvetica", Size: 12}
	fit := fitFor(0, f, 1)

	if fit.BoxPt >= rectW {
		t.Fatalf("BoxPt %.2f did not subtract the %.0fpt inset from a %.2fpt rectangle",
			fit.BoxPt, stampInsetPt, rectW)
	}
	if fit.OverrunPt <= 0 {
		t.Errorf("overrun %.2fpt: text of %.2fpt in a %.2fpt rectangle anchored %.0fpt in "+
			"DOES overrun; a fit measured against the raw rectangle would call this a fit",
			fit.OverrunPt, w, rectW, stampInsetPt)
	}
	// The two USES of stampInsetPt agree: the fit's box starts where the glyphs do.
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	gotX := emittedAnchorX(t, pdf, f)
	wantX := f.Rect[0] + stampInsetPt
	if math.Abs(gotX-wantX) > 0.05 {
		t.Errorf("glyphs anchored at x=%.2f but the fit measures its box from x=%.2f — "+
			"the stamp description and fitFor disagree about the inset", gotX, wantX)
	}
	if math.Abs((f.Rect[2]-gotX)-fit.BoxPt) > 0.05 {
		t.Errorf("usable width from the emitted anchor is %.2fpt, fit reported %.2fpt",
			f.Rect[2]-gotX, fit.BoxPt)
	}

	// And the ordinary case stays negative and distinguishable from "not measured".
	roomy := Field{Page: 1, Rect: [4]float64{50, 400, 50 + w + 40, 420}, Text: text, Font: "Helvetica", Size: 12}
	if rf := fitFor(0, roomy, 1); rf.OverrunPt >= 0 {
		t.Errorf("a comfortably-fitting field reported overrun %.2fpt, want negative", rf.OverrunPt)
	}
}

// The shipped defect itself, pinned: a replacement far wider than its box is
// stamped without error, and now it is MEASURED.
func TestStampFieldsReportsTheShippedOverrun(t *testing.T) {
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	long := "This replacement string is very considerably wider than one hundred points"
	fields := []Field{
		{Page: 1, Rect: [4]float64{50, 400, 150, 420}, Text: long, Font: "Helvetica", Size: 12},
		{Page: 1, Rect: [4]float64{50, 300, 150, 320}, Text: "   "}, // skipped: empty
		{Page: 1, Rect: [4]float64{50, 200, 400, 220}, Text: "short", Font: "Helvetica", Size: 12},
	}
	_, fits, err := StampFields(pdf, fields)
	if err != nil {
		t.Fatal(err)
	}
	if len(fits) != 2 {
		t.Fatalf("got %d fits, want 2 (the empty field is skipped and owes none)", len(fits))
	}
	// Fits key on the input index, not on position — the empty field shifted them.
	if fits[0].Field != 0 || fits[1].Field != 2 {
		t.Errorf("fits carry input indices %d and %d, want 0 and 2", fits[0].Field, fits[1].Field)
	}
	if fits[0].OverrunPt < 250 {
		t.Errorf("the overrunning field reported %.2fpt of overrun, want the measured ~298pt",
			fits[0].OverrunPt)
	}
	if fits[1].OverrunPt >= 0 {
		t.Errorf("the fitting field reported %.2fpt, want negative", fits[1].OverrunPt)
	}
}

// C4 — one width door. Nothing outside mdpdf calls pdfcpu's metrics directly.
//
// The rule is ADR-009's and mdpdf.CoreWidth's doc comment states it: calling
// font.TextWidth with raw UTF-8 is the bug the door exists to prevent, and
// internal/p2p shipped exactly that defect once already.
//
// What this cannot see: a second implementation that reimplements the arithmetic
// from its own width table rather than calling pdfcpu's. That would need a
// different scan and no instance exists today.
func TestWidthMeasurementHasOneDoor(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	scanned, offenders := 0, []string{}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "dist", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, "mdpdf/") { // the door itself
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return nil // not our business to police unparseable files
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "font" {
				return true
			}
			if sel.Sel.Name == "TextWidth" || sel.Sel.Name == "CharWidth" {
				offenders = append(offenders,
					fmt.Sprintf("%s:%d: font.%s — measure through mdpdf.CoreWidth (ADR-009)",
						rel, fset.Position(call.Pos()).Line, sel.Sel.Name))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 50 {
		t.Fatalf("scanned only %d Go files outside mdpdf — the walk did not reach the tree, "+
			"so a clean result means nothing", scanned)
	}
	for _, o := range offenders {
		t.Error(o)
	}
	t.Logf("scanned %d Go files outside mdpdf/", scanned)
}

// stampStyle's SIZE half, held to the emitted Tf operand.
//
// The width tests all use Size: 12, which passes through every clamp branch
// unchanged — so deleting the clamps entirely left them green. The clamp decides
// what point size is stamped, and stampWidth is called with that size, so a drift
// here measures a size the page does not carry.
func TestStampStyleSizeMatchesTheEmittedTf(t *testing.T) {
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		size   float64
		y0, y1 float64
		want   int
	}{
		{"explicit", 12, 400, 420, 12},
		{"explicit below the floor", 3, 400, 420, 8},
		{"explicit above the cap", 500, 400, 420, 144},
		{"derived from a tall box", 0, 400, 500, 48},
		{"derived from a short box", 0, 400, 412, 8},
		{"derived, ordinary", 0, 400, 425, 18},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := Field{Page: 1, Rect: [4]float64{50, tc.y0, 300, tc.y1}, Text: "Plain ASCII", Font: "Helvetica", Size: tc.size}
			_, pts := stampStyle(f)
			if pts != tc.want {
				t.Errorf("stampStyle size = %d, want %d", pts, tc.want)
			}
			if got := emittedTfSize(t, pdf, f); math.Abs(got-float64(pts)) > 0.01 {
				t.Errorf("stampStyle said %dpt, pdfcpu emitted %.2fpt — the measurement "+
					"would be taken at a size the page does not carry", pts, got)
			}
		})
	}
}

// emittedTfSize is the point size in the stamped form XObject's Tf operator.
func emittedTfSize(t *testing.T, pdf []byte, f Field) float64 {
	t.Helper()
	out, _, err := StampFields(pdf, []Field{f})
	if err != nil {
		t.Fatalf("StampFields: %v", err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	d, _, _, _ := ctx.PageDict(1, false)
	res, _ := ctx.DereferenceDict(d["Resources"])
	xo, _ := ctx.DereferenceDict(res["XObject"])
	for _, ref := range xo {
		sd, _, err := ctx.DereferenceStreamDict(ref)
		if err != nil || sd == nil || sd.Dict["BBox"] == nil {
			continue
		}
		if err := sd.Decode(); err != nil {
			continue
		}
		if m := tfRE.FindSubmatch(sd.Content); m != nil {
			var v float64
			if _, err := fmt.Sscanf(string(m[1]), "%g", &v); err == nil {
				return v
			}
		}
	}
	t.Fatalf("no Tf operator found in the stamped form — nothing was measured")
	return 0
}

var tfRE = regexp.MustCompile(`/F[0-9]+ ([0-9.]+) Tf`)
