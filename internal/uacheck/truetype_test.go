package uacheck

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// P07.S03's fixtures: a synthetic TrueType program generator, and every shape of 7.21.6 (and of /pending 677's
// TrueType half) run on veraPDF 1.30.2 before the rules were written. The corpus holds only pass halves of t1 and
// t4, so their fail halves live here — and so does every way a program can fail veraPDF's parse, because the parse
// decides whether t1/t4 have a subject at all.

// ttTable is one table of a synthetic TrueType program.
type ttTable struct {
	tag  string
	data []byte
}

// sfnt assembles a program from its tables, in the order given, each 4-byte aligned; checksums are zero, which
// veraPDF never reads.
func sfnt(tables ...ttTable) []byte {
	var b bytes.Buffer
	b.Write(beBytes(uint32(0x00010000), uint16(len(tables)), uint16(0), uint16(0), uint16(0)))
	off := 12 + 16*len(tables)
	for _, t := range tables {
		b.WriteString(t.tag)
		b.Write(beBytes(uint32(0), uint32(off), uint32(len(t.data))))
		off += (len(t.data) + 3) &^ 3
	}
	for _, t := range tables {
		b.Write(t.data)
		b.Write(make([]byte, ((len(t.data)+3)&^3)-len(t.data)))
	}
	return b.Bytes()
}

func beBytes(v ...any) []byte {
	var b bytes.Buffer
	for _, x := range v {
		binary.Write(&b, binary.BigEndian, x)
	}
	return b.Bytes()
}

const ttGlyphs = 100

func ttHead() []byte {
	h := make([]byte, 54)
	copy(h, beBytes(uint32(0x00010000), uint32(0x00010000), uint32(0), uint32(0x5F0F3CF5), uint16(0), uint16(1000)))
	return h
}

func ttHhea(n int) []byte {
	h := make([]byte, 36)
	copy(h, beBytes(uint32(0x00010000), int16(800), int16(-200)))
	binary.BigEndian.PutUint16(h[34:], uint16(n))
	return h
}

func ttHmtx(n int) []byte {
	var b []byte
	for i := 0; i < n; i++ {
		b = append(b, beBytes(uint16(500), int16(0))...)
	}
	return b
}

func ttMaxp(n int) []byte { return beBytes(uint32(0x00005000), uint16(n)) }

func ttPost3() []byte { return append(beBytes(uint32(0x00030000)), make([]byte, 28)...) }

// ttSub is one cmap subtable: its (platform, encoding) and its body from the format field on.
type ttSub struct {
	plat, enc uint16
	body      []byte
}

// cmapFmt4 maps codes [first, first+n) to glyphs 1.. through one segment and the terminal one.
func cmapFmt4(first, n int) []byte {
	end := first + n - 1
	delta := uint16(1 - first)
	body := beBytes(uint16(4), uint16(0), uint16(0), uint16(4), uint16(0), uint16(0), uint16(0),
		uint16(end), uint16(0xFFFF), uint16(0), uint16(first), uint16(0xFFFF), delta, uint16(1), uint16(0), uint16(0))
	binary.BigEndian.PutUint16(body[2:], uint16(len(body)))
	return body
}

func ttCmap(subs ...ttSub) []byte {
	hdr := beBytes(uint16(0), uint16(len(subs)))
	var bodies []byte
	off := 4 + 8*len(subs)
	for _, s := range subs {
		hdr = append(hdr, beBytes(s.plat, s.enc, uint32(off))...)
		off += len(s.body)
		bodies = append(bodies, s.body...)
	}
	return append(hdr, bodies...)
}

// ttProgram is a program veraPDF opens, carrying the cmap subtables given (none: a cmap with zero subtables).
func ttProgram(subs ...ttSub) []byte {
	return sfnt(ttTable{"cmap", ttCmap(subs...)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
		ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"post", ttPost3()})
}

var (
	sub31 = ttSub{3, 1, cmapFmt4(0x20, 95)}
	sub10 = ttSub{1, 0, cmapFmt4(0x20, 95)}
	sub30 = ttSub{3, 0, cmapFmt4(0xF020, 95)}
)

// ttFontDict is a simple TrueType font over descriptor 12, with `extra` (an /Encoding) in its dictionary.
func ttFontDict(extra string) string {
	return "<< /Type /Font /Subtype /TrueType /BaseFont /Probe /FirstChar 65 /LastChar 67 /Widths [500 500 500] " +
		"/FontDescriptor 12 0 R " + extra + " >>"
}

// ttObjects is descriptor 12 with /Flags `flags` and, when prog is not nil, the program at 20 under `key`.
func ttObjects(flags, key string, prog []byte, progDict string) map[int]string {
	desc := "<< /Type /FontDescriptor /FontName /Probe " + flags + " /FontBBox [0 0 1000 1000] /ItalicAngle 0 " +
		"/Ascent 800 /Descent -200 /CapHeight 700 /StemV 80"
	o := map[int]string{}
	if prog != nil {
		desc += " /" + key + " 20 0 R"
		if key == "FontFile2" {
			progDict += fmt.Sprintf(" /Length1 %d", len(prog)) // pdfcpu requires it; veraPDF never reads it
		}
		o[20] = spStream(progDict, string(prog))
	}
	o[12] = desc + " >>"
	return o
}

// ttDoc is one tagged page showing `show` in that font.
func ttDoc(flags, enc, show string, prog []byte) []byte {
	return glyphDoc(ttFontDict(enc), show, ttObjects(flags, "FontFile2", prog, ""))
}

// ttPage is ttDoc with the whole content written by the caller.
func ttPage(flags, enc, content string, prog []byte) []byte {
	return glyphPage(content, "", "", ttFontDict(enc), ttObjects(flags, "FontFile2", prog, ""))
}

// ttShared is two fonts, F0 then F1, sharing one descriptor and so one program. The second is named apart unless
// `twin`, because pdfcpu's optimizer fuses byte-identical font dictionaries into one.
func ttShared(enc0, enc1 string, twin bool, prog []byte) []byte {
	second := ttFontDict(enc1)
	if !twin {
		second = strings.Replace(second, "/BaseFont /Probe", "/BaseFont /ProbeTwo", 1)
	}
	return glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj /F1 12 Tf (A) Tj ET", "/F1 11 0 R", "", ttFontDict(enc0),
		mergeObjs(ttObjects("/Flags 32", "FontFile2", prog, ""), map[int]string{11: second}))
}

