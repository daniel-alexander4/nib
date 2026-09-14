package pdfops

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
	"nib/mdpdf"
)

// Positioned runs — `PLAN-accessibility.md` P08.S02 (`PLAN-text-reflow.md` P03).

func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func helveticaRes() types.Dict {
	return types.Dict{"Font": types.Dict{"F1": types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type1"),
		"BaseFont": types.Name("Helvetica"), "Encoding": types.Name("WinAnsiEncoding")}}}
}

func walkContent(t *testing.T, res types.Dict, content string) []textRun {
	t.Helper()
	w := newRunWalker(widthXRef(t))
	w.walk([]byte(content), res, newRunGState(), 0, map[int]bool{})
	return w.runs
}

func helv(text string) float64 { return mdpdf.CoreWidth(text, "Helvetica", 1000) }

// TestAPDFStringDecodesEveryEscapeForm — the reader's half of what `contentstream` leaves as spans.
func TestAPDFStringDecodesEveryEscapeForm(t *testing.T) {
	for _, c := range []struct{ raw, want string }{
		{`(plain)`, "plain"},
		{`(a\(b\)c)`, "a(b)c"},
		{`(back\\slash)`, `back\slash`},
		{`(\101\102)`, "AB"},
		{`(\0053)`, "\x053"},
		{`(\7)`, "\x07"},
		{`(\n\r\t\b\f)`, "\n\r\t\b\f"},
		{"(x\\\ny)", "xy"},
		{"(x\\\r\ny)", "xy"},
		{"(a\r\nb)", "a\nb"},
		{`(\q)`, "q"},
		{`<48656C6C6F>`, "Hello"},
		{`<48 65 6c>`, "Hel"},
		{`<486>`, "H`"},
		{`<>`, ""},
		{`()`, ""},
	} {
		if got := string(decodePDFString([]byte(c.raw))); got != c.want {
			t.Errorf("%s decodes to %q, want %q", c.raw, got, c.want)
		}
	}
}

// TestToUnicodeReadsCharsRangesAndSurrogates — both producers the corpus measured write `bfchar`
// only; the specification's two `bfrange` forms are driven here, where no generated document reaches.
func TestToUnicodeReadsCharsRangesAndSurrogates(t *testing.T) {
	cm := parseToUnicode([]byte(`/CIDInit /ProcSet findresource begin
12 dict begin begincmap
1 begincodespacerange <0000> <FFFF> endcodespacerange
3 beginbfchar
<0001> <005A>
<0002> <00660069>
<0003> <D83DDE00>
endbfchar
2 beginbfrange
<0010> <0012> <0041>
<0020> <0021> [<0061> <0062>]
endbfrange
endcmap`))
	for code, want := range map[string]string{
		"\x00\x01": "Z", "\x00\x02": "fi", "\x00\x03": "\U0001F600",
		"\x00\x10": "A", "\x00\x11": "B", "\x00\x12": "C",
		"\x00\x20": "a", "\x00\x21": "b",
	} {
		if got, ok := cm[code]; !ok || got != want {
			t.Errorf("code % X maps to %q (present %v), want %q", code, got, ok, want)
		}
	}
	if _, ok := cm["\x00\x13"]; ok {
		t.Error("a code past the incrementing range's end was mapped")
	}
	huge := parseToUnicode([]byte("1 beginbfrange <000000> <FFFFFF> <0041> endbfrange"))
	if len(huge) != 0 {
		t.Errorf("a range of 2^24 codes expanded to %d entries — one line in a document allocating without limit", len(huge))
	}
}

