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
	// Box chosen so 2pt is the WHOLE verdict: the text fits the raw rectangle and
	// does not fit it once the anchor inset is taken off. So a fit measured against
	// the wrong one does not merely report a different number — it reaches a
	// different OUTCOME, "as-drawn" instead of "shrunk", and stamps a different size.
	rectW := w + stampInsetPt/2
	f := Field{Page: 1, Rect: [4]float64{50, 400, 50 + rectW, 420}, Text: text, Font: "Helvetica", Size: 12}

	got, outcome, _, boxPt := resolveFit(f)
	if boxPt >= rectW {
		t.Fatalf("box %.2f did not subtract the %.0fpt inset from a %.2fpt rectangle",
			boxPt, stampInsetPt, rectW)
	}
	if w <= boxPt || w > rectW {
		t.Fatalf("fixture is inert: %.2fpt text must fit the %.2fpt rectangle and not the "+
			"%.2fpt box, or this proves nothing", w, rectW, boxPt)
	}
	if outcome != FitShrunk {
		t.Errorf("outcome %q: text of %.2fpt in a %.2fpt rectangle anchored %.0fpt in must "+
			"shrink; measured against the raw rectangle it would report %q",
			outcome, w, rectW, stampInsetPt, FitAsDrawn)
	}
	if got.Size >= f.Size {
		t.Errorf("shrunk to %.0fpt from %.0fpt — nothing was reduced", got.Size, f.Size)
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
			"the stamp description and resolveFit disagree about the inset", gotX, wantX)
	}
	if math.Abs((f.Rect[2]-gotX)-boxPt) > 0.05 {
		t.Errorf("usable width from the emitted anchor is %.2fpt, fit reported %.2fpt",
			f.Rect[2]-gotX, boxPt)
	}

	// And the ordinary case stays negative and distinguishable from "not measured".
	roomy := Field{Page: 1, Rect: [4]float64{50, 400, 50 + w + 40, 420}, Text: text, Font: "Helvetica", Size: 12}
	ro, rw, rb := func() (FitOutcome, float64, float64) { _, o, w, b := resolveFit(roomy); return o, w, b }()
	if rf := fitFor(0, roomy, 1, ro, rw, rb); rf.OverrunPt >= 0 || rf.Outcome != FitAsDrawn {
		t.Errorf("a comfortably-fitting field reported overrun %.2fpt outcome %q, want negative and %q",
			rf.OverrunPt, rf.Outcome, FitAsDrawn)
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
			// A WIDE box and a one-glyph string, deliberately: this test is about the
			// size clamp, and once resolveFit's ladder exists an overrunning fixture
			// would shrink and measure the shrink instead. Fit.StampedPt is what
			// predicts the emitted size in general; stampStyle does so only here,
			// where nothing overruns. TestShrinkIsWhatTheTfReports covers the other side.
			f := Field{Page: 1, Rect: [4]float64{50, tc.y0, 560, tc.y1}, Text: "x", Font: "Helvetica", Size: tc.size}
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

// C1 — each outcome is reachable, and each fixture reaches ONLY its own.
//
// The last clause is the one that matters. A table where every row happens to be
// satisfiable by the same branch proves the branch runs, not that the ladder
// chooses; so each row asserts the outcome it wants AND that it is not one of the
// others, and the table asserts all four values are covered.
func TestEachFitOutcomeIsReachableAndDistinct(t *testing.T) {
	const line = "Plain ASCII"
	w12 := stampWidth(line, "Helvetica", 12) // 61.36pt

	cases := []struct {
		name string
		f    Field
		want FitOutcome
		why  string
	}{
		{
			"as-drawn", Field{Page: 1, Rect: [4]float64{50, 400, 50 + w12 + 40, 420}, Text: line, Font: "Helvetica", Size: 12},
			FitAsDrawn, "40pt of slack: nothing to do",
		},
		{
			// Needs ~11pt. Reachable well above the floor.
			"shrunk", Field{Page: 1, Rect: [4]float64{50, 400, 50 + w12*0.95, 420}, Text: line, Font: "Helvetica", Size: 12},
			FitShrunk, "5% over: one point down clears it",
		},
		{
			// Far too wide to shrink into (would need ~2pt), but the box is TALL, so
			// the wrapped lines have somewhere to go.
			"wrapped", Field{Page: 1, Rect: [4]float64{50, 300, 130, 420}, Text: "alpha bravo charlie delta echo foxtrot", Font: "Helvetica", Size: 12},
			FitWrapped, "120pt tall: room for the lines wrapping produces",
		},
		{
			// The same text in a ONE-LINE box: shrinking cannot reach it and the box
			// has no vertical room, so it is stamped and reported.
			"overran", Field{Page: 1, Rect: [4]float64{50, 400, 130, 420}, Text: "alpha bravo charlie delta echo foxtrot", Font: "Helvetica", Size: 12},
			FitOverran, "same text, 20pt tall: nowhere to wrap into",
		},
	}

	seen := map[FitOutcome]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got, _, _ := resolveFit(tc.f)
			if got != tc.want {
				t.Errorf("outcome %q, want %q (%s)", got, tc.want, tc.why)
			}
			seen[got] = true
		})
	}
	for _, o := range []FitOutcome{FitAsDrawn, FitShrunk, FitWrapped, FitOverran} {
		if !seen[o] {
			t.Errorf("no fixture reached %q — that arm is asserted by nothing", o)
		}
	}
}

