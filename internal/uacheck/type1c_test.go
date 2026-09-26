package uacheck

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// P07.S05a's fixture generator: a small CFF program, laid out header → Name → Top DICT → String → Global Subr INDEX →
// charset → encoding → CharStrings → Private DICT → local Subrs, with every DICT offset written as a five-byte integer
// so the layout is computed in one pass.

// cffIndexBytes is a CFF INDEX of `items`.
func cffIndexBytes(items [][]byte) []byte {
	if len(items) == 0 {
		return []byte{0, 0}
	}
	total := 1
	for _, it := range items {
		total += len(it)
	}
	off := 1
	switch {
	case total > 0xFFFFFF:
		off = 4
	case total > 0xFFFF:
		off = 3
	case total > 0xFF:
		off = 2
	}
	b := []byte{byte(len(items) >> 8), byte(len(items)), byte(off)}
	put := func(v int) {
		for k := off - 1; k >= 0; k-- {
			b = append(b, byte(v>>(8*k)))
		}
	}
	pos := 1
	put(pos)
	for _, it := range items {
		pos += len(it)
		put(pos)
	}
	for _, it := range items {
		b = append(b, it...)
	}
	return b
}

// dictInt is a DICT operand as a five-byte integer.
func dictInt(v int) []byte {
	b := []byte{29, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], uint32(int32(v)))
	return b
}

// csNum is a Type 2 charstring operand.
func csNum(v int) []byte {
	if v >= -107 && v <= 107 {
		return []byte{byte(v + 139)}
	}
	return []byte{28, byte(uint16(int16(v)) >> 8), byte(uint16(int16(v)))}
}

// cs is a charstring of operands (ints) and operators (strings: "endchar", "hmoveto", "rmoveto", "callsubr",
// "callgsubr", "return", "rlineto", "hstem", or "#<byte>" for a raw byte).
func cs(parts ...any) []byte {
	ops := map[string]byte{"endchar": 14, "hmoveto": 22, "vmoveto": 4, "rmoveto": 21, "callsubr": 10, "callgsubr": 29,
		"return": 11, "rlineto": 5, "hstem": 1, "hintmask": 19}
	var b []byte
	for _, p := range parts {
		switch v := p.(type) {
		case int:
			b = append(b, csNum(v)...)
		case string:
			if strings.HasPrefix(v, "#") {
				n := 0
				for _, c := range v[1:] {
					n = n*10 + int(c-'0')
				}
				b = append(b, byte(n))
			} else {
				b = append(b, ops[v])
			}
		}
	}
	return b
}

// cffSpec is one program. Glyph names are standard strings where they are one, else added to the String INDEX.
type cffSpec struct {
	names       []string // GID 1.. (GID 0 is .notdef)
	charstrings [][]byte // GID 0..; nil: every glyph `endchar` (defaultWidthX)
	charset     []byte   // raw charset bytes; nil: format 0 over names
	predefined  int      // with charset nil and predefined > 0: charset offset predefined-1 (0 ISOAdobe, 1 Expert, 2 ExpertSubset)
	encoding    []byte   // raw encoding bytes; nil: Standard (offset 0)
	expertEnc   bool     // Expert encoding (offset 1)
	private     []byte   // Private DICT operators (Subrs is added when lsubrs is set)
	lsubrs      [][]byte
	gsubrs      [][]byte
	topExtra    []byte
	topPrefix   []byte // Top DICT bytes before every generated operator
	header      []byte // raw header; nil: 1 0 4 1
}

