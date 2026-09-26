package uacheck

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The P07 phase-close review's findings in the glyph door (`glyphs.go`, `type3.go`) and the two width reads it routes,
// each measured on veraPDF 1.30.2 before the fix and asked again below wherever veraPDF is present.

// veraAsk runs veraPDF over docs and returns, per document, clause → "failed" | "passed" | "none" (no subject), or nil
// where veraPDF produced no validation report at all. It returns nil when veraPDF is absent.
func veraAsk(t *testing.T, docs [][]byte) []map[string]string {
	t.Helper()
	vp := veraPDFPath()
	if vp == "" {
		t.Log("NOTE (not a pass): veraPDF is absent, so its verdicts on these documents are unchecked in this run")
		return nil
	}
	dir := t.TempDir()
	var files []string
	for i, b := range docs {
		files = append(files, filepath.Join(dir, fmt.Sprintf("w%02d.pdf", i)))
		if err := os.WriteFile(files[i], b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1", "--passed"}, files...)...).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v", err)
	}
	res := make([]map[string]string, len(docs))
	seen := 0
	for _, j := range rep.Jobs {
		for i, f := range files {
			if filepath.Base(j.Item.Name) != filepath.Base(f) {
				continue
			}
			seen++
			if len(j.Report.Rules) == 0 {
				continue // no report
			}
			res[i] = map[string]string{}
			for _, r := range j.Report.Rules {
				st := r.Status
				if st == "passed" && r.Passed == "0" && r.Failed == "0" {
					st = "none"
				}
				res[i][r.Clause+" t"+r.Test] = st
			}
		}
	}
	if seen != len(files) {
		t.Fatalf("veraPDF returned %d job(s) for %d document(s)", seen, len(files))
	}
	return res
}