// C2 — wrap fires ONLY when the wrapped lines fit the box's height.
//
// Driven from both sides of the boundary with the SAME text, so the only thing that
// changes is the height. Without this, "wrapped" and "overran" are distinguishable
// only by text, and a wrap that ignored height entirely would pass the table above.
func TestWrapIsGatedOnMeasuredVerticalRoom(t *testing.T) {
	const text = "alpha bravo charlie delta echo foxtrot"
	const pts = 12
	boxW := 130.0 - (50 + stampInsetPt)
	lines := mdpdf.WrapCore(text, "Helvetica", pts, boxW)
	if len(lines) < 2 {
		t.Fatalf("fixture is inert: the text wraps to %d line(s), so there is no height "+
			"question to ask", len(lines))
	}
	need := float64(len(lines)) * mdpdf.CoreLineHeight("Helvetica", pts)

	// Exactly enough room, and one point less.
	for _, tc := range []struct {
		name string
		h    float64
		want FitOutcome
	}{
		{"exactly enough", need, FitWrapped},
		{"one point short", need - 1, FitOverran},
	} {
		t.Run(tc.name, func(t *testing.T) {
			y0 := 400.0
			f := Field{Page: 1, Rect: [4]float64{50, y0, 130, y0 + stampInsetPt + tc.h}, Text: text, Font: "Helvetica", Size: pts}
			if _, got, _, _ := resolveFit(f); got != tc.want {
				t.Errorf("%d lines need %.2fpt and the box offers %.2fpt: outcome %q, want %q",
					len(lines), need, tc.h, got, tc.want)
			}
		})
	}
}

// C3 — shrink stops at the floor and never jumps up.
//
// Probed at every size, because the bug this replaces was a DISCONTINUITY, not an
// off-by-one: stampStyle used to turn 5, 4 and 1 into 8, so a loop walking down
// from 7 got a larger size back and never converged.
func TestShrinkStopsAtTheFloorAndNeverJumpsUp(t *testing.T) {
	for size := 1; size <= 20; size++ {
		f := Field{Page: 1, Rect: [4]float64{50, 400, 150, 420}, Text: "x", Font: "Helvetica", Size: float64(size)}
		_, pts := stampStyle(f)
		if size >= stampFloorPt && pts != size {
			t.Errorf("size %d became %d — at or above the floor nothing should move it", size, pts)
		}
		if pts < stampFloorPt {
			t.Errorf("size %d became %d, below the %dpt floor", size, pts, stampFloorPt)
		}
	}
	// The ladder lands ON its floor rather than skipping past it — and which floor
	// binds depends on the size asked for, so both arms are driven.
	for _, tc := range []struct{ asked, want int }{
		{12, 9}, // ratio binds: 0.75 x 12
		{8, 6},  // absolute binds: 0.75 x 8 = 6, and 6 is the floor
		{6, 6},  // already at the floor: nothing below it
	} {
		if got := shrinkFloorFor(tc.asked); got != tc.want {
			t.Errorf("shrinkFloorFor(%d) = %d, want %d", tc.asked, got, tc.want)
		}
	}
	// From 12pt, text that fits only at its ratio floor comes back SHRUNK at that size.
	w9 := stampWidth("Plain ASCII", "Helvetica", shrinkFloorFor(12))
	atFloor := Field{Page: 1, Rect: [4]float64{50, 400, 50 + stampInsetPt + w9, 420}, Text: "Plain ASCII", Font: "Helvetica", Size: 12}
	if got, outcome, _, _ := resolveFit(atFloor); outcome != FitShrunk || int(got.Size) != shrinkFloorFor(12) {
		t.Errorf("text that fits exactly at the ratio floor came back %q at %.0fpt, want %q at %d",
			outcome, got.Size, FitShrunk, shrinkFloorFor(12))
	}
}