// TestTheTextStateMachine — every operator that moves or measures text, each driven so a reader that
// ignored it would put a run in a different place or give it a different width.
func TestTheTextStateMachine(t *testing.T) {
	res := helveticaRes()
	type want struct {
		text     string
		x, y     float64
		size     float64
		width    float64
		decoded  bool
		widthSrc widthSource
	}
	for _, c := range []struct {
		name    string
		content string
		want    []want
	}{
		{"Td positions, Tf sizes, widths from the core door", "BT /F1 10 Tf 100 700 Td (AB) Tj ET",
			[]want{{"AB", 100, 700, 10, helv("AB") / 1000 * 10, true, widthFromStd14}}},
		{"the font survives ET and BT", "BT /F1 10 Tf ET BT 5 5 Td (A) Tj ET",
			[]want{{"A", 5, 5, 10, helv("A") / 100, true, widthFromStd14}}},
		{"Q restores the font away", "q BT /F1 10 Tf ET Q BT (A) Tj ET",
			[]want{{"", 0, 0, 0, 0, false, widthNone}}},
		{"cm scales position, size and width", "2 0 0 2 10 20 cm BT /F1 10 Tf 5 5 Td (A) Tj ET",
			[]want{{"A", 20, 30, 20, helv("A") / 100 * 2, true, widthFromStd14}}},
		{"TD sets the leading T* uses", "BT /F1 10 Tf 0 700 Td (A) Tj 0 -12 TD (B) Tj T* (C) Tj ET",
			[]want{{"A", 0, 700, 10, helv("A") / 100, true, widthFromStd14},
				{"B", 0, 688, 10, helv("B") / 100, true, widthFromStd14},
				{"C", 0, 676, 10, helv("C") / 100, true, widthFromStd14}}},
		{"Tm sets the matrix outright", "BT /F1 10 Tf 9 9 Td 1 0 0 1 50 60 Tm (A) Tj ET",
			[]want{{"A", 50, 60, 10, helv("A") / 100, true, widthFromStd14}}},
		{"Tc, Tw and Tz each change the advance", "BT /F1 10 Tf 2 Tc 3 Tw 50 Tz ( A) Tj ET",
			[]want{{" A", 0, 0, 10, (helv(" ")/100+2+3)*0.5 + (helv("A")/100+2)*0.5, true, widthFromStd14}}},
		{"TJ kerning is applied, in one run", "BT /F1 10 Tf [(A) -500 (B)] TJ ET",
			[]want{{"AB", 0, 0, 10, helv("A")/100 + 5 + helv("B")/100, true, widthFromStd14}}},
		{"' and \" move to the next line, and \" sets its spacing", "BT /F1 10 Tf 12 TL 0 700 Td (A) Tj (B) ' 1 2 (C) \" ET",
			[]want{{"A", 0, 700, 10, helv("A") / 100, true, widthFromStd14},
				{"B", 0, 688, 10, helv("B") / 100, true, widthFromStd14},
				{"C", 0, 676, 10, helv("C")/100 + 2, true, widthFromStd14}}},
		{"Ts raises the origin", "BT /F1 10 Tf 0 700 Td 3 Ts (A) Tj ET",
			[]want{{"A", 0, 703, 10, helv("A") / 100, true, widthFromStd14}}},
		{"an unknown font resource is not guessed", "BT /F9 10 Tf (A) Tj ET",
			[]want{{"", 0, 0, 10, 0, false, widthNone}}},
	} {
		runs := walkContent(t, res, c.content)
		if len(runs) != len(c.want) {
			t.Errorf("%s: %d run(s), want %d", c.name, len(runs), len(c.want))
			continue
		}
		for i, w := range c.want {
			r := runs[i]
			if r.text != w.text || !approxEqual(r.x, w.x) || !approxEqual(r.y, w.y) || !approxEqual(r.size, w.size) ||
				!approxEqual(r.width, w.width) || r.decoded != w.decoded || r.widthSrc != w.widthSrc {
				t.Errorf("%s: run %d = {%q x=%v y=%v size=%v width=%v decoded=%v src=%q}, want {%q x=%v y=%v size=%v width=%v decoded=%v src=%q}",
					c.name, i, r.text, r.x, r.y, r.size, r.width, r.decoded, r.widthSrc,
					w.text, w.x, w.y, w.size, w.width, w.decoded, w.widthSrc)
			}
		}
	}
}