// TestAWidthReadVeraPDFThrowsOnReportsNothing — R2-4, measured: `getIntegerKey` is null for a value that is neither a
// number nor a string, and veraPDF throws on it — a /FirstChar or /LastChar (either, whatever the code), or a /W
// entry's opening CID — where it reads the width: a glyph drawn, in a font whose program it parsed. A string, no
// program and an empty string each report. pdfcpu refuses the first shape's document and drops the second's font, so
// the documents are built valid and the entry written into nib's reading afterwards; veraPDF is asked about the bytes
// with the entry in place.
func TestAWidthReadVeraPDFThrowsOnReportsNothing(t *testing.T) {
	std := ttProgram(sub31, sub10)
	win := "/Encoding /WinAnsiEncoding"
	tt := func(w, show string, prog []byte) []byte {
		return glyphDoc(mfDict(w, win), show, ttObjects("/Flags 32", "FontFile2", prog, ""))
	}
	cidShow := func(s string) string { return "BT /F0 12 Tf 10 10 Td " + s + " ET" }
	valid := "/FirstChar 65 /LastChar 67 /Widths [500 500 500]"
	cases := []struct {
		name    string
		bytes   []byte // what veraPDF is asked about
		open    []byte // what nib opens, before `obj`'s `key` is written
		obj     int
		key     string
		val     types.Object
		throws  string // the door's answer: "" or the words it holds
		refuses string // a metric clause's words where the door is silent but nib does not answer
	}{
		{"/LastChar a name", tt("/FirstChar 65 /LastChar /X /Widths [500 500 500]", "(A) Tj", std), tt(valid, "(A) Tj", std), 10, "LastChar", types.Name("X"), "/FirstChar or /LastChar", ""},
		{"/FirstChar a name", tt("/FirstChar /X /LastChar 67 /Widths [500 500 500]", "(A) Tj", std), tt(valid, "(A) Tj", std), 10, "FirstChar", types.Name("X"), "/FirstChar or /LastChar", ""},
		{"/LastChar a boolean, the code below /FirstChar", tt("/FirstChar 65 /LastChar true /Widths [500 500 500]", "(@) Tj", std), tt(valid, "(@) Tj", std), 10, "LastChar", types.Boolean(true), "/FirstChar or /LastChar", ""},
		{"/LastChar a name, drawn invisibly", tt("/FirstChar 65 /LastChar /X /Widths [500 500 500]", "3 Tr (A) Tj", std), tt(valid, "3 Tr (A) Tj", std), 10, "LastChar", types.Name("X"), "/FirstChar or /LastChar", ""},
		{"/LastChar a string (control)", tt("/FirstChar 65 /LastChar (s) /Widths [500 500 500]", "(A) Tj", std), tt(valid, "(A) Tj", std), 10, "LastChar", types.StringLiteral("s"), "", "is a string"},
		{"/LastChar a name, no program (control)", tt("/FirstChar 65 /LastChar /X /Widths [500 500 500]", "(A) Tj", nil), tt(valid, "(A) Tj", nil), 10, "LastChar", types.Name("X"), "", ""},
		{"/LastChar a name, an empty string (control)", tt("/FirstChar 65 /LastChar /X /Widths [500 500 500]", "() Tj", std), tt(valid, "() Tj", std), 10, "LastChar", types.Name("X"), "", ""},
		{"/LastChar a name, no /Widths (control)", tt("/FirstChar 65 /LastChar /X", "(A) Tj", std), tt("/FirstChar 65 /LastChar 67", "(A) Tj", std), 10, "LastChar", types.Name("X"), "", ""},
		{"/W opening with a name", cidFontDoc("/W [/x [500]] /CIDToGIDMap /Identity", cidShow("<0021> Tj"), std, nil),
			cidFontDoc("/W [5 [500]] /CIDToGIDMap /Identity", cidShow("<0021> Tj"), std, nil), 11, "W", types.Array{types.Name("x"), types.Array{types.Integer(500)}}, "/W array", ""},
		{"/W with a later entry opening with a name, drawn invisibly", cidFontDoc("/W [5 [500] /x [500]] /CIDToGIDMap /Identity", cidShow("3 Tr <0021> Tj"), std, nil),
			cidFontDoc("/W [5 [500]] /CIDToGIDMap /Identity", cidShow("3 Tr <0021> Tj"), std, nil), 11, "W",
			types.Array{types.Integer(5), types.Array{types.Integer(500)}, types.Name("x"), types.Array{types.Integer(500)}}, "/W array", ""},
		{"/W opening with a string (control)", cidFontDoc("/W [(x) [500]] /CIDToGIDMap /Identity", cidShow("<0021> Tj"), std, nil),
			cidFontDoc("/W [5 [500]] /CIDToGIDMap /Identity", cidShow("<0021> Tj"), std, nil), 11, "W", types.Array{types.StringLiteral("x"), types.Array{types.Integer(500)}}, "", "with a string"},
		{"/W opening with a name, no program (control)", cidFontDoc("/W [/x [500]] /CIDToGIDMap /Identity", cidShow("<0021> Tj"), nil, nil),
			cidFontDoc("/W [5 [500]] /CIDToGIDMap /Identity", cidShow("<0021> Tj"), nil, nil), 11, "W", types.Array{types.Name("x"), types.Array{types.Integer(500)}}, "", ""},
		{"/W opening with a name, an empty string (control)", cidFontDoc("/W [/x [500]] /CIDToGIDMap /Identity", cidShow("() Tj"), std, nil),
			cidFontDoc("/W [5 [500]] /CIDToGIDMap /Identity", cidShow("() Tj"), std, nil), 11, "W", types.Array{types.Name("x"), types.Array{types.Integer(500)}}, "", ""},
	}
	var docs [][]byte
	for _, c := range cases {
		d := openMutated(t, c.open, func(d *Document, _ types.Dict) {
			dict, ok := d.Ctx.XRefTable.Table[c.obj].Object.(types.Dict)
			if !ok {
				t.Fatalf("%s: object %d is not a dictionary", c.name, c.obj)
			}
			dict[c.key] = c.val
		})
		glyphs, _ := d.glyphsDrawn()
		why := d.reportsNothing()
		switch {
		case c.throws != "" && len(glyphs) == 0:
			t.Errorf("%s: setup: no glyph drawn, so the door was not asked", c.name)
		case c.throws != "" && (!strings.Contains(why, "reports nothing") || !strings.Contains(why, c.throws)):
			t.Errorf("%s: the door answers %q, want veraPDF reporting nothing naming %q", c.name, why, c.throws)
		case c.throws == "" && why != "":
			t.Errorf("%s: the door answers %q, but veraPDF reports on this document", c.name, why)
		}
		if c.refuses != "" {
			d.trueTypeFonts()
			if m := d.metricsOf(glyphs[0]); m.known || !strings.Contains(m.why, c.refuses) {
				t.Errorf("%s: the metric door answers %+v, want a refusal naming %q", c.name, m, c.refuses)
			}
		}
		docs = append(docs, c.bytes)
	}
	for i, got := range veraAsk(t, docs) {
		if reports := got != nil; reports != (cases[i].throws == "") {
			t.Errorf("%s: veraPDF reports on it: %v — the measurement this door rests on has moved", cases[i].name, reports)
		}
	}
}