// The ratio bound is what stops a shrink silently rewriting the page.
//
// Text that would need HALF its asked-for size is not shrunk to fit; it is stamped as
// typed and reported. The bound is the whole reason: a 12pt edit set at 6pt no longer
// reads as the line it replaced, and on an executed document a silent resize is worse
// than a visible overrun, because the overrun is something the user can see and fix.
func TestShrinkRefusesToRewriteThePageAndReportsInstead(t *testing.T) {
	const line = "Plain ASCII"
	// A box that only 6pt would fit — below 12pt's ratio floor of 9pt.
	w6 := stampWidth(line, "Helvetica", stampFloorPt)
	if w6 >= stampWidth(line, "Helvetica", shrinkFloorFor(12)) {
		t.Fatalf("fixture is inert: %.2fpt at the absolute floor is not narrower than the "+
			"ratio floor's width, so no box can sit between them", w6)
	}
	f := Field{Page: 1, Rect: [4]float64{50, 400, 50 + stampInsetPt + w6, 420}, Text: line, Font: "Helvetica", Size: 12}

	got, outcome, _, _ := resolveFit(f)
	if outcome != FitOverran {
		t.Errorf("outcome %q at %.0fpt: text needing %dpt from an asked-for 12pt must be "+
			"REPORTED, not shrunk past the ratio bound — a silent halving rewrites the page",
			outcome, got.Size, stampFloorPt)
	}
	if int(got.Size) != 12 {
		t.Errorf("the reported field was stamped at %.0fpt, want the 12pt that was typed — "+
			"an overran outcome must bake what the user asked for", got.Size)
	}
	// The control: raise the box to the ratio floor's width and it DOES shrink, so the
	// refusal above is the bound firing rather than shrink being broken.
	w9 := stampWidth(line, "Helvetica", shrinkFloorFor(12))
	ok := Field{Page: 1, Rect: [4]float64{50, 400, 50 + stampInsetPt + w9, 420}, Text: line, Font: "Helvetica", Size: 12}
	if _, oc, _, _ := resolveFit(ok); oc != FitShrunk {
		t.Fatalf("the control did not shrink (%q) — the refusal above proves nothing", oc)
	}
}

// C4 — a field that cannot be made to fit is still STAMPED, and the document is whole.
//
// /api/bake is what every save, print, flatten, export and signature runs through,
// and the client aborts the whole operation on a bake that is not OK. Refusing here
// would make a document carrying one over-long edit impossible to save at all — so
// the outcome is a report, and this is the assertion that keeps it one.
func TestAnUnfittableFieldStillProducesAWholeDocument(t *testing.T) {
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	f := Field{Page: 1, Rect: [4]float64{50, 400, 130, 420},
		Text: "alpha bravo charlie delta echo foxtrot", Font: "Helvetica", Size: 12}
	out, fits, err := StampFields(pdf, []Field{f})
	if err != nil {
		t.Fatalf("an over-long field failed the bake: %v — every save runs through this", err)
	}
	if len(fits) != 1 || fits[0].Outcome != FitOverran {
		t.Fatalf("got %d fit(s) outcome %v, want one %q", len(fits), fits, FitOverran)
	}
	if fits[0].OverrunPt <= 0 {
		t.Errorf("outcome is %q but the overrun is %.2fpt — the report contradicts itself",
			FitOverran, fits[0].OverrunPt)
	}
	// The document is readable and the text is really on the page.
	if _, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("the returned PDF does not re-read: %v", err)
	}
	if len(out) <= len(pdf) {
		t.Error("nothing was added to the document")
	}
}

// StampedPt is what predicts the emitted size once shrink can fire — stampStyle
// alone no longer does, and that narrowing is asserted rather than assumed.
func TestShrinkIsWhatTheTfReports(t *testing.T) {
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	const line = "Plain ASCII"
	w12 := stampWidth(line, "Helvetica", 12)
	f := Field{Page: 1, Rect: [4]float64{50, 400, 50 + w12*0.95, 420}, Text: line, Font: "Helvetica", Size: 12}

	_, fits, err := StampFields(pdf, []Field{f})
	if err != nil {
		t.Fatal(err)
	}
	if fits[0].Outcome != FitShrunk {
		t.Fatalf("fixture did not shrink (outcome %q), so this asserts nothing", fits[0].Outcome)
	}
	if _, asked := stampStyle(f); fits[0].StampedPt >= asked {
		t.Errorf("StampedPt %d is not below the %dpt asked for", fits[0].StampedPt, asked)
	}
	resolved, _, _, _ := resolveFit(f)
	if got := emittedTfSize(t, pdf, resolved); math.Abs(got-float64(fits[0].StampedPt)) > 0.01 {
		t.Errorf("StampedPt says %d, pdfcpu emitted %.2fpt", fits[0].StampedPt, got)
	}
}