func (s cffSpec) build() []byte {
	var strs []string
	sid := func(name string) int {
		for i, n := range cffStandardStrings {
			if n == name {
				return i
			}
		}
		for i, n := range strs {
			if n == name {
				return len(cffStandardStrings) + i
			}
		}
		strs = append(strs, name)
		return len(cffStandardStrings) + len(strs) - 1
	}
	charset := s.charset
	if charset == nil && s.predefined == 0 {
		charset = []byte{0}
		for _, n := range s.names {
			v := sid(n)
			charset = append(charset, byte(v>>8), byte(v))
		}
	}
	css := s.charstrings
	if css == nil {
		css = make([][]byte, len(s.names)+1)
		for i := range css {
			css[i] = cs("endchar")
		}
	}
	var strItems [][]byte
	for _, n := range strs {
		strItems = append(strItems, []byte(n))
	}
	header := s.header
	if header == nil {
		header = []byte{1, 0, 4, 1}
	}
	nameIdx := cffIndexBytes([][]byte{[]byte("Probe")})
	private := append([]byte{}, s.private...)
	if s.lsubrs != nil {
		private = append(private, dictInt(0)...) // patched below: Subrs is relative to the Private DICT
		private = append(private, 19)
	}
	top := func(charsetOff, encOff, csOff, privOff int) []byte {
		t := append([]byte{}, s.topPrefix...)
		if charset != nil {
			t = append(append(t, dictInt(charsetOff)...), 15)
		} else {
			t = append(append(t, dictInt(s.predefined-1)...), 15)
		}
		switch {
		case s.encoding != nil:
			t = append(append(t, dictInt(encOff)...), 16)
		case s.expertEnc:
			t = append(append(t, dictInt(1)...), 16)
		}
		t = append(append(t, dictInt(csOff)...), 17)
		t = append(append(append(t, dictInt(len(private))...), dictInt(privOff)...), 18)
		return append(t, s.topExtra...)
	}
	strIdx := cffIndexBytes(strItems)
	gsubIdx := cffIndexBytes(s.gsubrs)
	csIdx := cffIndexBytes(css)
	topLen := len(cffIndexBytes([][]byte{top(0, 0, 0, 0)}))
	charsetOff := len(header) + len(nameIdx) + topLen + len(strIdx) + len(gsubIdx)
	encOff := charsetOff + len(charset)
	csOff := encOff + len(s.encoding)
	privOff := csOff + len(csIdx)
	if s.lsubrs != nil {
		copy(private[len(private)-6:], dictInt(len(private)))
	}
	out := append([]byte{}, header...)
	out = append(out, nameIdx...)
	out = append(out, cffIndexBytes([][]byte{top(charsetOff, encOff, csOff, privOff)})...)
	out = append(out, strIdx...)
	out = append(out, gsubIdx...)
	out = append(out, charset...)
	out = append(out, s.encoding...)
	out = append(out, csIdx...)
	out = append(out, private...)
	if s.lsubrs != nil {
		out = append(out, cffIndexBytes(s.lsubrs)...)
	}
	return out
}

// t1cDoc is a tagged page drawing `show` in a Type 1 font over the Type1C program `prog` (none when nil), named `base`,
// with `font` entries (/Widths, /Encoding, …) and `desc` entries (/CharSet, /MissingWidth, …).
func t1cDoc(base, font, desc, show string, prog []byte, progSubtype string) []byte {
	objs := map[int]string{
		12: "<< /Type /FontDescriptor /FontName /" + base + " /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 " +
			"/Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 " + desc,
	}
	if prog != nil {
		objs[12] += " /FontFile3 20 0 R"
		objs[20] = spStream("/Subtype /"+progSubtype, string(prog))
	}
	objs[12] += " >>"
	return glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /"+base+" /FontDescriptor 12 0 R "+font+" >>", show, objs)
}