// TestAMalformedCMapsWModeIsRefused — R2-5, measured: veraPDF reads a CMap its parser throws on as an EMPTY one, WMode 0,
// whatever `/WMode 1 def` preceded the throw; nib's own scanner read that `def`, a false pass under a dictionary /WMode 1
// and a false fail under none. Whether the parser throws is `fontcode`'s question, and a CMap it finds malformed refuses
// where the program declares a /WMode other than 0 — the one case the two readings differ.
func TestAMalformedCMapsWModeIsRefused(t *testing.T) {
	sys := "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >>"
	cases := []struct {
		name, vera string
		pdf        []byte
		refused    bool
	}{
		{"/WMode 1 in both, then a cidchar of the wrong kind", "failed", buildPDF(type0Doc("20 0 R", sys, map[int]string{20: cmapStream("/WMode 1 "+sys, "Cust", "/WMode 1 def 1 begincidchar <0021> (x) endcidchar")})), true},
		{"/WMode 1 in the program only, then a cidchar of the wrong kind", "passed", buildPDF(type0Doc("20 0 R", sys, map[int]string{20: cmapStream(sys, "Cust", "/WMode 1 def 1 begincidchar <0021> (x) endcidchar")})), true},
		{"/WMode 1 in both, then a bfchar of the wrong kind", "failed", buildPDF(type0Doc("20 0 R", sys, map[int]string{20: cmapStream("/WMode 1 "+sys, "Cust", "/WMode 1 def 1 beginbfchar <0021> 5 endbfchar")})), true},
		// Malformed, but declaring no /WMode: both readings are 0, so the answer stands.
		{"no /WMode anywhere, a cidchar of the wrong kind (control)", "passed", buildPDF(type0Doc("20 0 R", sys, map[int]string{20: cmapStream(sys, "Cust", "1 begincidchar <0021> (x) endcidchar")})), false},
		{"/WMode 1 in both, well formed (control)", "passed", buildPDF(type0Doc("20 0 R", sys, map[int]string{20: cmapStream("/WMode 1 "+sys, "Cust", "/WMode 1 def 1 begincidchar <0021> 5 endcidchar")})), false},
		// RR1-4: a dictionary /WMode that is neither 0 nor the program's fails under BOTH readings, so nib fails it.
		{"/WMode 1 in the dictionary, 2 in the program, then a cidchar of the wrong kind", "failed", buildPDF(type0Doc("20 0 R", sys, map[int]string{20: cmapStream("/WMode 1 "+sys, "Cust", "/WMode 2 def 1 begincidchar <0021> (x) endcidchar")})), false},
	}
	var docs [][]byte
	for _, c := range cases {
		got := verdictOf(t, c.pdf, "7.21.3.3 t2")
		switch {
		case c.refused && (got.Verdict != CannotCheck || !strings.Contains(got.Why, "wrong kind")):
			t.Errorf("%s: 7.21.3.3 t2 reports %v (%s), want CannotCheck naming the malformed CMap", c.name, got.Verdict, got.Why)
		case !c.refused && c.vera == "failed" && got.Verdict != Fail:
			t.Errorf("%s: 7.21.3.3 t2 reports %v (%s), want Fail — both of veraPDF's readings fail it", c.name, got.Verdict, got.Why)
		case !c.refused && c.vera == "passed" && got.Verdict != Pass:
			t.Errorf("%s: 7.21.3.3 t2 reports %v (%s), want Pass", c.name, got.Verdict, got.Why)
		}
		docs = append(docs, c.pdf)
	}
	for i, got := range veraAsk(t, docs) {
		if got["7.21.3.3 t2"] != cases[i].vera {
			t.Errorf("%s: veraPDF now says %q, measured %q", cases[i].name, got["7.21.3.3 t2"], cases[i].vera)
		}
	}
}