// ttSharedAs is two fonts named `base0` and `base1` sharing one program under `key` (`progDict` on its stream).
func ttSharedAs(key, progDict, base0, enc0, base1, enc1 string, prog []byte) []byte {
	f := func(base, enc string) string {
		return strings.Replace(ttFontDict(enc), "/BaseFont /Probe", "/BaseFont /"+base, 1)
	}
	return glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj /F1 12 Tf (A) Tj ET", "/F1 11 0 R", "", f(base0, enc0),
		mergeObjs(ttObjects("/Flags 32", key, prog, progDict), map[int]string{11: f(base1, enc1)}))
}

func mergeObjs(a, b map[int]string) map[int]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

// withTableField rewrites one field of a named table's directory record (8: offset, 12: length).
func withTableField(prog []byte, tag string, field int, f func(uint32) uint32) []byte {
	out := append([]byte(nil), prog...)
	n := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < n; i++ {
		r := 12 + i*16
		if string(out[r:r+4]) == tag {
			binary.BigEndian.PutUint32(out[r+field:], f(binary.BigEndian.Uint32(out[r+field:])))
		}
	}
	return out
}

// brokenPrograms are programs whose parse veraPDF refuses or survives, each measured both ways (`-nonsym` under a
// WinAnsi dictionary with Differences, `-sym` with no encoding). `ok` is whether veraPDF parsed it.
func brokenPrograms() []struct {
	name string
	prog []byte
	ok   bool
} {
	full := func(cm, post, hm []byte, nh int) []byte {
		t := []ttTable{{"cmap", cm}, {"head", ttHead()}, {"hhea", ttHhea(nh)}, {"hmtx", hm}, {"maxp", ttMaxp(ttGlyphs)}}
		if post != nil {
			t = append(t, ttTable{"post", post})
		}
		return sfnt(t...)
	}
	std := ttCmap(sub31, sub10)
	p25 := func(b byte) []byte {
		p := append(beBytes(uint32(0x00028000)), make([]byte, 28)...)
		return append(p, bytes.Repeat([]byte{b}, ttGlyphs)...)
	}
	p2 := append(beBytes(uint32(0x00020000)), make([]byte, 28)...)
	p2 = append(p2, beBytes(uint16(ttGlyphs))...)
	p2 = append(p2, make([]byte, 2*ttGlyphs)...)
	p2 = append(p2, 3, 'a', 'b', 'c')
	post2 := full(std, p2, ttHmtx(ttGlyphs), ttGlyphs)
	otto := ttProgram(sub31, sub10)
	copy(otto, "OTTO")
	fmt0Short := ttSub{3, 1, beBytes(uint16(0), uint16(262), uint16(0), make([]byte, 10))}
	cutFmt0 := ttSub{3, 1, beBytes(uint16(0), uint16(262), uint16(0))}
	big := sfnt(ttTable{"cmap", std}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
		ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"glyf", make([]byte, 12000)})
	past := func(uint32) uint32 { return 900000 }
	oneCode := beBytes(uint16(4), uint16(32), uint16(0), uint16(4), uint16(0), uint16(0), uint16(0), uint16(0x20), uint16(0xFFFF),
		uint16(0), uint16(0x20), uint16(0xFFFF), uint16(0), uint16(1), uint16(60000), uint16(0))
	f4 := beBytes(uint16(4), uint16(32), uint16(0), uint16(4), uint16(0), uint16(0), uint16(0), uint16(0x7E), uint16(0xFFFF),
		uint16(0), uint16(0x20), uint16(0xFFFF), uint16(0), uint16(1), uint16(60000), uint16(0))
	return []struct {
		name string
		prog []byte
		ok   bool
	}{
		// A subtable cut short and FOLLOWED by other tables reads their bytes: reads are bounded by the program.
		{"a format 0 subtable cut short inside the program", full(ttCmap(fmt0Short), ttPost3(), ttHmtx(ttGlyphs), ttGlyphs), true},
		{"a format 2.5 post indexing within the Mac names", full(std, p25(0), ttHmtx(ttGlyphs), ttGlyphs), true},
		{"a format 2.5 post indexing up to name 226", full(std, p25(0x7f), ttHmtx(ttGlyphs), ttGlyphs), true},
		{"a format 2.5 post indexing below the Mac names", full(std, p25(0xFF), ttHmtx(ttGlyphs), ttGlyphs), false},
		{"an hmtx shorter than hhea says", full(std, ttPost3(), ttHmtx(10), 50000), false},
		{"no head, maxp or post", sfnt(ttTable{"cmap", std}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}), true},
		{"no hhea", sfnt(ttTable{"cmap", std}, ttTable{"head", ttHead()}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(ttGlyphs)}), false},
		{"a subtable past the program's end", full(beBytes(uint16(0), uint16(1), uint16(3), uint16(1), uint32(900000)), ttPost3(), ttHmtx(ttGlyphs), ttGlyphs), false},
		{"a format 12 subtable, which veraPDF skips", full(ttCmap(sub31, ttSub{3, 10, beBytes(uint16(12), uint16(0), uint32(16), uint32(0), uint32(0))}), ttPost3(), ttHmtx(ttGlyphs), ttGlyphs), true},
		{"an OTTO signature", otto, true},
		{"five bytes", []byte{0, 1, 0, 0, 0}, false},
		{"a directory cut short", ttProgram(sub31, sub10)[:12+16*2+5], false},
		{"a second cmap entry that is cut short", sfnt(ttTable{"cmap", std}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"cmap", ttCmap(cutFmt0)}), false},
		{"a first cmap entry that is cut short", sfnt(ttTable{"cmap", ttCmap(cutFmt0)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"cmap", ttCmap(sub31)}), true},
		{"a format 2 post", post2, true},
		{"a format 2 post whose length runs past the end", withTableField(post2, "post", 12, func(v uint32) uint32 { return v + 100 }), false},
		{"an hhea past the program's end", withTableField(ttProgram(sub31, sub10), "hhea", 8, past), false},
		{"a program over 10240 bytes", big, true},
		{"no horizontal metrics at all", full(std, ttPost3(), nil, 0), true},
		{"a format 4 range offset past the end", full(ttCmap(ttSub{3, 1, f4}), ttPost3(), ttHmtx(ttGlyphs), ttGlyphs), false},
		// The two edges of a read at the program's END, the table directory saying nothing about either: a glyph-index
		// array that ends exactly there parses, and one byte shorter it does not; an empty hmtx sitting at the end parses.
		// The blind pass's three survivors, each an edge nothing else reached.
		{"a format 2.5 post indexing name 258, one past the last", sfnt(ttTable{"cmap", std}, ttTable{"head", ttHead()},
			ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(200)}, ttTable{"post", post25At(200, 131, 127)}), false},
		{"a format 2.5 post with no maxp, walked over hhea's count", sfnt(ttTable{"cmap", std}, ttTable{"head", ttHead()},
			ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"post", post25At(ttGlyphs, 0, 0xFF)}), false},
		{"a one-code format 4 segment whose range offset runs past the end", full(ttCmap(ttSub{3, 1, oneCode}), ttPost3(), ttHmtx(ttGlyphs), ttGlyphs), false},
		{"format 4 glyph indices ending exactly at the program's end", fmt4AtEnd(0), true},
		{"format 4 glyph indices cut one byte short by the program's end", fmt4AtEnd(1), false},
		{"an empty hmtx at the program's end", sfnt(ttTable{"cmap", std}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(0)}, ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"hmtx", nil}), true},
	}
}