// TestAFormXObjectIsWalkedAtItsMatrixAndASelfDrawingFormEnds.
func TestAFormXObjectIsWalkedAtItsMatrixAndASelfDrawingFormEnds(t *testing.T) {
	form := types.StreamDict{Dict: types.Dict{"Type": types.Name("XObject"), "Subtype": types.Name("Form"),
		"Matrix": nums(1, 0, 0, 1, 100, 0), "Resources": helveticaRes()},
		Content: []byte("BT /F1 10 Tf 0 0 Td (A) Tj ET")}
	runs := walkContent(t, types.Dict{"XObject": types.Dict{"Fm0": form}}, "q 1 0 0 1 0 50 cm /Fm0 Do Q")
	if len(runs) != 1 || runs[0].text != "A" || runs[0].x != 100 || runs[0].y != 50 {
		t.Fatalf("the form's run is %+v, want text A at (100, 50) — the form's /Matrix composed with the page's cm", runs)
	}

	// A form that draws itself: Go maps can hold themselves, and so can a PDF through direct objects.
	selfRes := helveticaRes()
	self := types.StreamDict{Dict: types.Dict{"Subtype": types.Name("Form"), "Resources": selfRes},
		Content: []byte("BT /F1 10 Tf (x) Tj ET /Fm0 Do")}
	selfRes["XObject"] = types.Dict{"Fm0": self}
	runs = walkContent(t, selfRes, "/Fm0 Do")
	if len(runs) != maxFormDepth {
		t.Errorf("a self-drawing form produced %d run(s), want the depth ceiling %d", len(runs), maxFormDepth)
	}
}

// TestACIDFontIsSplitIntoTwoByteCodesAndDecodedThroughToUnicode — nib's own Markdown output's shape.
func TestACIDFontIsSplitIntoTwoByteCodesAndDecodedThroughToUnicode(t *testing.T) {
	cid := types.Dict{"Subtype": types.Name("Type0"), "BaseFont": types.Name("X"), "Encoding": types.Name("Identity-H"),
		"DescendantFonts": types.Array{types.Dict{"Subtype": types.Name("CIDFontType2"),
			"W": types.Array{types.Integer(1), nums(500, 600)}}},
		"ToUnicode": types.StreamDict{Dict: types.Dict{}, Content: []byte("2 beginbfchar <0001> <005A> <0002> <0042> endbfchar")}}
	runs := walkContent(t, types.Dict{"Font": types.Dict{"F2": cid}}, "BT /F2 10 Tf <00010002> Tj ET")
	if len(runs) != 1 {
		t.Fatalf("%d runs", len(runs))
	}
	r := runs[0]
	if r.text != "ZB" || r.codes != 2 || !approxEqual(r.width, 11) || r.widthSrc != widthFromW || !r.decoded {
		t.Errorf("run = %+v, want text ZB from 2 two-byte codes, width 11 from W", r)
	}
	// The same bytes under a CMap nib does not parse: nothing is guessed.
	other := cid.Clone().(types.Dict)
	other["Encoding"] = types.Name("UniJIS-UCS2-H")
	runs = walkContent(t, types.Dict{"Font": types.Dict{"F2": other}}, "BT /F2 10 Tf <00010002> Tj ET")
	if len(runs) != 1 || runs[0].decoded || runs[0].widthSrc != widthNone {
		t.Errorf("a Type0 font under an unparsed CMap read as %+v — its code lengths are unknown, so text and width must both say so", runs)
	}
	// And a map whose keys happen to be one byte long must not make those bytes readable: the code
	// length is still unknown, and a lookup that succeeds by accident is a guess that looks like text.
	oneByte := other.Clone().(types.Dict)
	oneByte["ToUnicode"] = types.StreamDict{Dict: types.Dict{}, Content: []byte("2 beginbfchar <00> <0078> <01> <005A> endbfchar")}
	runs = walkContent(t, types.Dict{"Font": types.Dict{"F2": oneByte}}, "BT /F2 10 Tf <0001> Tj ET")
	if len(runs) != 1 || runs[0].decoded || runs[0].text != "" {
		t.Errorf("an unparsed CMap with one-byte ToUnicode keys read as %+v — the bytes were decoded under a code length nobody stated", runs)
	}
}