// sharedToUnicodeDoc draws code 0x41 in n TrueType fonts that share one /ToUnicode (object 30) whose dictionary uses a
// second (31), which maps it.
func sharedToUnicodeDoc(n int) []byte {
	objs := ttObjects("/Flags 32", "FontFile2", ttProgram(sub31, sub10), "")
	var fonts, content strings.Builder
	for i := 1; i < n; i++ {
		fmt.Fprintf(&fonts, "/F%d %d 0 R ", i, 100+i)
		// Each its own face: pdfcpu merges identical font dictionaries at open, which would leave one font.
		objs[100+i] = strings.Replace(mfDict("/FirstChar 65 /LastChar 65 /Widths [500]", "/Encoding /WinAnsiEncoding /ToUnicode 30 0 R"),
			"/BaseFont /Probe", fmt.Sprintf("/BaseFont /Probe%d", i), 1)
	}
	for i := 0; i < n; i++ {
		fmt.Fprintf(&content, "/F%d 12 Tf (A) Tj ", i)
	}
	objs[30] = toUni("1 beginbfchar <42> <0042> endbfchar")
	objs[30] = strings.Replace(objs[30], "<< ", "<< /UseCMap 31 0 R ", 1)
	objs[31] = toUni("1 beginbfchar <41> <0058> endbfchar")
	return glyphPage("BT 10 10 Td "+content.String()+"ET", fonts.String(), "",
		mfDict("/FirstChar 65 /LastChar 65 /Widths [500]", "/Encoding /WinAnsiEncoding /ToUnicode 30 0 R"), objs)
}

// TestAToUnicodeChainIsReadOncePerStream — R2-2: the /UseCMap chain was walked, and every hop's entries copied, once per
// FONT. Fonts sharing a stream now share its chain: two streams read for forty fonts, and every font's code still
// answers from the used CMap.
func TestAToUnicodeChainIsReadOncePerStream(t *testing.T) {
	for _, n := range []int{1, 40} {
		d, err := open(sharedToUnicodeDoc(n))
		if err != nil {
			t.Fatal(err)
		}
		glyphs, why := d.glyphsDrawn()
		if why != "" || len(glyphs) != n {
			t.Fatalf("%d fonts: setup: %d glyphs drawn (%s), want one per font", n, len(glyphs), why)
		}
		for _, g := range glyphs {
			if s, st, why := d.toUnicode(g); st != uniMapped || s != "X" {
				t.Errorf("%d fonts: %s maps 0x41 to %q (%v, %s), want the used CMap's X", n, g.font.name, s, st, why)
			}
		}
		if d.toUnicodeHops != 2 {
			t.Errorf("%d fonts: %d /ToUnicode streams read, want 2 — the chain is read per stream, not per font", n, d.toUnicodeHops)
		}
	}
}