func type1CFixtures() []measuredFixture {
	abc := []string{"A", "B", "C"}
	wd := func(ws ...int) [][]byte {
		out := [][]byte{cs(0, "endchar")}
		for _, w := range ws {
			out = append(out, cs(w, "endchar"))
		}
		return out
	}
	std := cffSpec{names: abc, charstrings: wd(500, 500, 500)}.build()
	w500 := "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding"
	full := "/CharSet (/A/B/C)"
	sub := "ABCDEF+Probe"
	show := "(ABC) Tj"
	return []measuredFixture{
		{name: "T1C: widths agree", vera: "PPPPP", pdf: t1cDoc(sub, w500, full, show, std, "Type1C")},
		{name: "T1C: a width 100 off", vera: "FPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [600 500 500] /Encoding /WinAnsiEncoding", full, show, std, "Type1C")},
		{name: "T1C: a width 1 off", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [501 500 500] /Encoding /WinAnsiEncoding", full, show, std, "Type1C")},
		{name: "T1C: a name the charset lacks", vera: "FFPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 500 500] /Encoding /WinAnsiEncoding", full, "(D) Tj", std, "Type1C")},
		{name: "T1C: defaultWidthX, nominalWidthX", vera: "FPPPP", pdf: t1cDoc(sub, w500, full, show, cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), cs("endchar"), cs(-100, "endchar"), cs(0, 0, "hmoveto")}, private: append(append(dictInt(500), 20), append(dictInt(600), 21)...)}.build(), "Type1C")},
		{name: "T1C: a FontMatrix of 0.002", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", full, show, cffSpec{names: abc, charstrings: wd(250, 250, 250), topExtra: append(append(append(append(append(append(dictRealBytes("0.002"), dictRealBytes("0")...), dictRealBytes("0")...), dictRealBytes("0.002")...), dictRealBytes("0")...), dictRealBytes("0")...), 12, 7)}.build(), "Type1C")},
		{name: "T1C: a width in a local subroutine", vera: "PPPPP", pdf: t1cDoc(sub, w500, full, show, cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), cs(-107, "callsubr"), cs(-107, "callsubr"), cs(-107, "callsubr")}, lsubrs: [][]byte{cs(500, "endchar")}}.build(), "Type1C")},
		{name: "T1C: a call past the subroutines, skipped", vera: "FPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", full, "(A) Tj", cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), cs(-100, "callsubr"), cs(500, "endchar"), cs(500, "endchar")}, lsubrs: [][]byte{cs(500, "endchar")}}.build(), "Type1C")},
		{name: "T1C: a call past the subroutines, /MissingWidth 500", vera: "FPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", full+" /MissingWidth 500", "(A) Tj", cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), cs(-100, "callsubr"), cs(500, "endchar"), cs(500, "endchar")}, lsubrs: [][]byte{cs(500, "endchar")}}.build(), "Type1C")},
		{name: "T1C: no /Encoding, the program's Standard", vera: "FFPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 500 500]", full, "(ABD) Tj", std, "Type1C")},
		{name: "T1C: no /Encoding, the program's own, code 0x44 unlisted", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 700 500]", full, "(ABCD) Tj", cffSpec{names: abc, charstrings: wd(500, 500, 700), encoding: []byte{0, 3, 65, 66, 67}}.build(), "Type1C")},
		{name: "T1C: a program veraPDF cannot parse", vera: "PPPPF", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [900 900 900] /Encoding /WinAnsiEncoding", full, show, std[:30], "Type1C")},
		{name: "T1C: /FontFile3 of another subtype", vera: "PPPPF", pdf: t1cDoc(sub, w500, full, show, std, "Foo")},
		{name: "T1C: ISOAdobe charset", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", "", show, cffSpec{predefined: 1, charstrings: make230()[:40]}.build(), "Type1C")},
		{name: "CS: a CharSet missing a glyph", vera: "PPPFP", pdf: t1cDoc(sub, w500, "/CharSet (/A/B)", show, std, "Type1C")},
		{name: "CS: a CharSet with a glyph too many", vera: "PPPFP", pdf: t1cDoc(sub, w500, "/CharSet (/A/B/C/D)", show, std, "Type1C")},
		{name: "CS: a CharSet with .notdef", vera: "PPPPP", pdf: t1cDoc(sub, w500, "/CharSet (/.notdef/A/B/C)", show, std, "Type1C")},
		{name: "CS: a CharSet with duplicates", vera: "PPPPP", pdf: t1cDoc(sub, w500, "/CharSet (/A/A/B/C/C)", show, std, "Type1C")},
		{name: "CS: not subset-named, CharSet short", vera: "PPPPP", pdf: t1cDoc("Probe", w500, "/CharSet (/A/B)", show, std, "Type1C")},
		{name: "CS: subset-named, no CharSet", vera: "PPPPP", pdf: t1cDoc(sub, w500, "", show, std, "Type1C")},
		{name: "CS: unparsed program, CharSet short", vera: "PPPPF", pdf: t1cDoc(sub, w500, "/CharSet (/A)", show, std[:30], "Type1C")},
		{name: "CS: a format 2 charset reading past the count", vera: "FPPFP", pdf: t1cDoc(sub, w500, full, show, cffSpec{names: abc, charset: []byte{2, 0, 34, 0, 4}}.build(), "Type1C")},
		{name: "CS: lowercase prefix, CharSet short", vera: "PPPPP", pdf: t1cDoc("abcdef+Probe", w500, "/CharSet (/A/B)", show, std, "Type1C")},
		{name: "CS: a hash-escaped name", vera: "PPPPP", pdf: t1cDoc(sub, w500, "/CharSet (/#41/B/C)", show, std, "Type1C")},
	}
}

// dictRealBytes is a DICT real operand.
func dictRealBytes(s string) []byte {
	var nib []byte
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			nib = append(nib, byte(c-'0'))
		case c == '.':
			nib = append(nib, 0xA)
		case c == '-':
			nib = append(nib, 0xE)
		}
	}
	nib = append(nib, 0xF)
	if len(nib)%2 == 1 {
		nib = append(nib, 0xF)
	}
	b := []byte{30}
	for i := 0; i < len(nib); i += 2 {
		b = append(b, nib[i]<<4|nib[i+1])
	}
	return b
}

func make230() [][]byte {
	out := make([][]byte, 230)
	for i := range out {
		out[i] = cs(500, "endchar")
	}
	return out
}

// type1CRefusals are the (fixture, clause) pairs nib deliberately refuses, as knownCannotCheck records them.
var type1CRefusals = map[string]bool{
	"T1C: charstring type 1 / 7.21.5 t1":   true,
	"T1C: charstring type 1 / 7.21.4.1 t2": true,
}

// type1CClauses are the clauses P07.S05a's fixtures are measured over, in the order of their `vera` strings.
var type1CClauses = []string{"7.21.5 t1", "7.21.4.1 t2", "7.21.8 t1", "7.21.4.2 t1", "7.21.4.1 t1"}

