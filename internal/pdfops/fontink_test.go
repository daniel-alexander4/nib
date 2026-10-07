package pdfops

import (
	"encoding/binary"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"nib/internal/pdfread"
	"nib/mdpdf"
)

// Print's own ink on the map (ADR-098, fontink.go).
//
// What these can reach: the reader against a second reader of the same programs (x/image's, which shares no code
// with it), the rule for which answer a font gives, and the whole path from an authored page to `MapText.Ink`.
// What they cannot: whether the ink is where a renderer paints it. That is `build/accuracy.sh`'s vertical-reach
// column, against poppler's pixels on real documents.

const inkProbeMarkdown = "# Apply\n\nxxxx\n\nglyph Apply\n"

// vendoredFace is one of nib's own TrueType files and x/image's reading of it.
func vendoredFace(t *testing.T, path string) ([]byte, *sfnt.Font) {
	t.Helper()
	b, err := ocrFontFS.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	f, err := sfnt.Parse(b)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return b, f
}

// TestAProgramsGlyphsReachWhereAnotherReaderSaysTheyDo: every glyph of nib's own faces, read from its points, against
// x/image's bounds for the same glyph. The two share nothing but the file.
func TestAProgramsGlyphsReachWhereAnotherReaderSaysTheyDo(t *testing.T) {
	read := 0
	for name, path := range authoringFontFiles {
		b, face := vendoredFace(t, path)
		ink := readGlyfInk(b)
		if ink == nil {
			t.Errorf("%s: not read", name)
			continue
		}
		read++
		upem := float64(face.UnitsPerEm())
		var buf sfnt.Buffer
		checked := 0
		for g := 0; g < face.NumGlyphs(); g++ {
			bounds, _, err := face.GlyphBounds(&buf, sfnt.GlyphIndex(g), fixed.I(int(face.UnitsPerEm())), font.HintingNone)
			if err != nil {
				continue
			}
			lo, hi := -float64(bounds.Max.Y)/64/upem, -float64(bounds.Min.Y)/64/upem // sfnt's y runs down
			if !(hi > lo) {
				if ink.inked[g] {
					t.Errorf("%s glyph %d: read as inked %v, and it paints nothing", name, g, ink.glyphs[g])
				}
				continue
			}
			checked++
			if !ink.inked[g] || math.Abs(ink.glyphs[g][0]-lo) > 1.5/upem || math.Abs(ink.glyphs[g][1]-hi) > 1.5/upem {
				t.Errorf("%s glyph %d: read %v (inked %v), want [%.4f %.4f]", name, g, ink.glyphs[g], ink.inked[g], lo, hi)
			}
			if ink.glyphs[g][0] < ink.lo || ink.glyphs[g][1] > ink.hi {
				t.Errorf("%s glyph %d reaches %v, outside the font's own %.4f..%.4f", name, g, ink.glyphs[g], ink.lo, ink.hi)
			}
		}
		if checked < 100 {
			t.Errorf("%s: only %d glyphs compared", name, checked)
		}
	}
	if read == 0 {
		t.Fatal("no vendored face could be read: the comparison compared nothing")
	}
}

