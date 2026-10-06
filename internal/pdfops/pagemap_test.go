package pdfops

import (
	"math"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// ADR-088 — the page map.

const mapPageContent = "0.5 w 100 600 m 300 600 l S\n" + // a horizontal rule
	"100 500 m 300 500 l 300 520 l 100 520 l h S\n" + // a box drawn as four lines
	"72 400 200 0.8 re f\n" + // a rule drawn as a thin filled rectangle
	"400 300 m 400 400 l 500 400 l S\n" + // ONE path, two rules: a vertical then a horizontal
	"1 g 50 50 100 100 re f 0 g\n" + // painted white: not there
	"200 200 12 12 re S\n" + // a checkbox-sized square
	"100 100 m 300 101 l S\n" + // a line a point off level: its box is rule-thin, and it is not a rule
	"BT /F1 10 Tf 100 700 Td (Name) Tj ET"

func mapNear(a, b float64) bool { return math.Abs(a-b) < 0.0012 } // under a point on a letter page

func findShape(t *testing.T, m PageMap, kind string, left, top float64) MapShape {
	t.Helper()
	for _, s := range m.Shapes {
		if s.Kind == kind && mapNear(s.Rect[0], left) && mapNear(s.Rect[1], top) {
			return s
		}
	}
	t.Fatalf("no %q shape at left %.4f top %.4f; the map holds %+v", kind, left, top, m.Shapes)
	return MapShape{}
}

// TestTheMapHoldsEachRuleAndBoxWhereThePageDrawsIt — a piece of a path is its own shape, in fractions of the page
// from its top-left.
func TestTheMapHoldsEachRuleAndBoxWhereThePageDrawsIt(t *testing.T) {
	m, err := MapPage(pageWith(mapPageContent, "", "", "", helvetica), 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Width != 612 || m.Height != 792 {
		t.Fatalf("displayed page = %gx%g, want 612x792", m.Width, m.Height)
	}
	fx := func(x float64) float64 { return x / 612 }
	fy := func(y float64) float64 { return 1 - y/792 }

	h := findShape(t, m, "h", fx(100-0.25), fy(600+0.25))
	if !mapNear(h.Rect[2], fx(300+0.25)) {
		t.Errorf("the rule ends at %.4f, want %.4f — its extent is the line's, not an estimate", h.Rect[2], fx(300.25))
	}
	box := findShape(t, m, "box", fx(100), fy(520))
	if !mapNear(box.Rect[2], fx(300)) || !mapNear(box.Rect[3], fy(500)) {
		t.Errorf("the four-line box = %v, want it to reach (%.4f, %.4f)", box.Rect, fx(300), fy(500))
	}
	findShape(t, m, "h", fx(72), fy(400.8))          // the thin filled rectangle is a rule
	findShape(t, m, "v", fx(400-0.25), fy(400+0.25)) // one path…
	findShape(t, m, "h", fx(400-0.25), fy(400+0.25)) // …two rules
	findShape(t, m, "box", fx(200), fy(212))         // the small square
	for _, s := range m.Shapes {
		if mapNear(s.Rect[0], fx(50)) && mapNear(s.Rect[3], fy(50)) {
			t.Errorf("a white-painted rectangle is in the map (%+v): on a white page it is not there", s)
		}
		if mapNear(s.Rect[0], fx(100)) && mapNear(s.Rect[3], fy(100)) {
			t.Errorf("a slanted line is in the map as %+v; its box says nothing about where it is", s)
		}
	}
	if got := len(m.Shapes); got != 6 {
		t.Errorf("the map holds %d shapes, want 6: %+v", got, m.Shapes)
	}
}

// TestTheMapPlacesEachGlyphByTheFontsOwnWidths — Helvetica's N a m e are 722, 556, 833, 556 thousandths.
func TestTheMapPlacesEachGlyphByTheFontsOwnWidths(t *testing.T) {
	m, err := MapPage(pageWith(mapPageContent, "", "", "", helvetica), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Text) != 1 || m.Text[0].Text != "Name" {
		t.Fatalf("text = %+v, want the one run \"Name\"", m.Text)
	}
	r := m.Text[0]
	if strings.Join(r.Chars, "") != "Name" || len(r.Cuts) != 5 {
		t.Fatalf("chars %q with %d cuts, want Name and 5", r.Chars, len(r.Cuts))
	}
	x := 100.0
	for i, w := range []float64{0, 7.22, 5.56, 8.33, 5.56} {
		x += w
		if !mapNear(r.Cuts[i], x/612) {
			t.Errorf("cut %d = %.5f, want %.5f (x=%.2fpt) — a glyph placed by anything but the font's width", i, r.Cuts[i], x/612, x)
		}
	}
	if !mapNear(r.Rect[0], 100.0/612) || !mapNear(r.Rect[2], x/612) {
		t.Errorf("the run's box = %v, want it from 100pt to %.2fpt across", r.Rect, x)
	}
	if !(r.Rect[1] < 1-700.0/792 && r.Rect[3] > 1-700.0/792) {
		t.Errorf("the run's box %v does not straddle its baseline at %.4f", r.Rect, 1-700.0/792)
	}
	if m.NoText {
		t.Error("a page with text says it has none")
	}
}

// TestAKernMovesTheGlyphsAfterIt — `[(Na) -500 (me)] TJ` opens half an em between a and m.
func TestAKernMovesTheGlyphsAfterIt(t *testing.T) {
	m, err := MapPage(pageWith("BT /F1 10 Tf 100 700 Td [(Na) -500 (me)] TJ ET", "", "", "", helvetica), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Text) != 1 || len(m.Text[0].Cuts) != 5 {
		t.Fatalf("text = %+v, want one run of four glyphs", m.Text)
	}
	c := m.Text[0].Cuts
	// N 7.22, a 5.56, then 5pt of kern, then m 8.33 and e 5.56.
	if want := (100 + 7.22 + 5.56 + 5) / 612; !mapNear(c[2], want) {
		t.Errorf("m starts at %.5f, want %.5f — the kern before it was not counted, so every glyph after it is misplaced", c[2], want)
	}
	if want := (100 + 7.22 + 5.56 + 5 + 8.33 + 5.56) / 612; !mapNear(c[4], want) {
		t.Errorf("the run ends at %.5f, want %.5f", c[4], want)
	}
}

// TestTheMapIsInTheDisplayedPagesSpace — a crop box whose corner is not the origin, and a quarter turn. Every caller
// gets fractions of what is on screen; none of them has to know either was there.
func TestTheMapIsInTheDisplayedPagesSpace(t *testing.T) {
	rule := "0 w 112 120 m 312 120 l S"
	t.Run("crop box off the origin", func(t *testing.T) {
		m, err := MapPage(pageWith(rule, "", "", "/CropBox [12 20 612 792] ", helvetica), 1)
		if err != nil {
			t.Fatal(err)
		}
		if m.Width != 600 || m.Height != 772 {
			t.Fatalf("displayed page = %gx%g, want the crop box's 600x772", m.Width, m.Height)
		}
		s := m.Shapes[0]
		if s.Kind != "h" || !mapNear(s.Rect[0], 100.0/600) || !mapNear(s.Rect[2], 300.0/600) || !mapNear(s.Rect[1], 1-100.0/772) {
			t.Errorf("the rule = %+v, want h from %.4f to %.4f at %.4f — measured from the crop box, not the media box", s, 100.0/600, 300.0/600, 1-100.0/772)
		}
	})
	t.Run("turned a quarter", func(t *testing.T) {
		m, err := MapPage(pageWith(rule, "", "", "/CropBox [12 20 612 792] /Rotate 90 ", helvetica), 1)
		if err != nil {
			t.Fatal(err)
		}
		if m.Width != 772 || m.Height != 600 {
			t.Fatalf("displayed page = %gx%g, want 772x600 — the turn swaps them", m.Width, m.Height)
		}
		s := m.Shapes[0]
		// The page's bottom edge is the display's left edge: a horizontal rule 100pt up is a vertical one 100pt in.
		if s.Kind != "v" || !mapNear(s.Rect[0], 100.0/772) || !mapNear(s.Rect[1], 100.0/600) || !mapNear(s.Rect[3], 300.0/600) {
			t.Errorf("the rule = %+v, want v at %.4f from %.4f to %.4f", s, 100.0/772, 100.0/600, 300.0/600)
		}
	})
	t.Run("turned a half and three quarters", func(t *testing.T) {
		for rot, want := range map[string]MapRect{
			"180": {1 - 300.0/600, 100.0 / 772, 1 - 100.0/600, 100.0 / 772},
			"270": {1 - 100.0/772, 1 - 300.0/600, 1 - 100.0/772, 1 - 100.0/600},
		} {
			m, err := MapPage(pageWith(rule, "", "", "/CropBox [12 20 612 792] /Rotate "+rot+" ", helvetica), 1)
			if err != nil {
				t.Fatal(err)
			}
			s := m.Shapes[0]
			mid := MapRect{(s.Rect[0] + s.Rect[2]) / 2, s.Rect[1], (s.Rect[0] + s.Rect[2]) / 2, s.Rect[3]}
			if rot == "180" {
				mid = MapRect{s.Rect[0], (s.Rect[1] + s.Rect[3]) / 2, s.Rect[2], (s.Rect[1] + s.Rect[3]) / 2}
			}
			for i := range want {
				if !mapNear(mid[i], want[i]) {
					t.Errorf("/Rotate %s: the rule's centre line = %v, want %v", rot, mid, want)
					break
				}
			}
		}
	})
}

// TestTheMapNamesTheFieldsTheDocumentAlreadyHas — so nothing proposes a field where one is.
func TestTheMapNamesTheFieldsTheDocumentAlreadyHas(t *testing.T) {
	pdf := pageWith("", "", "/AcroForm << /Fields [6 0 R 7 0 R 8 0 R] /DA (/Helv 0 Tf 0 g) >> ", "/Annots [6 0 R 7 0 R 8 0 R] ", helvetica,
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (surname) /Rect [100 100 300 120] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Btn /T (agree) /Rect [320 100 332 112] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Btn /Ff 32768 /T (choice) /Rect [340 100 352 112] >>")
	m, err := MapPage(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Widgets) != 3 {
		t.Fatalf("widgets = %+v, want three", m.Widgets)
	}
	for i, want := range []MapWidget{{Kind: "text", Name: "surname"}, {Kind: "check", Name: "agree"}, {Kind: "radio", Name: "choice"}} {
		if got := m.Widgets[i]; got.Kind != want.Kind || got.Name != want.Name {
			t.Errorf("widget %d = %s %q, want %s %q", i, got.Kind, got.Name, want.Kind, want.Name)
		}
	}
	if w := m.Widgets[0].Rect; !mapNear(w[0], 100.0/612) || !mapNear(w[1], 1-120.0/792) || !mapNear(w[2], 300.0/612) || !mapNear(w[3], 1-100.0/792) {
		t.Errorf("the text field's rect = %v, want it where /Rect puts it", w)
	}
}

// TestOnlyTheMapsReaderKeepsShapes — reflow's and tagging's readers walk past every path piece, as before.
func TestOnlyTheMapsReaderKeepsShapes(t *testing.T) {
	ctx, err := pdfread.Validated(pageWith(mapPageContent, "", "", "", helvetica), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 1)
	for name, read := range map[string]func() (pageRuns, error){
		"readPageRuns":      func() (pageRuns, error) { return readPageRuns(ctx, pg) },
		"readPageGlyphRuns": func() (pageRuns, error) { return readPageGlyphRuns(ctx, pg) },
	} {
		pr, err := read()
		if err != nil {
			t.Fatal(err)
		}
		if len(pr.shapes) != 0 {
			t.Errorf("%s kept %d shapes; only the page map's reader pays for them", name, len(pr.shapes))
		}
	}
	pr, err := readPageShapes(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.shapes) == 0 || len(pr.marks) == 0 || len(pr.runs) != 1 {
		t.Errorf("the map's reader kept %d shapes, %d marks, %d runs; want all three", len(pr.shapes), len(pr.marks), len(pr.runs))
	}
}
