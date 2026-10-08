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
	"1 g 50 50 100 100 re f 0 g\n" + // painted white: a ground, and no box
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
		if mapNear(s.Rect[0], fx(50)) && mapNear(s.Rect[3], fy(50)) && s.Kind != "white" {
			t.Errorf("a white-painted rectangle is in the map as %+v: on a white page it is no box and no line", s)
		}
		if mapNear(s.Rect[0], fx(100)) && mapNear(s.Rect[3], fy(100)) {
			t.Errorf("a slanted line is in the map as %+v; its box says nothing about where it is", s)
		}
	}
	findShape(t, m, "white", fx(50), fy(150)) // the white rectangle, as the ground it is (ADR-096)
	if got := len(m.Shapes); got != 7 {
		t.Errorf("the map holds %d shapes, want 7: %+v", got, m.Shapes)
	}
}

// TestAWhiteGroundIsInTheMapAsAGround — ADR-096. A form made in a designer lays a white rectangle under each blank,
// and on a tinted page that rectangle is the only edge the blank has. It is kept as its own kind, so nothing that
// reads lines and boxes sees it; a white RULE erases part of another and is not kept; a ground a tint was painted
// over afterwards is not white on the page; and a box outlined in ink with a white inside is an empty box, not a
// shaded one.
func TestAWhiteGroundIsInTheMapAsAGround(t *testing.T) {
	const content = "0.9 g 100 600 300 100 re f\n" + // a tinted panel
		"1 g 120 650 80 12 re f\n" + // a white ground on it
		"1 g 120 640 80 0.8 re f\n" + // a white rule: an eraser
		"1 g 120 400 80 12 re f 0.9 g 100 380 300 50 re f\n" + // a white ground, then a tint over it
		"1 g 0 G 300 300 100 30 re B\n" + // outlined in black, white inside
		"0.9 g 300 200 100 30 re B\n" + // outlined in black, tinted inside
		"1 1 1 rg 0 0 1 RG 300 100 100 30 re B\n" + // outlined in blue, white inside
		"1 g 1 G 400 500 50 20 re S\n" + // outlined in white and not filled: nothing
		"1 g 420 650 60 12 re f 1 g 410 640 80 30 re f\n" + // a white ground, then a larger WHITE one over it: both white
		"1 g 120 500 80 12 re f 0 G 110 490 100 30 re S\n" + // a white ground, then an OUTLINE around it: still white
		// Three grounds each under a tint that stops short of one of its sides — left, right, bottom: still white there.
		"1 g 20 40 60 12 re f 0.9 g 30 30 60 30 re f\n" +
		"1 g 120 40 60 12 re f 0.9 g 110 30 60 30 re f\n" +
		"1 g 220 40 60 12 re f 0.9 g 210 45 80 30 re f\n" +
		"BT /F1 10 Tf 100 750 Td (Name) Tj ET"
	m, err := MapPage(pageWith(content, "", "", "", helvetica), 1)
	if err != nil {
		t.Fatal(err)
	}
	fx := func(x float64) float64 { return x / 612 }
	fy := func(y float64) float64 { return 1 - y/792 }

	g := findShape(t, m, "white", fx(120), fy(662))
	if !mapNear(g.Rect[2], fx(200)) || !mapNear(g.Rect[3], fy(650)) || g.Filled {
		t.Errorf("the white ground = %+v, want it to reach (%.4f, %.4f) and not be Filled — Filled is a colour", g, fx(200), fy(650))
	}
	whites := 0
	for _, s := range m.Shapes {
		if s.Kind == "white" {
			whites++
		}
		if mapNear(s.Rect[0], fx(120)) && mapNear(s.Rect[3], fy(640)) {
			t.Errorf("a white rule is in the map as %+v: it erases part of another line and is not a line", s)
		}
		if mapNear(s.Rect[0], fx(400)) && mapNear(s.Rect[3], fy(500)) {
			t.Errorf("a rectangle outlined in white is in the map as %+v: it has no inside, and on a white page no outline", s)
		}
		if mapNear(s.Rect[0], fx(120)) && mapNear(s.Rect[3], fy(400)) {
			t.Errorf("a white rectangle a tint was painted over is in the map as %+v: it is not white on the page", s)
		}
	}
	if whites != 7 {
		t.Errorf("the map holds %d white grounds, want 7: %+v", whites, m.Shapes)
	}
	for _, x := range []float64{20, 120, 220} {
		findShape(t, m, "white", fx(x), fy(52)) // a tint over part of a ground has not painted it out
	}
	findShape(t, m, "white", fx(420), fy(662)) // under a larger white one: still white on the page
	findShape(t, m, "white", fx(410), fy(670))
	findShape(t, m, "white", fx(120), fy(512)) // inside an outline: an outline paints nothing over it
	if b := findShape(t, m, "box", fx(300), fy(330)); b.Filled {
		t.Errorf("a box outlined in black and filled white = %+v: Filled is something other than white", b)
	}
	if b := findShape(t, m, "box", fx(300), fy(130)); b.Filled {
		t.Errorf("a box outlined in blue and filled white (rg) = %+v: Filled is something other than white", b)
	}
	if b := findShape(t, m, "box", fx(300), fy(230)); !b.Filled {
		t.Errorf("a box outlined in black and tinted = %+v, want Filled", b)
	}
	if p := findShape(t, m, "box", fx(100), fy(700)); !p.Filled {
		t.Errorf("the tinted panel = %+v, want Filled", p)
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

// TestAGroundPastTheBudgetIsTakenAsCovered: each ground is checked against everything painted after it, and a page
// chooses how much it paints. Past the budget the check stops and the ground is not proposed.
func TestAGroundPastTheBudgetIsTakenAsCovered(t *testing.T) {
	white := pageShape{box: [4]float64{10, 10, 60, 30}, rect: true, filled: true, white: true, whiteFill: true}
	shapes := []pageShape{white, {box: [4]float64{100, 100, 110, 110}, rect: true, filled: true}}
	budget := 1
	if paintedOver(shapes, 0, &budget) {
		t.Error("a ground nothing was painted over, inside the budget, was taken as covered")
	}
	if budget != 0 {
		t.Errorf("one piece was looked at and the budget went from 1 to %d", budget)
	}
	if !paintedOver(shapes, 0, &budget) {
		t.Error("with the budget spent, a ground was still walked against the page")
	}
}

// TestFaintPrintIsMarkedOnTheMap — ADR-099. A form prints a hint inside a blank in a light colour ("MM", "DD", "YYYY"
// at 0.753 in the IRS 1040's date blanks) and its labels in ink; the map says which a run is, from the colour it was
// filled with. White is not faint (a heading on a dark bar), nor is text that is not filled, nor an OCR layer.
func TestFaintPrintIsMarkedOnTheMap(t *testing.T) {
	const content = "BT /F1 10 Tf 100 750 Td\n" +
		"0.753 0.753 0.753 rg (Hint) Tj 0 -20 Td\n" +
		"0 0 0 rg (Ink) Tj 0 -20 Td\n" +
		"0.651 g (Grey) Tj 0 -20 Td\n" + // a greyed revision date on a corpus form: still a label
		"0.7 g (Edge) Tj 0 -20 Td\n" +
		"0.69 g (Under) Tj 0 -20 Td\n" +
		"1 1 1 rg (White) Tj 0 -20 Td\n" +
		"1 g (Blank) Tj 0 -20 Td\n" +
		"0 0 0 0.2 k (Cmyk) Tj 0 -20 Td\n" +
		"0 0 0 0.9 k (Dark) Tj 0 -20 Td\n" +
		"1 1 0 rg (Yellow) Tj 0 -20 Td\n" + // light, though no channel is grey
		"0 0 1 rg (Blue) Tj 0 -20 Td\n" +
		"0 1 1 0 k (Red) Tj 0 -20 Td\n" +
		"0 0.9 1 rg (Cyan) Tj 0 -20 Td\n" + // 0.64: green and blue weigh what they weigh, not what red does
		"0 0.1 0.1 0.5 k (Mid) Tj 0 -20 Td\n" + // light inks under half black
		"0 0 0.6 0 k (Lemon) Tj 0 -20 Td\n" +
		"1 0.1 0 0 k (Sky) Tj 0 -20 Td\n" + // 0.64: cyan takes out the red, magenta some green
		"/DeviceGray cs 0.8 sc (Space) Tj 0 -20 Td\n" +
		"0.9 g 0 G 1 Tr (Outline) Tj 0 -20 Td\n" + // stroked only: its colour is the stroke's
		"0.9 g 3 Tr (Layer) Tj 0 Tr ET"
	m, err := MapPage(pageWith(content, "", "", "", helvetica), 1)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Hint": true, "Ink": false, "Grey": false, "Edge": true, "Under": false, "White": false,
		"Blank": false, "Cmyk": true, "Dark": false, "Yellow": true, "Blue": false, "Red": false, "Cyan": false, "Mid": false, "Lemon": true, "Sky": false, "Space": true,
		"Outline": false, "Layer": false}
	seen := 0
	for _, r := range m.Text {
		w, ok := want[r.Text]
		if !ok {
			t.Errorf("an unexpected run %q", r.Text)
			continue
		}
		seen++
		if r.Faint != w {
			t.Errorf("%q: faint = %v, want %v", r.Text, r.Faint, w)
		}
	}
	if seen != len(want) {
		t.Errorf("the map holds %d of the %d runs: %+v", seen, len(want), m.Text)
	}
}

// White is white however the page sets it. Six spellings were matched by hand and `cs`/`sc` was not among them.
func TestWhiteIsWhiteHoweverItIsSet(t *testing.T) {
	for _, op := range []string{"1 g", "1 G", "1 1 1 rg", "1 1 1 RG", "0 0 0 0 k", "0 0 0 0 K",
		"/DeviceGray cs 1 sc", "/DeviceRGB cs 1 1 1 sc", "/DeviceCMYK cs 0 0 0 0 sc", "/DeviceRGB CS 1 1 1 SC"} {
		if !isWhitePaint(op) {
			t.Errorf("%q is white and was not read as white", op)
		}
	}
	for _, op := range []string{"", "0 g", "0.999 g", "1 1 0.99 rg", "0 0 0 0.01 k", "/DeviceGray cs 0.9 sc", "/DeviceRGB cs 1 1 0 sc", "/Pattern cs /P0 scn"} {
		if isWhitePaint(op) {
			t.Errorf("%q is not white and was read as white", op)
		}
	}
}