// TestAToUnicodeLoopIsFoundWhereItLoops — R2-6: the walk's loop check was keyed by a `*StreamDict` pdfcpu builds afresh
// on every dereference, so it never matched and a two-stream loop ran to the 64-stream ceiling. pdfcpu's reader never
// hands nib such a loop, so it is written into nib's reading after the open.
func TestAToUnicodeLoopIsFoundWhereItLoops(t *testing.T) {
	for _, loop := range []bool{false, true} {
		d := openMutated(t, sharedToUnicodeDoc(1), func(d *Document, _ types.Dict) {
			if loop {
				sd, ok := d.Ctx.XRefTable.Table[31].Object.(types.StreamDict)
				if !ok {
					t.Fatal("setup: object 31 is not a stream")
				}
				sd.Dict["UseCMap"] = *types.NewIndirectRef(30, 0)
			}
		})
		glyphs, _ := d.glyphsDrawn()
		if len(glyphs) != 1 {
			t.Fatalf("setup: %d glyphs drawn", len(glyphs))
		}
		_, st, why := d.toUnicode(glyphs[0])
		switch {
		case loop && (st != uniUnknown || !strings.Contains(why, "loops")):
			t.Errorf("a /UseCMap loop reads as %v (%s), want a refusal naming the loop", st, why)
		case !loop && st != uniMapped:
			t.Errorf("the chain without the loop reads as %v (%s)", st, why)
		case d.toUnicodeHops != 2:
			t.Errorf("loop %v: %d streams read, want 2 — the loop is found where it closes, not at the ceiling", loop, d.toUnicodeHops)
		}
	}
}

// TestATypeZeroFontsChainIsReadOncePerFont — R2-7: the glyph door walked a Type 0 font's CMap chain again for every
// glyph, in the throw test and in the /ToUnicode fallback. Two hundred codes of one font walk it once.
func TestATypeZeroFontsChainIsReadOncePerFont(t *testing.T) {
	var show strings.Builder
	for c := 0x21; c < 0x21+200; c++ {
		fmt.Fprintf(&show, "<%04X> Tj ", c)
	}
	pdf := cidCMapDoc(cidCMap("", "1 begincidrange <0000> <FFFF> 0 endcidrange"), "/DW 500 /CIDToGIDMap /Identity",
		"BT /F0 12 Tf 10 10 Td "+show.String()+"ET", ttProgram(sub31, sub10), nil)
	d, err := open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, why := d.glyphsDrawn()
	if why != "" || len(glyphs) != 200 {
		t.Fatalf("setup: %d glyphs (%s), want 200", len(glyphs), why)
	}
	if d.reportsNothing() != "" {
		t.Fatal("setup: the door refused the document")
	}
	for _, g := range glyphs {
		d.toUnicode(g) // no /ToUnicode: every glyph takes the Type 0 fallback, which reads the chain
	}
	if d.fontChainReads != 1 {
		t.Errorf("the font's CMap chain was walked %d times for 200 glyphs, want once", d.fontChainReads)
	}
}