// post25At is a format 2.5 post of n offsets, all zero but the one at `at`.
func post25At(n, at int, b byte) []byte {
	p := append(beBytes(uint32(0x00028000)), make([]byte, 28)...)
	offs := make([]byte, n)
	offs[at] = b
	return append(p, offs...)
}

// fmt4AtEnd is a program whose last table is a cmap with one format 4 subtable mapping codes 0x20-0x23 through a
// glyph-index array (range offset 4) that ends at the program's last byte — less `cut` bytes.
func fmt4AtEnd(cut int) []byte {
	f4 := beBytes(uint16(4), uint16(40), uint16(0), uint16(4), uint16(0), uint16(0), uint16(0),
		uint16(0x23), uint16(0xFFFF), uint16(0), uint16(0x20), uint16(0xFFFF), uint16(0), uint16(1), uint16(4), uint16(0),
		uint16(1), uint16(2), uint16(3), uint16(4))
	p := sfnt(ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"cmap", ttCmap(ttSub{3, 1, f4})})
	return p[:len(p)-cut]
}

// ttFixture is one measured document. `vera` is veraPDF's verdict on 7.21.6 t1, t2, t3, t4, 7.21.4.1 t1, 7.21.7 t1
// and t2, in that order — P, F, or - for no subject; `refused` holds, per position, the words nib's CannotCheck
// must carry where nib deliberately refuses instead (a "." keeps veraPDF's answer).
type ttFixture struct {
	name    string
	pdf     []byte
	vera    string
	refused []string
}

var ttClauses = []string{"7.21.6 t1", "7.21.6 t2", "7.21.6 t3", "7.21.6 t4", "7.21.4.1 t1", "7.21.7 t1", "7.21.7 t2"}