// TestASimpleFontIsDecodedOnlyThroughTheTableItNames — the three encodings the reader carries, each at a
// code where it differs from the others or from ASCII, and a code each leaves undefined.
func TestASimpleFontIsDecodedOnlyThroughTheTableItNames(t *testing.T) {
	font := func(base, enc string) types.Dict {
		d := types.Dict{"Subtype": types.Name("Type1"), "BaseFont": types.Name(base)}
		if enc != "" {
			d["Encoding"] = types.Name(enc)
		}
		return d
	}
	for _, c := range []struct {
		name string
		font types.Dict
		code byte
		want string
		ok   bool
	}{
		{"StandardEncoding's 0x27 is a right quote", font("Helvetica", ""), 0x27, "’", true},
		{"StandardEncoding's 0x60 is a left quote", font("Helvetica", "StandardEncoding"), 0x60, "‘", true},
		{"StandardEncoding past printable ASCII is not guessed", font("Helvetica", ""), 0xE1, "", false},
		{"WinAnsi 0x80 is the euro", font("Helvetica", "WinAnsiEncoding"), 0x80, "€", true},
		{"WinAnsi 0x27 is an apostrophe, not a quote", font("Helvetica", "WinAnsiEncoding"), 0x27, "'", true},
		{"WinAnsi leaves 0x81 undefined", font("Helvetica", "WinAnsiEncoding"), 0x81, "", false},
		{"MacRoman 0x8A is a-umlaut", font("Helvetica", "MacRomanEncoding"), 0x8A, "ä", true},
		{"a control code is not text", font("Helvetica", "WinAnsiEncoding"), 0x07, "", false},
		{"Symbol with no /Encoding is not read as Latin", font("Symbol", ""), 'a', "", false},
		{"a non-core font with no /Encoding is not guessed", font("Arial", ""), 'a', "", false},
	} {
		got, ok := loadRunFont(widthXRef(t), c.font).textFor([]byte{c.code})
		if got != c.want || ok != c.ok {
			t.Errorf("%s: code %#x → %q (%v), want %q (%v)", c.name, c.code, got, ok, c.want, c.ok)
		}
	}
}

// TestAnImageOnlyPageReturnsNoRunsAndSaysSo — P03's second exit criterion, structurally.
func TestAnImageOnlyPageReturnsNoRunsAndSaysSo(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.Black)
	var png4 bytes.Buffer
	if err := png.Encode(&png4, img); err != nil {
		t.Fatal(err)
	}
	scan, err := mdpdf.ImageToPDF(png4.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	text, err := testpdf.Text("words on a page")
	if err != nil {
		t.Fatal(err)
	}
	read := func(pdf []byte) pageRuns {
		ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
		if rerr != nil {
			t.Fatal(rerr)
		}
		pr, perr := readPageRuns(ctx, 1)
		if perr != nil {
			t.Fatal(perr)
		}
		return pr
	}
	// Stimulus first: the text page must read as text, or "no runs" below means nothing.
	tp := read(text)
	if tp.noText || len(tp.runs) == 0 || !strings.Contains(joinRunText(tp.runs), "words") {
		t.Fatalf("the text page read as noText=%v with runs %+v", tp.noText, tp.runs)
	}
	sp := read(scan)
	if !sp.noText || len(sp.runs) != 0 {
		t.Errorf("an image-only page read as noText=%v with %d run(s), want noText and none", sp.noText, len(sp.runs))
	}
}