// TestAToUnicodeChainReachedDeepKeepsTheCeiling — R2-2's memo keeps a stream's chain, and the 64-stream ceiling counts
// from the FONT: a chain of ten read shallow from one font is memoised, and reached again further down another font's
// chain it may be past the ceiling there — a memo hit must answer exactly as a fresh walk does, at the boundary too:
// 64 streams read, 65 refuse.
func TestAToUnicodeChainReachedDeepKeepsTheCeiling(t *testing.T) {
	for _, yLen := range []int{54, 55} {
		for _, shared := range []bool{false, true} {
			objs := ttObjects("/Flags 32", "FontFile2", ttProgram(sub31, sub10), "")
			link := func(next int) string {
				s := toUni("1 beginbfchar <41> <0058> endbfchar")
				if next == 0 {
					return s
				}
				return strings.Replace(s, "<< ", fmt.Sprintf("<< /UseCMap %d 0 R ", next), 1)
			}
			for i := 0; i < 10; i++ { // X: 200..209, ten streams
				next := 201 + i
				if i == 9 {
					next = 0
				}
				objs[200+i] = link(next)
			}
			for i := 0; i < yLen; i++ { // Y: 300.., the last using X's first
				next := 301 + i
				if i == yLen-1 {
					next = 200
				}
				objs[300+i] = link(next)
			}
			// F0 reads X from its head first when `shared`, so F1 meets X as a memo hit; otherwise X is walked fresh.
			x := "/ToUnicode 200 0 R"
			if !shared {
				x = ""
			}
			objs[101] = strings.Replace(mfDict("/FirstChar 65 /LastChar 65 /Widths [500]", "/Encoding /WinAnsiEncoding /ToUnicode 300 0 R"), "/BaseFont /Probe", "/BaseFont /ProbeY", 1)
			pdf := glyphPage("BT 10 10 Td /F0 12 Tf (A) Tj /F1 12 Tf (A) Tj ET", "/F1 101 0 R", "",
				mfDict("/FirstChar 65 /LastChar 65 /Widths [500]", "/Encoding /WinAnsiEncoding "+x), objs)
			d, err := open(pdf)
			if err != nil {
				t.Fatal(err)
			}
			glyphs, why := d.glyphsDrawn()
			if why != "" || len(glyphs) != 2 || glyphs[1].font.name != "/F1" {
				t.Fatalf("setup: %d glyphs (%s), want F0's then F1's", len(glyphs), why)
			}
			if _, st, why := d.toUnicode(glyphs[0]); shared && st != uniMapped {
				t.Errorf("Y=%d: the ten-stream chain read shallow: %v (%s), want mapped", yLen, st, why)
			}
			_, st, why := d.toUnicode(glyphs[1])
			switch total := yLen + 10; {
			case total <= maxUseCMapChain && st != uniMapped:
				t.Errorf("a %d-stream chain (memo hit %v): %v (%s), want mapped", total, shared, st, why)
			case total > maxUseCMapChain && (st != uniUnknown || !strings.Contains(why, "runs past")):
				t.Errorf("a %d-stream chain (memo hit %v): %v (%s), want a refusal naming the ceiling", total, shared, st, why)
			}
		}
	}
}

// TestAnUnreadFontDrawsNoGlyphWithAnEmptyString — R2-8: a font nib cannot cut is asked only whether a string shows
// anything, which veraPDF answers by the string's decoded bytes: an empty literal, an empty hex string and a literal of
// line continuations show nothing (measured: `() Tj` builds no glyph), and a string of one byte shows a glyph nib
// cannot read.
func TestAnUnreadFontDrawsNoGlyphWithAnEmptyString(t *testing.T) {
	gb := "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 0 >>"
	for _, c := range []struct {
		show string
		want int
	}{
		{"() Tj <> Tj < > Tj (\\\n) Tj [() <>] TJ", 0},
		{"() Tj <21> Tj", 1},
		{"[() (\\041)] TJ", 1},
	} {
		objs := type0Doc("/GB-EUC-H", gb, map[int]string{4: spStream("", "/P <</MCID 0>> BDC BT /F0 12 Tf 10 10 Td "+c.show+" ET EMC")})
		d, err := open(buildPDF(objs))
		if err != nil {
			t.Fatal(err)
		}
		glyphs, why := d.glyphsDrawn()
		if why != "" {
			t.Fatalf("%q: %s", c.show, why)
		}
		for _, g := range glyphs {
			if g.unread == "" {
				t.Fatalf("setup: %q drew a glyph nib read, so the font is not one it cannot cut", c.show)
			}
		}
		if len(glyphs) != c.want {
			t.Errorf("%q drew %d glyphs, want %d", c.show, len(glyphs), c.want)
		}
	}
}