// ttFixtures is every measured shape with a veraPDF report.
func ttFixtures() []ttFixture {
	std := ttProgram(sub31, sub10)
	noHhea := sfnt(ttTable{"cmap", ttCmap(sub31)}, ttTable{"head", ttHead()}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(ttGlyphs)})
	noCmap := sfnt(ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(ttGlyphs)})
	winDiffs := "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /A /B /C] >>"
	shared := "fonts share one TrueType program"
	fused := "merged identical font dictionaries"
	fx := []ttFixture{
		{name: "non-symbolic, WinAnsi, (3,1) and (1,0)", pdf: ttDoc("/Flags 32", "/Encoding /WinAnsiEncoding", "(ABC) Tj", std)},
		{name: "non-symbolic, MacRoman", pdf: ttDoc("/Flags 32", "/Encoding /MacRomanEncoding", "(ABC) Tj", std)},
		{name: "non-symbolic, no encoding", pdf: ttDoc("/Flags 32", "", "(ABC) Tj", std)},
		{name: "non-symbolic, only a (3,0) subtable", pdf: ttDoc("/Flags 32", "/Encoding /WinAnsiEncoding", "(ABC) Tj", ttProgram(sub30))},
		{name: "non-symbolic, no cmap table", pdf: ttDoc("/Flags 32", "/Encoding /WinAnsiEncoding", "(ABC) Tj", noCmap)},
		{name: "non-symbolic, a cmap of no subtables", pdf: ttDoc("/Flags 32", "/Encoding /WinAnsiEncoding", "(ABC) Tj", ttProgram())},
		{name: "non-symbolic, (3,0) and (3,1)", pdf: ttDoc("/Flags 32", "/Encoding /WinAnsiEncoding", "(ABC) Tj", ttProgram(sub30, sub31))},
		{name: "non-symbolic, (3,1) twice", pdf: ttDoc("/Flags 32", "/Encoding /WinAnsiEncoding", "(ABC) Tj", ttProgram(sub31, sub31))},
		{name: "non-symbolic, named MacExpert", pdf: ttDoc("/Flags 32", "/Encoding /MacExpertEncoding", "(ABC) Tj", std)},
		{name: "non-symbolic, named Standard", pdf: ttDoc("/Flags 32", "/Encoding /StandardEncoding", "(A) Tj", std)},
		{name: "non-symbolic, a MacExpert base", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding /MacExpertEncoding >>", "(ABC) Tj", std)},
		{name: "non-symbolic, Differences in the glyph list", pdf: ttDoc("/Flags 32", winDiffs, "(ABC) Tj", std)},
		{name: "non-symbolic, a Differences name outside the glyph list", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /foo123] >>", "(ABC) Tj", std)},
		{name: "non-symbolic, Differences naming .notdef", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /.notdef] >>", "(ABC) Tj", std)},
		{name: "non-symbolic, Differences with no (3,1) subtable", pdf: ttDoc("/Flags 32", winDiffs, "(ABC) Tj", ttProgram(sub10))},
		{name: "non-symbolic, a Differences that is not an array", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences 5 >>", "(ABC) Tj", std)},
		{name: "non-symbolic, a Differences code past 255", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [300 /A] >>", "(A) Tj", std)},
		{name: "non-symbolic, a base named by a string", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding (WinAnsiEncoding) >>", "(ABC) Tj", std)},
		{name: "non-symbolic, a base named by a string, code 0x80", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding (WinAnsiEncoding) >>", "<80> Tj", std)},
		{name: "non-symbolic, a dictionary with no base", pdf: ttDoc("/Flags 32", "/Encoding << /Differences [65 /A] >>", "(ABC) Tj", std)},
		{name: "non-symbolic, a dictionary with no base, code 0x80", pdf: ttDoc("/Flags 32", "/Encoding << /Differences [65 /A] >>", "<80> Tj", std)},
		{name: "non-symbolic, a dictionary with no base, code 0x27", pdf: ttDoc("/Flags 32", "/Encoding << /Differences [65 /A] >>", "<27> Tj", std)},
		{name: "non-symbolic, a dictionary with no base, a program that does not parse", pdf: ttDoc("/Flags 32", "/Encoding << /Differences [65 /A] >>", "(ABC) Tj", noHhea)},
		{name: "non-symbolic, WinAnsi, a program that does not parse", pdf: ttDoc("/Flags 32", "/Encoding /WinAnsiEncoding", "(ABC) Tj", noHhea)},
		{name: "non-symbolic, Differences, a program that does not parse", pdf: ttDoc("/Flags 32", winDiffs, "(ABC) Tj", noHhea)},
		{name: "non-symbolic, no program, Differences outside the glyph list", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /foo123] >>", "(ABC) Tj", nil)},
		{name: "no /Flags", pdf: ttDoc("", "/Encoding /WinAnsiEncoding", "(ABC) Tj", std)},
		{name: "symbolic, only (3,0)", pdf: ttDoc("/Flags 4", "", "(ABC) Tj", ttProgram(sub30))},
		{name: "symbolic, only (3,1)", pdf: ttDoc("/Flags 4", "", "(ABC) Tj", ttProgram(sub31))},
		{name: "symbolic, (3,1) and (1,0)", pdf: ttDoc("/Flags 4", "", "(ABC) Tj", std)},
		{name: "symbolic, (3,0) and (1,0)", pdf: ttDoc("/Flags 4", "", "(ABC) Tj", ttProgram(sub30, sub10))},
		{name: "symbolic, (3,1) twice", pdf: ttDoc("/Flags 4", "", "(ABC) Tj", ttProgram(sub31, sub31))},
		{name: "symbolic, a cmap of no subtables", pdf: ttDoc("/Flags 4", "", "(ABC) Tj", ttProgram())},
		{name: "symbolic and non-symbolic flags both", pdf: ttDoc("/Flags 36", "", "(ABC) Tj", std)},
		{name: "symbolic, WinAnsi", pdf: ttDoc("/Flags 4", "/Encoding /WinAnsiEncoding", "(ABC) Tj", ttProgram(sub30))},
		{name: "symbolic, named Standard", pdf: ttDoc("/Flags 4", "/Encoding /StandardEncoding", "(A) Tj", std)},
		{name: "symbolic, a dictionary with no base", pdf: ttDoc("/Flags 4", "/Encoding << /Differences [65 /A] >>", "(ABC) Tj", ttProgram(sub30))},
		{name: "symbolic, a dictionary with a WinAnsi base", pdf: ttDoc("/Flags 4", "/Encoding << /BaseEncoding /WinAnsiEncoding >>", "(ABC) Tj", ttProgram(sub30))},
		{name: "symbolic, WinAnsi, no program", pdf: ttDoc("/Flags 4", "/Encoding /WinAnsiEncoding", "(ABC) Tj", nil)},
		{name: "symbolic, a program that does not parse", pdf: ttDoc("/Flags 4", "", "(ABC) Tj", noHhea)},
		{name: "the program under /FontFile3 /OpenType", pdf: glyphDoc(ttFontDict("/Encoding /WinAnsiEncoding"), "(ABC) Tj", ttObjects("/Flags 32", "FontFile3", std, "/Subtype /OpenType"))},
		{name: "a program under /FontFile3 that does not parse", pdf: glyphDoc(ttFontDict("/Encoding /WinAnsiEncoding"), "(ABC) Tj", ttObjects("/Flags 32", "FontFile3", noHhea, "/Subtype /OpenType"))},
		// One font, one object: the second render mode is a second chance at the cached parse, and still no subject.
		{name: "named MacExpert, drawn in modes 0 and 2", pdf: ttPage("/Flags 32", "/Encoding /MacExpertEncoding", "BT /F0 12 Tf 10 10 Td (A) Tj 2 Tr (A) Tj ET", std)},
		{name: "named MacExpert, drawn in mode 0 then 3", pdf: ttPage("/Flags 32", "/Encoding /MacExpertEncoding", "BT /F0 12 Tf 10 10 Td 0 Tr (A) Tj 3 Tr (A) Tj ET", std)},
		// The share group: one parse, the first font opened decides it.
		{name: "two fonts share a program, both named MacExpert", pdf: ttShared("/Encoding /MacExpertEncoding", "/Encoding /MacExpertEncoding", false, std)},
		{name: "two fonts share a program, MacExpert then WinAnsi", pdf: ttShared("/Encoding /MacExpertEncoding", "/Encoding /WinAnsiEncoding", false, std),
			refused: []string{".", ".", ".", ".", shared, ".", "."}},
		{name: "two fonts share a program, WinAnsi then MacExpert", pdf: ttShared("/Encoding /WinAnsiEncoding", "/Encoding /MacExpertEncoding", false, std),
			refused: []string{".", ".", ".", ".", shared, ".", "."}},
		// Byte-identical fonts reach nib as ONE (pdfcpu fuses them), where veraPDF opens two and the second parses.
		// Which font is opened first decides which one is unparsed, and here that decides 7.21.4.1 t1: F0 is drawn
		// invisibly and F1 visibly, so nib refuses.
		{name: "two fonts share a program, both MacExpert, the first drawn invisibly", pdf: glyphPage("BT /F0 12 Tf 10 10 Td 3 Tr (A) Tj 0 Tr /F1 12 Tf (A) Tj ET", "/F1 11 0 R", "",
			ttFontDict("/Encoding /MacExpertEncoding"), mergeObjs(ttObjects("/Flags 32", "FontFile2", std, ""),
				map[int]string{11: strings.Replace(ttFontDict("/Encoding /MacExpertEncoding"), "/BaseFont /Probe", "/BaseFont /ProbeTwo", 1)})),
			refused: []string{".", ".", ".", ".", shared, ".", "."}},
		{name: "two identical fonts share a program, both named MacExpert", pdf: ttShared("/Encoding /MacExpertEncoding", "/Encoding /MacExpertEncoding", true, std),
			refused: []string{fused, ".", ".", fused, fused, ".", "."}},
	}
	// The P07.S03 review's shapes. /Flags is read as a long, a real cast (4.0 and 36.7 are symbolic); a /FontFile3
	// program's name-table throw leaves EVERY sharing font unparsed; the subset flag in the OpenType key is "six
	// characters before the first plus"; a /Differences code wraps at 32 bits.
	fx = append(fx,
		ttFixture{name: "/Flags 4.0, WinAnsi, only (3,1)", pdf: ttDoc("/Flags 4.0", "/Encoding /WinAnsiEncoding", "(ABC) Tj", ttProgram(sub31))},
		ttFixture{name: "/Flags 36.7, WinAnsi, (3,1) and (1,0)", pdf: ttDoc("/Flags 36.7", "/Encoding /WinAnsiEncoding", "(ABC) Tj", std)},
		ttFixture{name: "two fonts share an OpenType program, both named MacExpert, no subtables",
			pdf: ttSharedAs("FontFile3", "/Subtype /OpenType", "Probe", "/Encoding /MacExpertEncoding", "ProbeTwo", "/Encoding /MacExpertEncoding", ttProgram())},
		ttFixture{name: "two fonts share a TrueType program, both named MacExpert, no subtables",
			pdf: ttSharedAs("FontFile2", "", "Probe", "/Encoding /MacExpertEncoding", "ProbeTwo", "/Encoding /MacExpertEncoding", ttProgram())},
		ttFixture{name: "an OpenType program shared by ABCDEF+ (WinAnsi) and abcdef+ (MacExpert)",
			pdf:     ttSharedAs("FontFile3", "/Subtype /OpenType", "ABCDEF+Probe", "/Encoding /WinAnsiEncoding", "abcdef+Two", "/Encoding /MacExpertEncoding", ttProgram(sub31)),
			refused: []string{shared, ".", ".", shared, shared, ".", "."}},
		ttFixture{name: "an OpenType program shared by ABCDEF+ (WinAnsi) and Probe6 (MacExpert)",
			pdf:     ttSharedAs("FontFile3", "/Subtype /OpenType", "ABCDEF+Probe", "/Encoding /WinAnsiEncoding", "Probe6", "/Encoding /MacExpertEncoding", ttProgram(sub31)),
			refused: []string{shared, ".", ".", shared, shared, ".", "."}},
		ttFixture{name: "a Differences code past 2^32", pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [4294967361 /zzbad] >>", "(A) Tj", std)},
	)
	for _, b := range brokenPrograms() {
		fx = append(fx,
			ttFixture{name: "non-symbolic, Differences: " + b.name, pdf: ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /A] >>", "(ABC) Tj", b.prog)},
			ttFixture{name: "symbolic: " + b.name, pdf: ttDoc("/Flags 4", "", "(ABC) Tj", b.prog)})
	}
	return fx
}

// TestTheTrueTypeClausesAgreeWithVeraPDF — every shape above against veraPDF 1.30.2's verdicts, measured before the
// rules were written (`--passed`, so a clause with no subject reads "-"). The live oracle asks veraPDF again on every
// run (`oracleCorpus`); this table is what the package asserts when veraPDF is not installed.
func TestTheTrueTypeClausesAgreeWithVeraPDF(t *testing.T) {
	want := map[byte]Verdict{'P': Pass, 'F': Fail, '-': NotApplicable}
	fx := ttFixtures()
	if len(fx) != len(ttVeraPDF) {
		t.Fatalf("%d fixtures and %d measured rows", len(fx), len(ttVeraPDF))
	}
	for i, f := range fx {
		for j, clause := range ttClauses {
			got := verdictOf(t, f.pdf, clause)
			if f.refused != nil && f.refused[j] != "." {
				if got.Verdict != CannotCheck || !strings.Contains(got.Why, f.refused[j]) {
					t.Errorf("%s: %s reports %v (%s), want CannotCheck naming %q (veraPDF %c)", f.name, clause, got.Verdict, got.Why, f.refused[j], ttVeraPDF[i][j])
				}
				continue
			}
			if w := want[ttVeraPDF[i][j]]; got.Verdict != w {
				t.Errorf("%s: %s reports %v (%s), veraPDF %c", f.name, clause, got.Verdict, got.Why, ttVeraPDF[i][j])
			}
		}
	}
}

// ttVeraPDF is veraPDF 1.30.2's answer for each of ttFixtures, in order, over ttClauses.
var ttVeraPDF = []string{
	"PPPPPPP",
	"PPPPPPP",
	"PFPPPFP",
	"FPPPPPP",
	"FPPPPPP",
	"FPPPPPP",
	"PPPPPPP",
	"PPPPPPP",
	"-FP-FPF",
	"-FP-FFP",
	"PFPPPPF",
	"PPPPPPP",
	"PFPPPFP",
	"PPPPPPF",
	"PFPPPPP",
	"PPPPPPP",
	"PPPPPPP",
	"PPPPPPP",
	"PPPPPPF",
	"PFPPPPP",
	"PFPPPPF",
	"PFPPPPP",
	"-FP-FFP",
	"-PP-FPP",
	"-FP-FPP",
	"-PP-FFP",
	"PPPPPPP",
	"PPPPPFP",
	"PPPPPFP",
	"PPPFPFP",
	"PPPPPFP",
	"PPPFPFP",
	"PPPFPFP",
	"PPPFPFP",
	"PPFPPPP",
	"PPFFPFP",
	"PPPPPFP",
	"PPFPPPP",
	"-PF-FPP",
	"-PP-FFP",
	"PPPPPPP",
	"-PP-FPP",
	"-FP-FPF",
	"-FP-FPF",
	"PFPPFPF",
	"PFPPFPF",
	"PFPPPPF",
	"PFPPPPF",
	"PFPPFPF",
	"PPFPPPP",
	"PPFFPPP",
	"-FP-FPF",
	"FFPPFPF",
	"PFPPPPF",
	"PFPPPPF",
	"PFPPPFP",
	"PPPPPPP",
	"PPPPPFP",
	"PPPPPPP",
	"PPPFPFP",
	"PPPPPPP",
	"PPPFPFP",
	"-FP-FPP",
	"-PP-FFP",
	"-FP-FPP",
	"-PP-FFP",
	"PPPPPPP",
	"PPPFPFP",
	"-FP-FPP",
	"-PP-FFP",
	"-FP-FPP",
	"-PP-FFP",
	"PPPPPPP",
	"PPPFPFP",
	"PPPPPPP",
	"PPPFPFP",
	"-FP-FPP",
	"-PP-FFP",
	"-FP-FPP",
	"-PP-FFP",
	"-FP-FPP",
	"-PP-FFP",
	"PPPPPPP",
	"PPPPPFP",
	"PPPPPPP",
	"PPPFPFP",
	"-FP-FPP",
	"-PP-FFP",
	"-FP-FPP",
	"-PP-FFP",
	"PPPPPPP",
	"PPPFPFP",
	"PPPPPPP",
	"PPPFPFP",
	"-FP-FPP",
	"-PP-FFP",
	"-FP-FPP",
	"-PP-FFP",
	"-FP-FPP",
	"-PP-FFP",
	"-FP-FPP",
	"-PP-FFP",
	"PPPPPPP",
	"PPPPPFP",
	"-FP-FPP",
	"-PP-FFP",
	"PPPPPPP",
	"PPPFPFP",
}

// TestWhereVeraPDFReportsNothingNibRefuses — two shapes veraPDF answers with NO report at all, because an exception
// escapes its handling: a program of 10240 bytes or more (read through a temporary file) whose table lies past its
// end, and a /Differences naming a negative code. Measured: `jobEndStatus` with no validation report. There is no
// answer to agree with, so every clause that would have to guess refuses and names why.
func TestWhereVeraPDFReportsNothingNibRefuses(t *testing.T) {
	big := sfnt(ttTable{"cmap", ttCmap(sub31, sub10)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
		ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"glyf", make([]byte, 12000)})
	bigPast := withTableField(big, "hhea", 8, func(uint32) uint32 { return 900000 })
	for _, c := range []struct {
		name    string
		pdf     []byte
		clauses []string
	}{
		{"a large non-symbolic program whose hhea lies past its end", ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /A] >>", "(ABC) Tj", bigPast),
			[]string{"7.21.6 t1", "7.21.6 t2", "7.21.6 t4", "7.21.4.1 t1"}},
		{"a large symbolic program whose hhea lies past its end", ttDoc("/Flags 4", "", "(ABC) Tj", bigPast),
			[]string{"7.21.6 t1", "7.21.6 t4", "7.21.4.1 t1"}},
		{"a Differences index that steps past 2^31", ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [2147483647 /A /B] >>", "(A) Tj", ttProgram(sub31, sub10)),
			[]string{"7.21.6 t1", "7.21.6 t2", "7.21.6 t4", "7.21.4.1 t1"}},
		{"a negative Differences code", ttDoc("/Flags 32", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [-5 /A] >>", "(A) Tj", ttProgram(sub31, sub10)),
			[]string{"7.21.6 t1", "7.21.6 t2", "7.21.6 t4", "7.21.4.1 t1"}},
	} {
		for _, clause := range c.clauses {
			if got := verdictOf(t, c.pdf, clause); got.Verdict != CannotCheck || !strings.Contains(got.Why, "reports nothing") {
				t.Errorf("%s: %s reports %v (%s), want CannotCheck saying veraPDF reports nothing", c.name, clause, got.Verdict, got.Why)
			}
		}
	}
	// The boundary is 10240 itself: veraPDF reads from memory only BELOW it (measured at the review: exactly 10240 bytes
	// with hhea past the end reports nothing, 10239 fails the parse).
	withGlyf := func(n int) []byte {
		return sfnt(ttTable{"cmap", ttCmap(sub31, sub10)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
			ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"glyf", make([]byte, n)})
	}
	exact := withTableField(withGlyf(10240-len(withGlyf(0))), "hhea", 8, func(uint32) uint32 { return 900000 })
	if len(exact) != 10240 {
		t.Fatalf("the boundary program is %d bytes, want 10240", len(exact))
	}
	if p := readTrueType(exact, maxTrueTypeReads); p.state != ttUnknown {
		t.Errorf("a 10240-byte program whose hhea lies past its end is %v (%s), want unknown", p.state, p.why)
	}
	if p := readTrueType(exact[:10239], maxTrueTypeReads); p.state != ttFailed {
		t.Errorf("a 10239-byte program whose hhea lies past its end is %v (%s), want failed", p.state, p.why)
	}
	// The same program under 10240 bytes is an ordinary failed parse: veraPDF reads it from memory and throws an
	// exception it handles, so t1 has no subject and 7.21.4.1 t1 fails.
	small := withTableField(ttProgram(sub31, sub10), "hhea", 8, func(uint32) uint32 { return 900000 })
	pdf := ttDoc("/Flags 32", "/Encoding /WinAnsiEncoding", "(ABC) Tj", small)
	if got := verdictOf(t, pdf, "7.21.6 t1"); got.Verdict != NotApplicable {
		t.Errorf("a small program whose hhea lies past its end: 7.21.6 t1 reports %v (%s), want NotApplicable", got.Verdict, got.Why)
	}
	if got := verdictOf(t, pdf, "7.21.4.1 t1"); got.Verdict != Fail {
		t.Errorf("a small program whose hhea lies past its end: 7.21.4.1 t1 reports %v (%s), want Fail", got.Verdict, got.Why)
	}
}

// TestAProgramPastTheReadBudgetIsUnknown — a cmap whose subtable records all point at one format 4 subtable of
// 32,767 segments costs veraPDF billions of reads; nib stops at `maxTrueTypeReads` and says so, and the same program
// with one record parses.
func TestAProgramPastTheReadBudgetIsUnknown(t *testing.T) {
	const segs = 32767
	body := beBytes(uint16(4), uint16(0), uint16(0), uint16(segs*2), uint16(0), uint16(0), uint16(0))
	for i := 0; i < segs; i++ {
		body = append(body, beBytes(uint16(0xFFFF))...) // end codes
	}
	body = append(body, 0, 0)
	for k := 0; k < 3; k++ { // start codes, deltas, range offsets
		for i := 0; i < segs; i++ {
			body = append(body, beBytes(uint16(0xFFFF))...)
		}
	}
	cmap := func(records int) []byte {
		c := beBytes(uint16(0), uint16(records))
		off := uint32(4 + 8*records)
		for i := 0; i < records; i++ {
			c = append(c, beBytes(uint16(3), uint16(1), off)...)
		}
		return append(c, body...)
	}
	prog := func(records int) []byte {
		return sfnt(ttTable{"cmap", cmap(records)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)})
	}
	// A map entry costs `mapEntryCost` reads, so one segment mapping every code costs at least 65,536 of them (a 1 KB
	// program of such segments pinned 142 MiB at one read per entry — the P07.S04a review).
	wide := beBytes(uint16(4), uint16(32), uint16(0), uint16(4), uint16(0), uint16(0), uint16(0), uint16(0xFFFE), uint16(0xFFFF),
		uint16(0), uint16(0), uint16(0xFFFF), uint16(0), uint16(1), uint16(0), uint16(0))
	full := sfnt(ttTable{"cmap", ttCmap(ttSub{3, 1, wide})}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)})
	if p := readTrueType(full, maxTrueTypeReads); p.state != ttParsed || p.reads < 65535*mapEntryCost || mapEntryCost < 4 {
		t.Errorf("one segment over every code: state %v after %d reads, want parsed and charged at least %d (mapEntryCost %d)", p.state, p.reads, 65535*4, mapEntryCost)
	}
	if p := readTrueType(prog(1), maxTrueTypeReads); p.state != ttParsed || p.nrCmaps != 1 {
		t.Fatalf("one record: state %v (%s), %d subtables, want parsed with 1", p.state, p.why, p.nrCmaps)
	}
	p := readTrueType(prog(200), maxTrueTypeReads)
	if p.state != ttUnknown || !strings.Contains(p.why, "reads") {
		t.Fatalf("200 records over one 32,767-segment subtable: state %v (%s), want unknown past the read budget", p.state, p.why)
	}
}

// unresolvedBeside is a symbolic TrueType font naming WinAnsi with no program, drawn beside /F9, which the page's
// resources do not hold.
func unresolvedBeside() []byte {
	return ttPage("/Flags 4", "/Encoding /WinAnsiEncoding", "BT /F0 12 Tf 10 10 Td (A) Tj /F9 12 Tf (A) Tj ET", nil)
}

// TestAnUnresolvedFontDoesNotSilenceADefiniteFailure — the P07.S03 review measured veraPDF failing 7.21.4.1 t1 and
// 7.21.6 t3 on this document, where nib had refused every TrueType clause because /F9 might have been a TrueType font.
// A font nib read and veraPDF fails outranks a font nib could not read; the refusal stands only where nothing fails.
func TestAnUnresolvedFontDoesNotSilenceADefiniteFailure(t *testing.T) {
	pdf := unresolvedBeside()
	for _, clause := range []string{"7.21.4.1 t1", "7.21.6 t3"} {
		if got := verdictOf(t, pdf, clause); got.Verdict != Fail {
			t.Errorf("%s reports %v (%s), want Fail as veraPDF measured", clause, got.Verdict, got.Why)
		}
	}
	// Nothing fails t2 (the font is symbolic), so the unresolved font's refusal is the answer.
	if got := verdictOf(t, pdf, "7.21.6 t2"); got.Verdict != CannotCheck || !strings.Contains(got.Why, "does not resolve") {
		t.Errorf("7.21.6 t2 reports %v (%s), want CannotCheck naming the unresolved font", got.Verdict, got.Why)
	}
}

// TestFontsSharingAProgramParseItOnce — every font sharing one program stream is charged ONE parse against the
// document's budget. Before the P07.S03 review each font re-read the program with a full budget of its own (200 fonts
// on one program measured 3.4 s), and the share group compared every font's names with every other's.
func TestFontsSharingAProgramParseItOnce(t *testing.T) {
	prog := ttProgram(sub31, sub10)
	const n = 400
	objs := ttObjects("/Flags 32", "FontFile2", prog, "")
	var fonts, content strings.Builder
	content.WriteString("BT 10 10 Td ")
	for i := 0; i < n; i++ {
		obj := 100 + i
		objs[obj] = strings.Replace(ttFontDict("/Encoding /WinAnsiEncoding"), "/BaseFont /Probe", fmt.Sprintf("/BaseFont /P%d", i), 1)
		fmt.Fprintf(&fonts, "/G%d %d 0 R ", i, obj)
		fmt.Fprintf(&content, "/G%d 12 Tf (A) Tj ", i)
	}
	content.WriteString("ET")
	d, err := open(glyphPage(content.String(), fonts.String(), "", ttFontDict("/Encoding /WinAnsiEncoding"), objs))
	if err != nil {
		t.Fatal(err)
	}
	list, why := d.trueTypeFonts()
	if why != "" || len(list) < n {
		t.Fatalf("read %d fonts (%s), want at least %d", len(list), why, n)
	}
	if one := readTrueType(prog, maxTrueTypeReads).reads; d.ttReads != one {
		t.Errorf("%d fonts on one program cost %d reads, want one parse's %d", len(list), d.ttReads, one)
	}
}

// TestAnUnresolvedFontMayBeTheFirstToOpenASharedProgram — nib cannot tell whether a font it could not resolve shares a
// program with the fonts it did, and veraPDF opening that font FIRST would decide an OpenType group's parse and every
// non-symbolic group's glyph names. So beside an unresolved font those refuse, and a symbolic group — whose names are
// " " whoever is first — and a /FontFile2 group where nothing throws still answer.
func TestAnUnresolvedFontMayBeTheFirstToOpenASharedProgram(t *testing.T) {
	open3 := glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj /F9 12 Tf (A) Tj ET", "", "", ttFontDict("/Encoding /WinAnsiEncoding"),
		ttObjects("/Flags 32", "FontFile3", ttProgram(sub31, sub10), "/Subtype /OpenType"))
	if got := verdictOf(t, open3, "7.21.6 t1"); got.Verdict != CannotCheck || !strings.Contains(got.Why, "opened first") {
		t.Errorf("OpenType beside an unresolved font: 7.21.6 t1 reports %v (%s), want CannotCheck naming the first-opened font", got.Verdict, got.Why)
	}
	d, err := open(glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj /F9 12 Tf (A) Tj ET", "", "", ttFontDict("/Encoding << /Differences [66 /B] >>"),
		ttObjects("/Flags 32", "FontFile2", ttProgram(sub31, sub10), "")))
	if err != nil {
		t.Fatal(err)
	}
	if _, known, why := d.trueTypeFallbackName(d.ttList0(t), 'A'); known || !strings.Contains(why, "opened first") {
		t.Errorf("a non-symbolic font's fallback name beside an unresolved font is known=%v (%s), want unknown", known, why)
	}
	dt, err := open(ttPage("/Flags 32", "/Encoding /WinAnsiEncoding", "BT /F0 12 Tf 10 10 Td (A) Tj /F9 12 Tf (A) Tj ET", ttProgram(sub31, sub10)))
	if err != nil {
		t.Fatal(err)
	}
	if st, why := dt.trueTypeEmbedded(dt.ttList0(t)); st != ttParsed {
		t.Errorf("a /FontFile2 font where nothing throws is %v (%s) beside an unresolved font, want parsed", st, why)
	}
	ds, err := open(ttPage("/Flags 4", "", "BT /F0 12 Tf 10 10 Td (A) Tj /F9 12 Tf (A) Tj ET", ttProgram(sub30)))
	if err != nil {
		t.Fatal(err)
	}
	if name, known, why := ds.trueTypeFallbackName(ds.ttList0(t), 'A'); !known || name != "" {
		t.Errorf("a symbolic font's fallback name beside an unresolved font is %q known=%v (%s), want a known null", name, known, why)
	}
}