// TestTheType1CClausesAgreeWithVeraPDF — every Type1C shape against veraPDF 1.30.2's verdicts, measured before the rules
// were written; the live oracle asks veraPDF again on every run.
func TestTheType1CClausesAgreeWithVeraPDF(t *testing.T) {
	want := map[byte]Verdict{'P': Pass, 'F': Fail}
	for _, f := range append(append(type1CFixtures(), type1CFixturesMore()...), type1CFixturesProbed()...) {
		for j, clause := range type1CClauses {
			got := verdictOf(t, f.pdf, clause)
			w, ok := want[f.vera[j]]
			if !ok {
				t.Fatalf("%s: verdict letter %q is neither P nor F", f.name, f.vera[j])
			}
			// A declared refusal holds only on the clauses `type1CRefusals` names — the same ones knownCannotCheck lists.
			if f.refused != "" && type1CRefusals[f.name+" / "+clause] && got.Verdict == CannotCheck && strings.Contains(got.Why, f.refused) {
				continue
			}
			if got.Verdict != w {
				t.Errorf("%s: %s reports %v (%s), veraPDF %c", f.name, clause, got.Verdict, got.Why, f.vera[j])
			}
		}
	}
}

// type1CFixturesMore are P07.S05a's second round of Type1C shapes: the program's own Expert, format 1 and supplemented
// encodings, full (non-subset) fonts read lazily, the stream boundary, charstring type 1, and each width-setting operator.
func type1CFixturesMore() []measuredFixture {
	abc := []string{"A", "B", "C"}
	w500 := "/FirstChar 65 /LastChar 67 /Widths [500 500 500]"
	full := "/CharSet (/A/B/C)"
	sub := "ABCDEF+Probe"
	wd := [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(700, "endchar")}
	big := make([][]byte, 1)
	big[0] = cs(0, "endchar")
	for i := 0; i < 3; i++ {
		body := append(cs(500), make([]byte, 0)...)
		for len(body) < 4000 {
			body = append(body, csNum(1)...)
		}
		big = append(big, append(body, 14))
	}
	pastEnd := func(n int) []byte {
		p := cffSpec{names: abc, charstrings: wd}.build()
		for len(p) < n {
			p = append(p, 0)
		}
		// point CharStrings (operator 17) past the end: find the five-byte integer before byte 17 in the Top DICT
		for i := 0; i+5 < len(p); i++ {
			if p[i] == 29 && p[i+5] == 17 {
				copy(p[i+1:i+5], []byte{0, 0x10, 0, 0})
				break
			}
		}
		return p
	}
	return []measuredFixture{
		{name: "T1C: no /Encoding, the program's Expert", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 32 /LastChar 67 /Widths [500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500 500]", "", "( !) Tj", cffSpec{names: []string{"space", "exclamsmall"}, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar")}, expertEnc: true}.build(), "Type1C")},
		{name: "T1C: no /Encoding, a format 1 program encoding", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [500 500 700]", full, "(ABC) Tj", cffSpec{names: abc, charstrings: wd, encoding: []byte{1, 1, 65, 2}}.build(), "Type1C")},
		{name: "T1C: no /Encoding, a supplement storing a SID", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 700 500]", full, "(ABCD) Tj", cffSpec{names: abc, charstrings: wd, encoding: []byte{128, 3, 65, 66, 67, 1, 68, 0, 1}}.build(), "Type1C")},
		{name: "T1C: a full font, widths agree", vera: "PPPPP", pdf: t1cDoc("Probe", w500+" /Encoding /WinAnsiEncoding", full, "(ABC) Tj", cffSpec{names: abc, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(500, "endchar")}}.build(), "Type1C")},
		{name: "T1C: a full font, a name the charset lacks", vera: "PFPPP", pdf: t1cDoc("Probe", "/FirstChar 65 /LastChar 68 /Widths [500 500 500 0] /Encoding /WinAnsiEncoding", full, "(D) Tj", cffSpec{names: abc, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(500, "endchar")}}.build(), "Type1C")},
		{name: "T1C: a small program, CharStrings past its end", vera: "PPPPF", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", full, "(A) Tj", pastEnd(0), "Type1C")},
		{name: "T1C: charstring type 1", vera: "FPPPP", refused: "type 1", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", full, "(A) Tj", cffSpec{names: abc, charstrings: wd, topExtra: append(dictInt(1), 12, 6)}.build(), "Type1C")},
		{name: "T1C: a width set by hintmask", vera: "PPPPP", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", full, "(A) Tj", cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), cs(500, 1, 2, "hintmask"), cs("endchar"), cs("endchar")}}.build(), "Type1C")},
		{name: "T1C: a width set by rmoveto", vera: "PPPPP", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", full, "(A) Tj", cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), cs(500, 1, 2, "rmoveto"), cs("endchar"), cs("endchar")}}.build(), "Type1C")},
		{name: "T1C: rmoveto with two operands sets none", vera: "PPPPP", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", full, "(A) Tj", cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), cs(1, 2, "rmoveto"), cs("endchar"), cs("endchar")}, private: append(dictInt(500), 20)}.build(), "Type1C")},
		{name: "T1C: a fixed-point width 500.5", vera: "PPPPP", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", full, "(A) Tj", cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), append([]byte{255, 0x01, 0xF4, 0x80, 0x00}, 14), cs("endchar"), cs("endchar")}}.build(), "Type1C")},
		{name: "T1C: a fixed-point width 501.5", vera: "FPPPP", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", full, "(A) Tj", cffSpec{names: abc, charstrings: [][]byte{cs("endchar"), append([]byte{255, 0x01, 0xF5, 0x80, 0x00}, 14), cs("endchar"), cs("endchar")}}.build(), "Type1C")},
		{name: "T1C: a big full font, widths agree", vera: "PPPPP", pdf: t1cDoc("Probe", w500+" /Encoding /WinAnsiEncoding", full, "(ABC) Tj", cffSpec{names: abc, charstrings: big}.build(), "Type1C")},
		// The review's shapes (measured).
		{name: "CS: drawn only invisibly, CharSet short", vera: "PPPFP", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", "/CharSet (/A/B)", "3 Tr (ABC) Tj", cffSpec{names: abc, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(500, "endchar")}}.build(), "Type1C")},
		{name: "CS: a hex-string CharSet, short", vera: "PPPFP", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", "/CharSet <2F412F42>", "(ABC) Tj", cffSpec{names: abc, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(500, "endchar")}}.build(), "Type1C")},
		{name: "T1C: Encoding with no operand before ROS", vera: "PPPPF", pdf: t1cDoc(sub, w500+" /Encoding /WinAnsiEncoding", "", "(A) Tj", cffSpec{names: abc, topPrefix: []byte{16, 0x8b, 0x8b, 0x8b, 12, 30}}.build(), "Type1C")},
	}
}

// TestAPaddedINDEXCopyIsBounded — only an INDEX's last offset is bounded by its data, so a middle one can ask for a copy
// of up to 2 GiB of zeros (the P07.S05a review measured 512 MB from a 100-byte program); past cffMaxAlloc nib refuses.
func TestAPaddedINDEXCopyIsBounded(t *testing.T) {
	x := cffIndex{count: 2, offsets: []int{1, 0x10000000, 3}, data: []byte("xa"), padded: new(int)}
	if _, err := x.get(0); err == nil || err.kind != errRefuse {
		t.Fatalf("a 256 MiB padded copy: %v, want a refusal", err)
	}
	small := cffIndex{count: 1, offsets: []int{1, 5}, data: []byte("xa"), padded: new(int)}
	if b, err := small.get(0); err != nil || len(b) != 4 || b[2] != 0 {
		t.Fatalf("a short padded copy: %v %v, want four bytes, zero-padded", b, err)
	}
}

// TestGetRefusesABackwardsRangeBeforeAStartPastTheData — `Arrays.copyOfRange` checks from > to (an
// IllegalArgumentException, which veraPDF does not catch) before a start past the data (an ArrayIndexOutOfBounds it
// sometimes does); the review measured the reverse order answering "not parsed" where veraPDF reports nothing.
func TestGetRefusesABackwardsRangeBeforeAStartPastTheData(t *testing.T) {
	x := cffIndex{count: 2, offsets: []int{16, 5, 6}, data: make([]byte, 5)}
	if _, err := x.get(0); err == nil || err.kind != errThrow {
		t.Fatalf("offsets [16 5 6] over 5 bytes: %v, want the uncaught IllegalArgumentException", err)
	}
	past := cffIndex{count: 1, offsets: []int{7, 8}, data: make([]byte, 5)}
	if _, err := past.get(0); !isAIOOBE(err) {
		t.Fatalf("offsets [7 8] over 5 bytes: %v, want an ArrayIndexOutOfBounds (a start past the data, in order)", err)
	}
}

// type1CFixturesProbed are the shapes P07.S05a's red-proof found no fixture holding — each surviving mutation's case,
// measured on veraPDF before it was pinned (7.21.7 t1 among them, asked through the oracle).
func type1CFixturesProbed() []measuredFixture {
	abc := []string{"A", "B", "C"}
	sub := "ABCDEF+Probe"
	win := " /Encoding /WinAnsiEncoding"
	w := func(a, b, c int) string { return fmt.Sprintf("/FirstChar 65 /LastChar 67 /Widths [%d %d %d]", a, b, c) }
	full := "/CharSet (/A/B/C)"
	css := func(a, b, c []byte) [][]byte { return [][]byte{cs(300, "endchar"), a, b, c} }
	e500 := cs(500, "endchar")
	// a String-less CharStrings INDEX whose entry 3 runs backwards
	backwards := func(base string) []byte {
		p := cffSpec{names: abc, charstrings: css(e500, e500, cs(500, 1, "hmoveto"))}.build()
		i := bytes.Index(p, []byte{0, 4, 1, 1})
		p[i+6] = p[i+7] + 1
		return p
	}
	lsubrs := make([][]byte, 1240)
	for i := range lsubrs {
		lsubrs[i] = cs(900, "endchar")
	}
	lsubrs[0] = cs(500, "endchar")
	return []measuredFixture{
		{name: "T1C: no /Encoding, a supplement landing on a wider glyph", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 700 700]", full, "(ABCD) Tj", cffSpec{names: abc, charstrings: css(e500, e500, cs(700, "endchar")), encoding: []byte{128, 3, 65, 66, 67, 1, 68, 0, 2}}.build(), "Type1C")},
		{name: "T1C: a charstring error under nominalWidthX 600", vera: "PPPPP", pdf: t1cDoc(sub, w(599, 500, 500)+win, full, "(A) Tj", cffSpec{names: abc, charstrings: css(cs(-108, "callsubr"), e500, e500), lsubrs: [][]byte{e500}, private: append(dictInt(600), 21)}.build(), "Type1C")},
		{name: "T1C: a charstring error, /MissingWidth 500", vera: "PPPPP", pdf: t1cDoc(sub, w(500, 500, 500)+win, full+" /MissingWidth 500", "(A) Tj", cffSpec{names: abc, charstrings: css(cs(-108, "callsubr"), e500, e500), lsubrs: [][]byte{e500}}.build(), "Type1C")},
		{name: "T1C: endchar with four operands sets no width", vera: "PPPPP", pdf: t1cDoc(sub, w(500, 500, 500)+win, full, "(A) Tj", cffSpec{names: abc, charstrings: css(cs(1, 2, 3, 4, "endchar"), e500, e500), private: append(dictInt(500), 20)}.build(), "Type1C")},
		{name: "T1C: hmoveto with one operand sets no width", vera: "PPPPP", pdf: t1cDoc(sub, w(500, 500, 500)+win, full, "(A) Tj", cffSpec{names: abc, charstrings: css(cs(10, "hmoveto"), e500, e500), private: append(dictInt(500), 20)}.build(), "Type1C")},
		{name: "T1C: no /Encoding, a code whose GID is the count", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 500 300]", full, "(D) Tj", cffSpec{names: abc, charstrings: css(e500, e500, e500), encoding: []byte{0, 4, 65, 66, 67, 68}}.build(), "Type1C")},
		{name: "T1C: full, an undrawn charstring running backwards", vera: "PPPPP", pdf: t1cDoc("Probe", w(500, 500, 500)+win, full, "(A) Tj", backwards("Probe"), "Type1C")},
		{name: "T1C: code 0 named by /Differences, absent from the charset", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 0 /LastChar 0 /Widths [300] /Encoding << /Type /Encoding /Differences [0 /zzz] >>", full, "<00> Tj", cffSpec{names: abc, charstrings: css(e500, e500, e500)}.build(), "Type1C")},
		{name: "CS: no + at the seventh character, CharSet short", vera: "PPPPP", pdf: t1cDoc("ABCDEFGProbe", w(500, 500, 500)+win, "/CharSet (/A/B)", "(ABC) Tj", cffSpec{names: abc, charstrings: css(e500, e500, e500)}.build(), "Type1C")},
		{name: "T1C: /FontFile3 of another subtype, no /Encoding", vera: "PPPPF", pdf: t1cDoc(sub, w(500, 500, 500), full, "(A) Tj", cffSpec{names: abc, charstrings: css(e500, e500, e500)}.build(), "Foo")},
		{name: "T1C: a negative 16-bit charstring width", vera: "PPPPP", pdf: t1cDoc(sub, w(500, 500, 500)+win, full, "(A) Tj", cffSpec{names: abc, charstrings: css([]byte{28, 0xFF, 0x38, 14}, e500, e500), private: append(dictInt(700), 21)}.build(), "Type1C")},
		{name: "T1C: defaultWidthX written with byte 28 above 32767", vera: "PPPPP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [40000 500 500]"+win, full, "(A) Tj", cffSpec{names: abc, charstrings: css(cs("endchar"), e500, e500), private: []byte{28, 0x9C, 0x40, 20}}.build(), "Type1C")},
		{name: "T1C: 1240 local subroutines, bias 1131", vera: "PPPPP", pdf: t1cDoc(sub, w(500, 500, 500)+win, full, "(A) Tj", cffSpec{names: abc, charstrings: css(cs(-1131, "callsubr"), e500, e500), lsubrs: lsubrs}.build(), "Type1C")},
	}
}

// allocated is the bytes the process has allocated so far — the work count the view tests below read in place of a
// clock, since a copy costs its length whatever the machine.
func allocated() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc
}

// TestASubroutineCallCostsWhatItReads — a call reads its subroutine through a view, never a copy: a 1 MB subroutine
// that calls itself was copied whole at each of its 64 nested calls, so 500 glyphs allocated 30 GB (7.5 s, measured at
// the P07 phase close) while reading 130 bytes each. The control is the same megabyte holding a real width.
func TestASubroutineCallCostsWhatItReads(t *testing.T) {
	names := make([]string, 499)
	for i := range names {
		names[i] = fmt.Sprintf("g%d", i+1)
	}
	css := make([][]byte, 500)
	for i := range css {
		css[i] = []byte{32, 10} // -107 callsubr: local subroutine 0 under a bias of 107
	}
	build := func(head []byte) []byte {
		subr := make([]byte, 1<<20)
		copy(subr, head)
		return cffSpec{names: names, charstrings: css, lsubrs: [][]byte{subr}}.build()
	}
	for _, c := range []struct {
		name string
		head []byte
		want func(p cffProgram) string // "" when the stimulus happened
	}{
		{"a subroutine calling itself", []byte{32, 10}, func(p cffProgram) string {
			for gid := range 500 {
				if !strings.Contains(p.widths.errs[gid], "deeper than") {
					return fmt.Sprintf("glyph %d: %q, want refused at the nesting bound", gid, p.widths.errs[gid])
				}
			}
			return ""
		}},
		{"a subroutine setting a width (the control)", cs(500, "endchar"), func(p cffProgram) string {
			for gid, w := range p.widths.subset {
				if w != 500 || p.widths.errs[gid] != "" {
					return fmt.Sprintf("glyph %d: width %v (%q), want 500", gid, w, p.widths.errs[gid])
				}
			}
			return ""
		}},
	} {
		prog := build(c.head)
		spend := &cffSpend{}
		before := allocated()
		p := readCFF(prog, true, spend)
		spent := allocated() - before
		if p.state != ttParsed || p.widths == nil || len(p.widths.subset) != 500 {
			t.Fatalf("%s: %v (%s), want parsed with 500 widths", c.name, p.state, p.why)
		}
		if msg := c.want(p); msg != "" {
			t.Fatalf("%s: %s", c.name, msg)
		}
		if spend.charstring < 500*2 {
			t.Fatalf("%s: %d charstring bytes read, want at least each glyph's own", c.name, spend.charstring)
		}
		if spent > 64<<20 {
			t.Errorf("%s: reading 500 glyphs allocated %d MB — the subroutine is copied, not read through a view", c.name, spent>>20)
		}
	}
}

// TestAFetchedCharstringIsAViewNotACopy — a charstring is fetched at every width asked of a type-1 CFF program (which
// then refuses), and from a large CharStrings INDEX one entry may span the program: 256 asks of a 4 MB glyph allocated
// 1 GB before the fetch became a view (measured at the P07 phase close).
func TestAFetchedCharstringIsAViewNotACopy(t *testing.T) {
	big := make([]byte, 4<<20)
	big[0] = 14
	p := readCFF(cffSpec{names: []string{"A"}, charstrings: [][]byte{big, {14}}, topExtra: append(dictInt(1), 12, 6)}.build(), false, nil)
	if p.state != ttParsed || !p.widths.bigCS {
		t.Fatalf("a type-1 program with a 4 MB glyph: %v (%s), big CharStrings %v, want parsed from the stream", p.state, p.why, p.widths != nil && p.widths.bigCS)
	}
	before := allocated()
	refused := 0
	for range 256 {
		if _, err := p.widths.width(0); err != nil && strings.Contains(err.why, "type 1") {
			refused++
		}
	}
	spent := allocated() - before
	if refused != 256 {
		t.Fatalf("%d of 256 asks refused naming type 1, want every one (each fetches the charstring first)", refused)
	}
	if spent > 64<<20 {
		t.Errorf("256 fetches of a 4 MB charstring allocated %d MB — it is copied, not viewed", spent>>20)
	}
}

// TestTheCFFBudgetsAreTheDocuments — the CFF bounds are the DOCUMENT's, as the TrueType budget is
// (`TestTheReadBudgetIsTheDocuments`): each case is two programs, each under the bound alone (the control), together
// over it — the second refused naming the document's CFF programs. Through a document once (the names), and at the
// reader for the charstring bytes; `TestTheCIDBudgetsAreTheDocuments` holds the DICT and FDSelect ones.
func TestTheCFFBudgetsAreTheDocuments(t *testing.T) {
	// Glyph names: a charset of ten SIDs over one 1 MB string is 10 MB of the 16 MB bound.
	named := func(tag string) []byte {
		nm := make([]string, 10)
		for i := range nm {
			nm[i] = tag + strings.Repeat("x", 1<<20)
		}
		return cffSpec{names: nm}.build()
	}
	a, b := named("a"), named("b")
	spend := &cffSpend{}
	if p := readCFF(a, false, spend); p.state != ttParsed || spend.names < cffMaxNameBytes/2 || spend.names > cffMaxNameBytes*3/4 {
		t.Fatalf("one program of 10 MB of names: %v (%s) after %d bytes, want parsed between half and three quarters of the bound", p.state, p.why, spend.names)
	}
	if p := readCFF(b, false, nil); p.state != ttParsed {
		t.Fatalf("the second program alone: %v (%s), want parsed (the control)", p.state, p.why)
	}
	doc := func(progA, progB []byte) []byte {
		objs := map[int]string{}
		for i, prog := range [][]byte{progA, progB} {
			objs[12+i] = fmt.Sprintf("<< /Type /FontDescriptor /FontName /P%d /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 "+
				"/Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile3 %d 0 R >>", i, 20+i)
			objs[20+i] = spStream("/Subtype /Type1C", string(prog))
		}
		objs[11] = "<< /Type /Font /Subtype /Type1 /BaseFont /P1 /FontDescriptor 13 0 R /Encoding /WinAnsiEncoding >>"
		return glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj /F1 12 Tf (A) Tj ET", "/F1 11 0 R", "",
			"<< /Type /Font /Subtype /Type1 /BaseFont /P0 /FontDescriptor 12 0 R /Encoding /WinAnsiEncoding >>", objs)
	}
	d, err := open(doc(a, b))
	if err != nil {
		t.Fatal(err)
	}
	var known []bool
	var whys []string
	for _, n := range []int{10, 11} {
		sp, ok, why, _ := d.simpleProgramOf(d.dict(*types.NewIndirectRef(n, 0)))
		if sp.kind != "CFF" {
			t.Fatalf("font %d: program kind %q, want CFF", n, sp.kind)
		}
		known, whys = append(known, ok), append(whys, why)
	}
	if len(d.type1CReads) != 2 {
		t.Fatalf("%d programs read, want both", len(d.type1CReads))
	}
	if !known[0] || known[1] || !strings.Contains(whys[1], "the document's CFF programs") {
		t.Fatalf("two name-heavy programs in one document: known %v (%q), want the first read and the second refused naming the document's CFF programs", known, whys)
	}

	// Charstring bytes: 20 glyphs each reading just under a megabyte of `return` through one subroutine is 20 MB of the
	// 32 MB bound; a width is one glyph's, so the second program's later glyphs are refused and the program parses.
	read := func() []byte {
		subr := bytes.Repeat([]byte{11}, 1<<20-64)
		css := make([][]byte, 20)
		for i := range css {
			css[i] = []byte{32, 10}
		}
		nm := make([]string, 19)
		for i := range nm {
			nm[i] = fmt.Sprintf("g%d", i+1)
		}
		return cffSpec{names: nm, charstrings: css, lsubrs: [][]byte{subr}}.build()
	}
	refusedBy := func(p cffProgram) int {
		n := 0
		for _, why := range p.widths.errs {
			if strings.Contains(why, "the document's CFF programs") {
				n++
			}
		}
		return n
	}
	spend = &cffSpend{}
	if p := readCFF(read(), true, spend); p.state != ttParsed || len(p.widths.errs) != 0 || spend.charstring < cffMaxCharstringTotal/2 || spend.charstring > cffMaxCharstringTotal*3/4 {
		t.Fatalf("one program reading 20 MB: %v (%s), %d glyphs refused, %d bytes read, want parsed with none refused between half and three quarters of the bound", p.state, p.why, len(p.widths.errs), spend.charstring)
	}
	if p := readCFF(read(), true, nil); p.state != ttParsed || len(p.widths.errs) != 0 {
		t.Fatalf("the second program alone: %v (%s), %d glyphs refused, want none (the control)", p.state, p.why, len(p.widths.errs))
	}
	if p := readCFF(read(), true, spend); p.state != ttParsed || refusedBy(p) == 0 {
		t.Fatalf("the second program after the first: %v (%s), %d glyphs refused naming the document's CFF programs, want some", p.state, p.why, refusedBy(p))
	}
}