func joinRunText(runs []textRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.text)
	}
	return b.String()
}

// TestTheRunReaderIsContained — reflow D6: the containment is made to FIRE, and every truncation of a
// real page is read without reaching it.
func TestTheRunReaderIsContained(t *testing.T) {
	_, err := containRunRead(7, func() (pageRuns, error) {
		var m map[string]int
		m["boom"] = 1 // a panic from anywhere beneath the walk
		return pageRuns{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "page 7") {
		t.Fatalf("a panic under the reader returned err=%v — the containment did not fire, or does not name the page", err)
	}

	md, err := ConvertDocToPDF([]byte("# Heading\n\nA paragraph with **bold**.\n\n- one\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(md), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, _, attrs, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	content, err := ctx.PageContent(d, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) < 200 {
		t.Fatalf("the page content is %d bytes, too short to truncate meaningfully", len(content))
	}
	for n := 0; n <= len(content); n++ {
		if _, perr := containRunRead(1, func() (pageRuns, error) {
			w := newRunWalker(ctx.XRefTable)
			w.walk(content[:n], attrs.Resources, newRunGState(), 0, map[int]bool{})
			return pageRuns{runs: w.runs}, nil
		}); perr != nil {
			t.Fatalf("the page truncated at byte %d of %d reached the containment: %v", n, len(content), perr)
		}
	}
}

// TestRunsOverTheCorpusAreDecodedAndMeasured — the instrument row's observable as a guard: every run
// the generated corpus draws is decoded and measured from the document, and the corpus draws some.
func TestRunsOverTheCorpusAreDecodedAndMeasured(t *testing.T) {
	for _, doc := range runCorpus(t) {
		ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(doc.pdf), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%s: %v", doc.name, err)
		}
		total := 0
		for p := 1; p <= ctx.PageCount; p++ {
			pr, perr := readPageRuns(ctx, p)
			if perr != nil {
				t.Fatalf("%s page %d: %v", doc.name, p, perr)
			}
			for _, r := range pr.runs {
				total++
				if !r.decoded {
					t.Errorf("%s page %d: run %q in %s was not fully decoded", doc.name, p, r.text, r.baseFont)
				}
				if r.widthSrc == widthNone {
					t.Errorf("%s page %d: run %q in %s has no measured width", doc.name, p, r.text, r.baseFont)
				}
			}
		}
		if total == 0 {
			t.Errorf("%s drew no runs, so it measured nothing", doc.name)
		}
	}
}

type runCorpusDoc struct {
	name string
	pdf  []byte
}

// runCorpus is S02's generated corpus: nib's own Markdown (Type0, Identity-H, literal strings), a
// core-font page (WinAnsi), and LibreOffice when present (TrueType, hex strings, one-byte ToUnicode).
func runCorpus(t *testing.T) []runCorpusDoc {
	t.Helper()
	md, err := ConvertDocToPDF([]byte("# A heading\n\nA paragraph that is long enough to wrap onto a second line when it is rendered at the default measure of the converter.\n\n- first item\n- second item with **bold** and *italic*\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	core, err := testpdf.Text("A core-font page", "and a second one")
	if err != nil {
		t.Fatal(err)
	}
	docs := []runCorpusDoc{{"converted Markdown", md}, {"core-font pages", core}}
	if LibreOfficeAvailable() {
		lo, lerr := ConvertOfficeToPDF([]byte("A heading line\n\nA paragraph of ordinary prose that wraps across the page width so the converter emits several lines.\n"), "txt")
		if lerr != nil {
			t.Fatalf("LibreOffice is present and could not convert: %v", lerr)
		}
		docs = append(docs, runCorpusDoc{"LibreOffice text", lo})
	} else {
		t.Log("NOTE (a narrower corpus, not a pass): LibreOffice is absent")
	}
	return docs
}