// ttList0 is the first TrueType font's dictionary, for tests that ask the door directly.
func (d *Document) ttList0(t *testing.T) types.Dict {
	t.Helper()
	list, why := d.trueTypeFonts()
	if why != "" || len(list) == 0 {
		t.Fatalf("no TrueType font read (%s)", why)
	}
	return list[0].dict
}

// TestFlagsAreReadAsJavaCastsThem — /Flags is `getIntegerKey`, a Java `(long)` cast of a real: truncation, NaN to 0,
// saturation at the ends; the bit is tested on the `intValue`. **A /Flags beyond 2^63 never reaches this** — pdfcpu's
// own parser reads `100000000000000000000.0` as the integer 0, where veraPDF saturates it to a symbolic font (the P07.S03
// re-review measured it failing t3 and t4). That is a reader limit, declared, not a rule nib can fix here (/pending 656).
func TestFlagsAreReadAsJavaCastsThem(t *testing.T) {
	for _, c := range []struct {
		f    float64
		want int64
	}{{4.9, 4}, {-4.9, -4}, {math.NaN(), 0}, {1e20, math.MaxInt64}, {-1e20, math.MinInt64}} {
		if got := javaLong(c.f); got != c.want {
			t.Errorf("javaLong(%v) = %d, want %d", c.f, got, c.want)
		}
	}
	if int32(javaLong(1e20))&4 == 0 {
		t.Error("a saturated /Flags must read symbolic, as veraPDF's intValue of Long.MAX_VALUE (-1) does")
	}
}