// S04 — the fit verdict says whether the width it rests on is the document's own.
//
// `classifyFont` cannot fail, only be wrong: it is a substring search defaulting to
// Helvetica, so a document set in a display face is measured with Helvetica's widths
// and reports its verdict with exactly the confidence of one measured exactly. This
// is the signal that separates them, and the cases are the ones pdf.js really hands
// over — measured in a real browser 2026-09-09: "Courier" for a Courier document,
// "Helvetica-Bold" for a Helvetica-Bold one, each prefixed by a CSS generic.
func TestFontFidelitySeparatesAnExactWidthFromAGuess(t *testing.T) {
	cases := []struct {
		name, baseFont, stamped string
		want                    FontFidelity
	}{
		{"the document's own face", "Courier", "Courier", FontExact},
		{"as pdf.js hands it over", "monospace Courier", "Courier", FontExact},
		{"bold, exactly", "sans-serif Helvetica-Bold", "Helvetica-Bold", FontExact},
		{"metric-compatible alias", "sans-serif ArialMT", "Helvetica", FontAlias},
		{"alias with punctuation", "Arial-BoldMT", "Helvetica-Bold", FontAlias},
		{"Courier New", "monospace CourierNewPSMT", "Courier", FontAlias},
		{"a subset of something else", "ABCDEF+MinionPro-Regular", "Helvetica", FontGuess},
		// **The case that proves the prefix is stripped at all.** A subset of a font
		// whose widths ARE the stamped face's must read as an alias — and without the
		// strip it reads as a guess, because "ABCDEF+Arial" matches no alias key. The
		// MinionPro case above cannot show this: it is a guess either way.
		{"a subset of a metric-compatible face", "ABCDEF+ArialMT", "Helvetica", FontAlias},
		{"a subset of the stamped face itself", "ABCDEF+Courier", "Courier", FontExact},
		{"a display face", "sans-serif Impact", "Helvetica", FontGuess},
		{"nothing was sent", "", "Helvetica", FontUnknown},
		{"whitespace is nothing", "   ", "Helvetica", FontUnknown},
		// The one that matters most: an alias name that does NOT match the face being
		// stamped is a guess, not an alias. Arial's widths are Helvetica's, not Courier's.
		{"alias of a different face", "ArialMT", "Courier", FontGuess},
	}
	seen := map[FontFidelity]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fontFidelityFor(tc.baseFont, tc.stamped); got != tc.want {
				t.Errorf("fontFidelityFor(%q, %q) = %q, want %q", tc.baseFont, tc.stamped, got, tc.want)
			}
			seen[fontFidelityFor(tc.baseFont, tc.stamped)] = true
		})
	}
	for _, f := range []FontFidelity{FontExact, FontAlias, FontGuess, FontUnknown} {
		if !seen[f] {
			t.Errorf("no case reached %q — that arm is asserted by nothing", f)
		}
	}
}

// The fidelity reaches the fit report, and a document that sends nothing still works.
//
// The second half is the compatibility clause: every pre-S04 client, the CLI, and
// every Go test constructs a Field with no BaseFont, and all of them must keep
// stamping exactly as before.
func TestFitCarriesFidelityAndWorksWithoutABaseFont(t *testing.T) {
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	roomy := [4]float64{50, 400, 400, 420}
	for _, tc := range []struct {
		name, baseFont string
		want           FontFidelity
	}{
		{"told, and exact", "Helvetica", FontExact},
		{"told, and a guess", "ABCDEF+MinionPro-Regular", FontGuess},
		{"not told", "", FontUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := Field{Page: 1, Rect: roomy, Text: "Plain ASCII", Font: "Helvetica", BaseFont: tc.baseFont, Size: 12}
			out, fits, err := StampFields(pdf, []Field{f})
			if err != nil {
				t.Fatalf("StampFields: %v", err)
			}
			if len(fits) != 1 {
				t.Fatalf("got %d fits, want 1", len(fits))
			}
			if fits[0].Fidelity != tc.want {
				t.Errorf("Fidelity = %q, want %q", fits[0].Fidelity, tc.want)
			}
			// Whatever it was told, the STAMP is unchanged — BaseFont is advisory.
			if got := emittedBBoxWidth(t, pdf, f); math.Abs(got-fits[0].WidthPt) > 0.01 {
				t.Errorf("BaseFont %q changed what was emitted: %.2f vs reported %.2f",
					tc.baseFont, got, fits[0].WidthPt)
			}
			if !bytes.HasPrefix(out, []byte("%PDF")) {
				t.Error("no PDF came back")
			}
		})
	}
}