// TestAnAuthoredPagesPrintCarriesTheInkOfTheGlyphsItShows is the whole path: nib's own Markdown conversion sets text
// in an embedded TrueType program under Identity-H, so each run's ink is its own glyphs' — checked against x/image
// measuring the same TEXT in the same face, which reaches the glyphs through the face's character map and not
// through the codes the page shows.
func TestAnAuthoredPagesPrintCarriesTheInkOfTheGlyphsItShows(t *testing.T) {
	pdf, err := ConvertDocToPDF([]byte(inkProbeMarkdown), ".md")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageShapes(ctx, pageAt(ctx, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string][2]float64{}
	for _, r := range pr.runs {
		text := strings.TrimSpace(r.text)
		if text == "" {
			continue
		}
		bottom, top, ok := printInk(r)
		if !ok {
			t.Fatalf("%q in %s has no ink", r.text, r.baseFont)
		}
		name := r.baseFont[strings.IndexByte(r.baseFont, '+')+1:]
		path, known := authoringFontFiles[name]
		if !known {
			t.Fatalf("%q is set in %s, which is not one of the authoring faces", r.text, name)
		}
		_, face := vendoredFace(t, path)
		wantBottom, wantTop := math.Inf(1), math.Inf(-1)
		var buf sfnt.Buffer
		for _, c := range r.text {
			gi, gerr := face.GlyphIndex(&buf, c)
			if gerr != nil || gi == 0 {
				t.Fatalf("%s has no glyph for %q", name, c)
			}
			b, _, berr := face.GlyphBounds(&buf, gi, fixed.I(int(face.UnitsPerEm())), font.HintingNone)
			if berr != nil || b.Max.Y <= b.Min.Y {
				continue
			}
			upem := float64(face.UnitsPerEm())
			wantBottom, wantTop = math.Min(wantBottom, -float64(b.Max.Y)/64/upem), math.Max(wantTop, -float64(b.Min.Y)/64/upem)
		}
		gotBottom, gotTop := (bottom-r.y)/r.size, (top-r.y)/r.size
		if math.Abs(gotBottom-wantBottom) > 0.002 || math.Abs(gotTop-wantTop) > 0.002 {
			t.Errorf("%q in %s: ink %.4f..%.4f of a size, want %.4f..%.4f", r.text, name, gotBottom, gotTop, wantBottom, wantTop)
		}
		seen[text] = [2]float64{gotBottom, gotTop}
	}
	x, apply := seen["xxxx"], seen["glyph Apply"]
	if x == ([2]float64{}) || apply == ([2]float64{}) {
		t.Fatalf("the page's runs are not the ones this reads: %v", seen)
	}
	// The point of reading the run's own glyphs: a word with no tall letter and no tail is boxed lower and shallower.
	if !(x[1] < apply[1]-0.1 && x[0] > apply[0]+0.1 && x[0] > -0.05) {
		t.Errorf("\"xxxx\" reaches %.3f..%.3f and \"glyph Apply\" %.3f..%.3f: the first should stop well inside the second", x[0], x[1], apply[0], apply[1])
	}

	// And on the map: every run of print has its ink, inside its line's reach here, top above bottom.
	m, err := MapPage(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Text) == 0 {
		t.Fatal("the page maps no text")
	}
	for _, mt := range m.Text {
		if len(mt.Ink) != 2 {
			t.Fatalf("%q has no ink on the map: %+v", mt.Text, mt)
		}
		if !(mt.Ink[0] < mt.Ink[1] && mt.Ink[0] > mt.Rect[1] && mt.Ink[1] <= mt.Rect[3]+1e-9) {
			t.Errorf("%q: ink %v against its reach %v..%v", mt.Text, mt.Ink, mt.Rect[1], mt.Rect[3])
		}
	}
}

// TestPrintInAFontThatIsNotEmbeddedHasNoInk: the same page set in the core faces carries no program, so no run is
// given ink and every box stays the line's reach.
func TestPrintInAFontThatIsNotEmbeddedHasNoInk(t *testing.T) {
	pdf, err := mdpdf.ConvertWithFonts([]byte(inkProbeMarkdown), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	m, err := MapPage(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Text) == 0 {
		t.Fatal("the page maps no text")
	}
	for _, mt := range m.Text {
		if len(mt.Ink) != 0 {
			t.Errorf("%q, set in a font the document does not carry, was given ink %v", mt.Text, mt.Ink)
		}
	}
}

// TestWhichInkARunIsGiven is `printInk`'s rule, on fonts written out by hand.
func TestWhichInkARunIsGiven(t *testing.T) {
	src := func(ink fontInk) *runFont { return &runFont{ink: &inkSource{done: true, ink: &ink}} }
	run := func(f *runFont, codes ...int) textRun {
		r := textRun{text: "x", size: 10, x: 50, y: 100, width: 5, face: f}
		for _, c := range codes {
			r.glyphs = append(r.glyphs, runGlyph{code: []byte{byte(c >> 8), byte(c)}, advance: 1})
		}
		return r
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

	// A font-wide answer only tightens: inside the reach it is used, past the reach the reach stands.
	if b, top, ok := printInk(run(src(fontInk{lo: -0.2, hi: 0.7}), 1)); !ok || !near(b, 98) || !near(top, 107) {
		t.Errorf("a font reaching -0.2..0.7: ink %v..%v (ok=%v), want 98..107", b, top, ok)
	}
	if b, top, ok := printInk(run(src(fontInk{lo: -0.4, hi: 1.2}), 1)); !ok || !near(b, 100-10*descentEm) || !near(top, 100+10*ascentEm) {
		t.Errorf("a font reaching -0.4..1.2: ink %v..%v (ok=%v), want the line's reach", b, top, ok)
	}

	// Codes that are glyph numbers: the run's own glyphs, deeper than the reach where they are, an empty glyph
	// passed over, and a number past the program's last glyph drawn as its first.
	byCode := fontInk{lo: -0.5, hi: 0.9, byCode: true,
		glyphs: [][2]float64{{0, 0.6}, {-0.01, 0.5}, {-0.45, 0.3}, {0, 0}, {0.2, 0.4}},
		inked:  []bool{true, true, true, false, true}}
	if b, top, ok := printInk(run(src(byCode), 4, 3)); !ok || !near(b, 102) || !near(top, 104) {
		t.Errorf("a raised glyph beside an empty one: ink %v..%v (ok=%v), want 102..104 — the empty glyph is not at the baseline", b, top, ok)
	}
	if b, top, ok := printInk(run(src(byCode), 1, 3)); !ok || !near(b, 99.9) || !near(top, 105) {
		t.Errorf("glyph 1 with an empty glyph: ink %v..%v (ok=%v), want 99.9..105", b, top, ok)
	}
	if b, top, ok := printInk(run(src(byCode), 1, 2)); !ok || !near(b, 95.5) || !near(top, 105) {
		t.Errorf("a deep tail: ink %v..%v (ok=%v), want 95.5..105 — below the reach's %v", b, top, ok, 100-10*descentEm)
	}
	if _, top, ok := printInk(run(src(byCode), 9)); !ok || !near(top, 106) {
		t.Errorf("a glyph the program does not have: top %v (ok=%v), want the first glyph's 106", top, ok)
	}
	if _, _, ok := printInk(run(src(byCode), 3)); ok {
		t.Error("a run that paints nothing was given ink")
	}
	one := run(src(byCode), 1)
	one.glyphs[0].code = []byte{1}
	if _, _, ok := printInk(one); ok {
		t.Error("a one-byte code was read as a glyph number")
	}

	// Never: no font, no program, a turned run, a run that is not simply filled.
	plain := src(fontInk{lo: -0.2, hi: 0.7})
	for name, r := range map[string]textRun{
		"no font":    run(nil, 1),
		"no program": run(&runFont{}, 1),
		"turned":     func() textRun { r := run(plain, 1); r.rotated = true; return r }(),
		"stroked":    func() textRun { r := run(plain, 1); r.state.tr = 2; return r }(),
		"hidden":     func() textRun { r := run(plain, 1); r.state.tr = 3; return r }(),
	} {
		if _, _, ok := printInk(r); ok {
			t.Errorf("%s: the run was given ink", name)
		}
	}
}

// TestWhichFontsCodesAreGlyphNumbers: only Identity-H over a CIDFontType2 that maps CIDs to glyphs by identity. Any
// other TrueType font answers for its whole program; anything else has no program this reads.
func TestWhichFontsCodesAreGlyphNumbers(t *testing.T) {
	xt := &model.XRefTable{}
	prog := types.StreamDict{Dict: types.Dict{}}
	desc := types.Dict{"FontFile2": prog}
	cid := func(extra types.Dict) types.Dict {
		d := types.Dict{"Subtype": types.Name("CIDFontType2"), "FontDescriptor": desc}
		for k, v := range extra {
			d[k] = v
		}
		return d
	}
	type0 := func(enc string, kid types.Dict) types.Dict {
		return types.Dict{"Subtype": types.Name("Type0"), "Encoding": types.Name(enc), "DescendantFonts": types.Array{kid}}
	}
	for name, c := range map[string]struct {
		d           types.Dict
		has, byCode bool
	}{
		"Identity-H, no map":       {type0("Identity-H", cid(nil)), true, true},
		"Identity-H, /Identity":    {type0("Identity-H", cid(types.Dict{"CIDToGIDMap": types.Name("Identity")})), true, true},
		"Identity-H, a map stream": {type0("Identity-H", cid(types.Dict{"CIDToGIDMap": prog})), true, false},
		"another CMap":             {type0("UniJIS-UCS2-H", cid(nil)), true, false},
		"Identity-V":               {type0("Identity-V", cid(nil)), true, false},
		"a simple TrueType font":   {types.Dict{"Subtype": types.Name("TrueType"), "FontDescriptor": desc}, true, false},
		"TrueType, not embedded":   {types.Dict{"Subtype": types.Name("TrueType"), "FontDescriptor": types.Dict{}}, false, false},
		"Type 1":                   {types.Dict{"Subtype": types.Name("Type1"), "FontDescriptor": desc}, false, false},
		"a CFF descendant":         {type0("Identity-H", types.Dict{"Subtype": types.Name("CIDFontType0"), "FontDescriptor": desc}), false, false},
	} {
		s := inkSourceFor(xt, c.d)
		if (s != nil) != c.has || (s != nil && s.byCode != c.byCode) {
			t.Errorf("%s: source %v, want has=%v byCode=%v", name, s, c.has, c.byCode)
		}
	}
}

// TestAProgramThatCannotBeReadWholeGivesNothing: cut anywhere, a program either reads to an answer inside what the
// whole one gave or gives none — and never panics. One misread glyph is a letter left outside a redaction.
func TestAProgramThatCannotBeReadWholeGivesNothing(t *testing.T) {
	b, _ := vendoredFace(t, authoringFontFiles[firstAuthoringFace(t)])
	whole := readGlyfInk(b)
	if whole == nil {
		t.Fatal("the control did not read")
	}
	refused := 0
	for n := 0; n < len(b); n += 1 + len(b)/997 {
		got := readGlyfInk(b[:n])
		if got == nil {
			refused++
			continue
		}
		if got.lo < whole.lo-1e-9 || got.hi > whole.hi+1e-9 {
			t.Fatalf("cut at %d of %d: reads %.4f..%.4f, outside the whole program's %.4f..%.4f", n, len(b), got.lo, got.hi, whole.lo, whole.hi)
		}
	}
	if refused == 0 {
		t.Error("no cut was refused: the cuts never reached the glyphs")
	}

	// A glyph made of itself, and one whose part is turned (a two-by-two), are not read — and so the font is not. A
	// part moved and scaled up the page is: 0..55 at half size, moved up 7, is 7..34.5.
	for name, c := range map[string]struct {
		flags uint16
		kid   uint16
		tail  []byte
		ok    bool
	}{
		"itself":                      {0x0002, 0, []byte{0, 7}, false},
		"a turned part":               {0x0002 | 0x0080, 1, []byte{0, 7, 0x40, 0, 0, 0, 0, 0, 0x40, 0}, false},
		"a scaled part":               {0x0002 | 0x0008, 1, []byte{0, 7, 0x20, 0}, true},
		"a moved part":                {0x0002, 1, []byte{0, 7}, true},
		"a part scaled with its move": {0x0002 | 0x0008 | 0x0800, 1, []byte{0, 8, 0x20, 0}, true},
	} {
		comp := make([]byte, 14, 32)
		binary.BigEndian.PutUint16(comp[0:], 0xFFFF) // -1 contours: made of parts
		binary.BigEndian.PutUint16(comp[10:], c.flags)
		binary.BigEndian.PutUint16(comp[12:], c.kid)
		comp = append(comp, c.tail...)
		tri := make([]byte, 10, 32)
		binary.BigEndian.PutUint16(tri[0:], 1)
		tri = append(tri, 0, 2, 0, 0)          // one contour ending at point 2; no instructions
		tri = append(tri, 0x37, 0x37, 0x37)    // three points, short x and y, both positive
		tri = append(tri, 0, 10, 10, 0, 50, 5) // x steps, then y steps: y = 0, 50, 55
		offs := []int{0, len(comp), len(comp) + len(tri)}
		g := glyfReader{glyf: append(append([]byte{}, comp...), tri...), count: 2,
			at: func(i int) int { return offs[i] }, lo: make([]float64, 2), hi: make([]float64, 2), state: make([]byte, 2)}
		if got := g.read(0, 0); got != c.ok {
			t.Errorf("a glyph made of %s: read=%v, want %v", name, got, c.ok)
		}
		if !g.read(1, 0) || g.lo[1] != 0 || g.hi[1] != 55 {
			t.Errorf("%s: the plain glyph beside it reads %v..%v, want 0..55", name, g.lo[1], g.hi[1])
		}
		want := map[string][2]float64{"a scaled part": {7, 34.5}, "a moved part": {7, 62}, "a part scaled with its move": {4, 31.5}}[name]
		if c.ok && (g.lo[0] != want[0] || g.hi[0] != want[1]) {
			t.Errorf("a glyph made of %s reaches %v..%v, want %v", name, g.lo[0], g.hi[0], want)
		}
	}
}

func firstAuthoringFace(t *testing.T) string {
	t.Helper()
	best := ""
	for name := range authoringFontFiles {
		if best == "" || name < best {
			best = name
		}
	}
	if best == "" {
		t.Fatal("no authoring face")
	}
	return best
}

// TestTheReachColumnReadsTheBaselineRunBoxWrote: `test/accuracy/reach.mjs` reads a run's baseline back out of its
// rect, so it carries `runBox`'s two constants. Changed here and not there, the column would measure every word's
// ink from the wrong line and still print numbers.
func TestTheReachColumnReadsTheBaselineRunBoxWrote(t *testing.T) {
	src, err := os.ReadFile("../../test/accuracy/reach.mjs")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`BOX_UP = ([0-9.]+), BOX_DOWN = ([0-9.]+);`).FindSubmatch(src)
	if m == nil {
		t.Fatal("reach.mjs no longer declares BOX_UP and BOX_DOWN")
	}
	up, _ := strconv.ParseFloat(string(m[1]), 64)
	down, _ := strconv.ParseFloat(string(m[2]), 64)
	if up != ascentEm || down != descentEm {
		t.Errorf("reach.mjs reads a baseline %v above a rect's bottom in a box %v tall; runBox writes %v and %v", down, up+down, descentEm, ascentEm+descentEm)
	}
}