// TestTheReadBudgetIsTheDocuments — two DIFFERENT programs, each under the budget, together over it: the second is
// unknown, because the budget bounds what one document costs, not what one program does.
func TestTheReadBudgetIsTheDocuments(t *testing.T) {
	heavy := func(records int, tag byte) []byte {
		const segs = 32767
		body := beBytes(uint16(4), uint16(0), uint16(0), uint16(segs*2), uint16(0), uint16(0), uint16(0))
		body = append(body, bytes.Repeat([]byte{0xFF, 0xFF}, segs)...)
		body = append(body, 0, 0)
		body = append(body, bytes.Repeat([]byte{0xFF, 0xFF}, 3*segs)...)
		c := beBytes(uint16(0), uint16(records))
		for i := 0; i < records; i++ {
			c = append(c, beBytes(uint16(3), uint16(1), uint32(4+8*records))...)
		}
		return sfnt(ttTable{"cmap", append(c, body...)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
			ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"name", []byte{tag}})
	}
	a, b := heavy(10, 'a'), heavy(10, 'b')
	if p := readTrueType(a, maxTrueTypeReads); p.state != ttParsed || p.reads > maxTrueTypeReads*2/3 {
		t.Fatalf("one heavy program: %v after %d reads, want parsed under two thirds of the budget", p.state, p.reads)
	}
	objs := mergeObjs(ttObjects("/Flags 32", "FontFile2", a, ""), map[int]string{
		11: strings.Replace(ttFontDict("/Encoding /WinAnsiEncoding"), "12 0 R", "13 0 R", 1),
		13: strings.Replace(ttObjects("/Flags 32", "FontFile2", b, "")[12], "20 0 R", "21 0 R", 1),
		21: spStream(fmt.Sprintf("/Length1 %d", len(b)), string(b)),
	})
	d, err := open(glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj /F1 12 Tf (A) Tj ET", "/F1 11 0 R", "", ttFontDict("/Encoding /WinAnsiEncoding"), objs))
	if err != nil {
		t.Fatal(err)
	}
	list, _ := d.trueTypeFonts()
	if len(list) != 2 || list[0].program.state != ttParsed || list[1].program.state != ttUnknown {
		t.Fatalf("two heavy programs: %d fonts, states %v, want the first parsed and the second unknown past the document budget", len(list), func() []ttState {
			var s []ttState
			for _, f := range list {
				s = append(s, f.program.state)
			}
			return s
		}())
	}
}
