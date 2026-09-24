package uacheck

import (
	"encoding/binary"
	"strings"
	"testing"
)

// P07.S04a's fixtures: every shape of 7.21.5 t1, 7.21.4.1 t2 and 7.21.8 t1 the slice reads, run on veraPDF 1.30.2
// before the rules were written. The corpus holds one 7.21.5 pair and nothing else for the three over TrueType.

var metricClauses = []string{"7.21.5 t1", "7.21.4.1 t2", "7.21.8 t1"}

// mfDict is a simple TrueType font with `widths` (the /FirstChar, /LastChar and /Widths entries) and `extra`.
func mfDict(widths, extra string) string {
	return "<< /Type /Font /Subtype /TrueType /BaseFont /Probe " + widths + " /FontDescriptor 12 0 R " + extra + " >>"
}

// cidFontDoc is a Type 0 font over Identity-H whose CIDFontType2 descendant carries `cid` entries and `prog` (none when
// nil), showing `show` in `content`'s place.
func cidFontDoc(cid, content string, prog []byte, extra map[int]string) []byte {
	objs := map[int]string{
		11: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Probe /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> " +
			"/FontDescriptor 12 0 R " + cid + " >>",
	}
	for k, v := range ttObjects("/Flags 4", "FontFile2", prog, "") {
		objs[k] = v
	}
	for k, v := range extra {
		objs[k] = v
	}
	return glyphPage(content, "", "", "<< /Type /Font /Subtype /Type0 /BaseFont /Probe /Encoding /Identity-H /DescendantFonts [11 0 R] >>", objs)
}

// fmt4Segments is a format 4 subtable of (start, end, glyph-of-start) segments with no range offsets, and the sentinel.
func fmt4Segments(segs [][3]int) []byte {
	segs = append(segs, [3]int{0xFFFF, 0xFFFF, 0})
	n := len(segs)
	b := beBytes(uint16(4), uint16(0), uint16(0), uint16(2*n), uint16(0), uint16(0), uint16(0))
	for _, s := range segs {
		b = append(b, beBytes(uint16(s[1]))...)
	}
	b = append(b, 0, 0)
	for _, s := range segs {
		b = append(b, beBytes(uint16(s[0]))...)
	}
	for _, s := range segs {
		b = append(b, beBytes(uint16((s[2]-s[0])&0xFFFF))...)
	}
	for range segs {
		b = append(b, 0, 0)
	}
	binary.BigEndian.PutUint16(b[2:], uint16(len(b)))
	return b
}

// fmt4RangeToFFFF is a format 4 subtable whose one segment runs from 0x20 to 0xFFFF through a range offset pointing
// past its own end — veraPDF skips such a segment unread, so its codes are unmapped and the program parses.
func fmt4RangeToFFFF() []byte {
	return beBytes(uint16(4), uint16(32), uint16(0), uint16(4), uint16(0), uint16(0), uint16(0), uint16(0xFFFF), uint16(0xFFFF),
		uint16(0), uint16(0x20), uint16(0xFFFF), uint16(0), uint16(1), uint16(60000), uint16(0))
}

// postNamedCount is postNamed with a post table counting `n` glyphs (maxp says 100): the post's own count is the one read.
func postNamedCount(n int) []byte {
	p2 := append(beBytes(uint32(0x00020000)), make([]byte, 28)...)
	p2 = append(p2, beBytes(uint16(n))...)
	for i := 0; i < n; i++ {
		idx := 0
		if i == 5 {
			idx = 36
		}
		p2 = append(p2, beBytes(uint16(idx))...)
	}
	return sfnt(ttTable{"cmap", ttCmap()}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
		ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"post", p2})
}

// postCountThenData has a post counting 6 glyphs (maxp says 100) that names glyph 5 `A`, then a table whose bytes, read
// as 94 more name indices, would name glyph 50 `A` too — and glyph 50 is 700 wide where every other is 500. Reading the
// post's own count, `A` is glyph 5 and 500 wide.
func postCountThenData() []byte {
	p2 := append(beBytes(uint32(0x00020000)), make([]byte, 28)...)
	p2 = append(p2, beBytes(uint16(6))...)
	for i := 0; i < 6; i++ {
		idx := 0
		if i == 5 {
			idx = 36
		}
		p2 = append(p2, beBytes(uint16(idx))...)
	}
	more := make([]byte, 2*94)
	binary.BigEndian.PutUint16(more[2*(50-6):], 36)
	hm := ttHmtx(ttGlyphs)
	binary.BigEndian.PutUint16(hm[4*50:], 700)
	binary.BigEndian.PutUint16(hm[4*51:], 700) // the post's padding shifts the misread by one index
	return sfnt(ttTable{"cmap", ttCmap()}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
		ttTable{"hmtx", hm}, ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"post", p2}, ttTable{"zzzz", more})
}

// tailProgramGlyphs is tailProgram with maxp counting `n` glyphs.
func tailProgramGlyphs(n int) []byte {
	hm := ttHmtx(9)
	hm = append(hm, beBytes(uint16(700), int16(0))...)
	return sfnt(ttTable{"cmap", ttCmap(sub31, sub10)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(10)},
		ttTable{"hmtx", hm}, ttTable{"maxp", ttMaxp(n)}, ttTable{"post", ttPost3()})
}

// emptyPostNameNoCmap is emptyPostName with no cmap table at all.
func emptyPostNameNoCmap() []byte {
	p2 := append(beBytes(uint32(0x00020000)), make([]byte, 28)...)
	p2 = append(p2, beBytes(uint16(ttGlyphs))...)
	for i := 0; i < ttGlyphs; i++ {
		idx := 0
		if i == 5 {
			idx = 258
		}
		p2 = append(p2, beBytes(uint16(idx))...)
	}
	p2 = append(p2, 0)
	return sfnt(ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)},
		ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"post", p2})
}

// tailProgram has 100 glyphs but ten advances, the last 700: glyph 34 ('A' through (3,1)) takes the last.
func tailProgram() []byte {
	hm := ttHmtx(9)
	hm = append(hm, beBytes(uint16(700), int16(0))...)
	return sfnt(ttTable{"cmap", ttCmap(sub31, sub10)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(10)},
		ttTable{"hmtx", hm}, ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"post", ttPost3()})
}

// zeroWideProgram is the standard program with glyph 0 — the fallback glyph — 300 wide where every other is 500.
func zeroWideProgram() []byte {
	hm := beBytes(uint16(300), int16(0))
	hm = append(hm, ttHmtx(ttGlyphs-1)...)
	return sfnt(ttTable{"cmap", ttCmap(sub31, sub10)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
		ttTable{"hmtx", hm}, ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"post", ttPost3()})
}

// fmt0With is a format 0 subtable mapping every code to glyph 1 but `code`, mapped to `gid`.
func fmt0With(code, gid byte) []byte {
	b := beBytes(uint16(0), uint16(262), uint16(0))
	for i := 0; i < 256; i++ {
		g := byte(1)
		if byte(i) == code {
			g = gid
		}
		b = append(b, g)
	}
	return b
}

// postNamed has a cmap of no subtables and a format 2 post naming glyph 5 `A` (Macintosh name index 36).
func postNamed() []byte {
	p2 := append(beBytes(uint32(0x00020000)), make([]byte, 28)...)
	p2 = append(p2, beBytes(uint16(ttGlyphs))...)
	for i := 0; i < ttGlyphs; i++ {
		idx := 0
		if i == 5 {
			idx = 36
		}
		p2 = append(p2, beBytes(uint16(idx))...)
	}
	return sfnt(ttTable{"cmap", ttCmap()}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
		ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"post", p2})
}

// emptyPostName is a program whose format 2 post names glyph 5 with an EMPTY custom string — a name, where Java's null is
// none (the review's false pass).
func emptyPostName() []byte {
	p2 := append(beBytes(uint32(0x00020000)), make([]byte, 28)...)
	p2 = append(p2, beBytes(uint16(ttGlyphs))...)
	for i := 0; i < ttGlyphs; i++ {
		idx := 0
		if i == 5 {
			idx = 258
		}
		p2 = append(p2, beBytes(uint16(idx))...)
	}
	p2 = append(p2, 0)
	return sfnt(ttTable{"cmap", ttCmap(sub31, sub10)}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)},
		ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(ttGlyphs)}, ttTable{"post", p2})
}

// metricFixture is one measured document; `refused` holds, per metric clause, the words nib's CannotCheck must carry
// where it deliberately refuses ("." keeps veraPDF's answer).
type metricFixture struct {
	name    string
	pdf     []byte
	refused []string
}

func metricFixtures() []metricFixture {
	std := ttProgram(sub31, sub10)
	win := "/Encoding /WinAnsiEncoding"
	abc := "/FirstChar 65 /LastChar 67 /Widths [500 500 500]"
	doc := func(flags, font, show string, prog []byte) []byte {
		return glyphDoc(font, show, ttObjects(flags, "FontFile2", prog, ""))
	}
	page := func(flags, font, content string, prog []byte) []byte {
		return glyphPage(content, "", "", font, ttObjects(flags, "FontFile2", prog, ""))
	}
	missing := strings.Replace(ttObjects("/Flags 32", "FontFile2", std, "")[12], "/StemV 80", "/StemV 80 /MissingWidth 500", 1)
	noHead := sfnt(ttTable{"cmap", ttCmap(sub31, sub10)}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}, ttTable{"maxp", ttMaxp(ttGlyphs)})
	broken := withTableField(std, "hhea", 8, func(uint32) uint32 { return 900000 })
	cid := "BT /F0 12 Tf 10 10 Td <0021> Tj ET"
	mapStream := spStream("", string([]byte{0, 0, 0, 5, 0, 7})) // CIDs 0-2 only
	return []metricFixture{
		{name: "widths agree", pdf: doc("/Flags 32", mfDict(abc, win), "(ABC) Tj", std)},
		{name: "a width 100 off", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 67 /Widths [600 500 500]", win), "(ABC) Tj", std)},
		{name: "widths 1 off", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 67 /Widths [501 499 500]", win), "(ABC) Tj", std)},
		{name: "a width 1.5 off", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 67 /Widths [501.5 500 500]", win), "(ABC) Tj", std)},
		{name: "no /Widths", pdf: doc("/Flags 32", mfDict("", win), "(ABC) Tj", std)},
		{name: "/LastChar short of the codes", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 65 /Widths [500 500 500]", win), "(ABC) Tj", std)},
		{name: "/Widths short of /LastChar", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 67 /Widths [500]", win), "(ABC) Tj", std)},
		{name: "/MissingWidth standing in for /Widths", pdf: glyphDoc(mfDict("", win), "(ABC) Tj", mergeObjs(ttObjects("/Flags 32", "FontFile2", std, ""), map[int]string{12: missing}))},
		{name: "no head table, so 2048 units per em", pdf: doc("/Flags 32", mfDict(abc, win), "(ABC) Tj", noHead)},
		{name: "a width off, drawn invisibly", pdf: page("/Flags 32", mfDict("/FirstChar 65 /LastChar 67 /Widths [600 500 500]", win), "BT /F0 12 Tf 10 10 Td 3 Tr (A) Tj ET", std)},
		{name: "eacute, which no subtable maps", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 233 /Widths [500]", win), "<E9> Tj", std)},
		{name: "WinAnsi 0x81, bullet, absent", pdf: doc("/Flags 32", mfDict(abc, win), "<81> Tj", std)},
		{name: "code 0 under WinAnsi", pdf: doc("/Flags 32", mfDict("/FirstChar 0 /LastChar 0 /Widths [500]", win), "<00> Tj", std)},
		{name: "code 0 under WinAnsi, drawn invisibly", pdf: page("/Flags 32", mfDict("/FirstChar 0 /LastChar 0 /Widths [500]", win), "BT /F0 12 Tf 10 10 Td 3 Tr <00> Tj ET", std)},
		{name: "symbolic, (3,0), codes it maps", pdf: doc("/Flags 4", mfDict(abc, ""), "(ABC) Tj", ttProgram(sub30))},
		{name: "symbolic, (3,0), a code it does not map", pdf: doc("/Flags 4", mfDict("/FirstChar 16 /LastChar 16 /Widths [500]", ""), "<10> Tj", ttProgram(sub30))},
		{name: "symbolic, only (3,1)", pdf: doc("/Flags 4", mfDict(abc, ""), "(ABC) Tj", ttProgram(sub31))},
		{name: "an absent glyph drawn invisibly", pdf: page("/Flags 32", mfDict("/FirstChar 65 /LastChar 233 /Widths [500]", win), "BT /F0 12 Tf 10 10 Td 3 Tr <E9> Tj ET", std)},
		{name: "no program, code 0", pdf: doc("/Flags 32", mfDict("/FirstChar 0 /LastChar 0 /Widths [500]", win), "<00> Tj", nil)},
		{name: "no program, 0x81", pdf: doc("/Flags 32", mfDict(abc, win), "<81> Tj", nil)},
		{name: "non-symbolic, no /Encoding", pdf: doc("/Flags 32", mfDict(abc, ""), "(ABC) Tj", std)},
		{name: "a program veraPDF cannot parse, widths off", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 67 /Widths [900 900 900]", win), "(ABC) Tj", broken)},
		{name: "a MacExpert base, whose 0x41 is refilled from Standard", pdf: doc("/Flags 32", mfDict(abc, "/Encoding << /BaseEncoding /MacExpertEncoding >>"), "(ABC) Tj", std)},
		{name: "Helvetica, not embedded, 0x81", pdf: glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>", "<81> Tj", nil)},
		{name: "Helvetica, not embedded, code 0", pdf: glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>", "<00> Tj", nil)},
		// The review's shapes: a font whose name table throws, drawn in two render modes — judged (the second font object
		// marks it parsed) for /FontFile2 on the page, NOT for /FontFile3, and not measured-explained inside a form.
		{name: "MacExpert-named, modes 0 and 2", pdf: page("/Flags 32", mfDict("/FirstChar 65 /LastChar 67 /Widths [900 900 900]", "/Encoding /MacExpertEncoding"), "BT /F0 12 Tf 10 10 Td (A) Tj 2 Tr (A) Tj ET", std)},
		{name: "MacExpert-named, modes 0 and 2, /FontFile3", pdf: glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj 2 Tr (A) Tj ET", "", "", mfDict("/FirstChar 65 /LastChar 67 /Widths [900 900 900]", "/Encoding /MacExpertEncoding"), ttObjects("/Flags 32", "FontFile3", std, "/Subtype /OpenType"))},
		{name: "MacExpert-named, mode 0 on the page and 2 in a form", pdf: glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj ET /X0 Do", "", "/XObject << /X0 40 0 R >>",
			mfDict("/FirstChar 65 /LastChar 67 /Widths [900 900 900]", "/Encoding /MacExpertEncoding"),
			mergeObjs(ttObjects("/Flags 32", "FontFile2", std, ""), map[int]string{40: spForm("/Resources << /Font << /F0 10 0 R >> >>", "BT /F0 12 Tf 2 Tr (A) Tj ET")})),
			refused: []string{"inside a form", "inside a form", "."}},
		{name: "MacExpert-named, modes 0 and 2, an empty post name", pdf: page("/Flags 32", mfDict("/FirstChar 65 /LastChar 67 /Widths [500 500 500]", "/Encoding /MacExpertEncoding"), "BT /F0 12 Tf 10 10 Td (A) Tj 2 Tr (A) Tj ET", emptyPostName())},
		// The red-proof's survivors, each an edge of a lookup nothing else reached.
		{name: "a glyph past the advances but below the glyph count takes the last advance", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 65 /Widths [700]", win), "(A) Tj", tailProgram())},
		{name: "symbolic, a (1,0) subtable mapping the code to glyph 0", pdf: doc("/Flags 4", mfDict(abc, ""), "(A) Tj", ttProgram(ttSub{1, 0, fmt0With(0x41, 0)}))},
		{name: "non-symbolic, only a (1,0) subtable, found by Mac OS Roman code", pdf: doc("/Flags 32", mfDict(abc, win), "(ABC) Tj", ttProgram(sub10))},
		{name: "non-symbolic, no subtable, the glyph named by post", pdf: doc("/Flags 32", mfDict(abc, win), "(A) Tj", postNamed())},
		{name: "a Differences name no subtable maps, over glyph 0's advance", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 65 /Widths [300]", "/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [65 /eacute] >>"), "(A) Tj", zeroWideProgram())},
		{name: "symbolic, WinAnsi named, code 0", pdf: doc("/Flags 4", mfDict("/FirstChar 0 /LastChar 0 /Widths [500]", win), "<00> Tj", ttProgram(sub30))},
		{name: "no program, no encoding, code 0", pdf: doc("/Flags 32", mfDict("/FirstChar 0 /LastChar 0 /Widths [500]", ""), "<00> Tj", nil)},
		{name: "MacExpert-named, mode 0 on the page and 2 in a pattern", pdf: glyphPage("BT /F0 12 Tf 10 10 Td (A) Tj ET /Pattern cs /P0 scn 0 0 10 10 re f", "", "/Pattern << /P0 40 0 R >>",
			mfDict("/FirstChar 65 /LastChar 67 /Widths [900 900 900]", "/Encoding /MacExpertEncoding"),
			mergeObjs(ttObjects("/Flags 32", "FontFile2", std, ""), map[int]string{40: spStream("/Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << /Font << /F0 10 0 R >> >>", "BT /F0 12 Tf 2 Tr (A) Tj ET")})),
			refused: []string{"a pattern", "a pattern", "."}},
		// The blind pass's survivors.
		{name: "two (3,1) records, the first mapping the code to glyph 0", pdf: doc("/Flags 32", mfDict(abc, win), "(A) Tj", ttProgram(ttSub{3, 1, fmt0With(0x41, 0)}, sub31))},
		{name: "symbolic, a (3,0) whose first code is 0 and whose glyphs sit at F020", pdf: doc("/Flags 4", mfDict(abc, ""), "(A) Tj", ttProgram(ttSub{3, 0, fmt4Segments([][3]int{{0, 0, 1}, {0xF020, 0xF07E, 2}})}))},
		{name: "a range-offset segment ending at FFFF, which veraPDF skips", pdf: doc("/Flags 32", mfDict(abc, win), "(A) Tj", ttProgram(ttSub{3, 1, fmt4RangeToFFFF()}))},
		{name: "a post table counting fewer glyphs than maxp", pdf: doc("/Flags 32", mfDict(abc, win), "(A) Tj", postNamedCount(6))},
		{name: "a post table counting fewer glyphs than maxp, more data after it", pdf: doc("/Flags 32", mfDict(abc, win), "(A) Tj", postCountThenData())},
		{name: "a glyph equal to the glyph count takes advance 0", pdf: doc("/Flags 32", mfDict("/FirstChar 65 /LastChar 65 /Widths [500]", win), "(A) Tj", tailProgramGlyphs(34))},
		{name: "symbolic, a (3,0) in the F200 page", pdf: doc("/Flags 4", mfDict(abc, ""), "(ABC) Tj", ttProgram(ttSub{3, 0, cmapFmt4(0xF220, 95)}))},
		{name: "MacExpert-named, modes 0 and 2, no cmap table, an empty post name", pdf: page("/Flags 32", mfDict("/FirstChar 65 /LastChar 65 /Widths [0]", "/Encoding /MacExpertEncoding"), "BT /F0 12 Tf 10 10 Td (A) Tj 2 Tr (A) Tj ET", emptyPostNameNoCmap())},
		// CIDFontType2 over Identity-H: a code is its CID, and the CIDToGIDMap takes it to a glyph.
		{name: "CID: /DW agreeing", pdf: cidFontDoc("/DW 500 /CIDToGIDMap /Identity", cid, std, nil)},
		{name: "CID: /DW absent, so 1000", pdf: cidFontDoc("/CIDToGIDMap /Identity", cid, std, nil)},
		{name: "CID: code 0", pdf: cidFontDoc("/DW 500 /CIDToGIDMap /Identity", "BT /F0 12 Tf 10 10 Td <0000> Tj ET", std, nil)},
		{name: "CID: a CID past the glyph count", pdf: cidFontDoc("/DW 500 /CIDToGIDMap /Identity", "BT /F0 12 Tf 10 10 Td <00C8> Tj ET", std, nil)},
		{name: "CID: /W single width off", pdf: cidFontDoc("/DW 500 /W [33 [600]] /CIDToGIDMap /Identity", cid, std, nil)},
		{name: "CID: /W range agreeing over a /DW that is off", pdf: cidFontDoc("/DW 900 /W [30 40 500] /CIDToGIDMap /Identity", cid, std, nil)},
		// pdfcpu drops this font, so nib sees text in a font that does not resolve — and refuses rather than answer "no glyph".
		{name: "CID: /W range whose width is not a number", pdf: cidFontDoc("/DW 900 /W [30 40 /x] /CIDToGIDMap /Identity", cid, std, nil),
			refused: []string{"does not resolve", "does not resolve", "does not resolve"}},
		{name: "CID: /W single before a range", pdf: cidFontDoc("/DW 900 /W [30 40 900 33 [500]] /CIDToGIDMap /Identity", cid, std, nil)},
		{name: "CID: a CIDToGIDMap stream mapping CID 1", pdf: cidFontDoc("/DW 500 /CIDToGIDMap 30 0 R", "BT /F0 12 Tf 10 10 Td <0001> Tj ET", std, map[int]string{30: mapStream})},
		{name: "CID: a CIDToGIDMap stream shorter than the CID", pdf: cidFontDoc("/DW 500 /CIDToGIDMap 30 0 R", cid, std, map[int]string{30: mapStream})},
		{name: "CID: no CIDToGIDMap", pdf: cidFontDoc("/DW 500", cid, std, nil)},
		{name: "CID: a CIDToGIDMap stream shorter than the CID, over glyph 0's advance", pdf: cidFontDoc("/DW 300 /CIDToGIDMap 30 0 R", cid, zeroWideProgram(), map[int]string{30: mapStream})},
		{name: "CID: a program veraPDF cannot parse", pdf: cidFontDoc("/DW 900 /CIDToGIDMap /Identity", "BT /F0 12 Tf 10 10 Td <00C8> Tj ET", broken, nil)},
		{name: "CID: an absent glyph drawn invisibly", pdf: cidFontDoc("/DW 500 /CIDToGIDMap /Identity", "BT /F0 12 Tf 10 10 Td 3 Tr <00C8> Tj ET", std, nil)},
		{name: "CID: no program", pdf: cidFontDoc("/DW 900", "BT /F0 12 Tf 10 10 Td <0000> Tj ET", nil, nil)},
	}
}

// TestThePerGlyphClausesAgreeWithVeraPDF — every shape above against veraPDF's verdicts, measured before the rules
// were written; the live oracle asks veraPDF again on every run.
func TestThePerGlyphClausesAgreeWithVeraPDF(t *testing.T) {
	want := map[byte]Verdict{'P': Pass, 'F': Fail, '-': NotApplicable}
	fx := metricFixtures()
	if len(fx) != len(metricVeraPDF) {
		t.Fatalf("%d fixtures and %d measured rows", len(fx), len(metricVeraPDF))
	}
	for i, f := range fx {
		for j, clause := range metricClauses {
			got := verdictOf(t, f.pdf, clause)
			if f.refused != nil && f.refused[j] != "." {
				if got.Verdict != CannotCheck || !strings.Contains(got.Why, f.refused[j]) {
					t.Errorf("%s: %s reports %v (%s), want CannotCheck naming %q", f.name, clause, got.Verdict, got.Why, f.refused[j])
				}
				continue
			}
			if w := want[metricVeraPDF[i][j]]; got.Verdict != w {
				t.Errorf("%s: %s reports %v (%s), veraPDF %c", f.name, clause, got.Verdict, got.Why, metricVeraPDF[i][j])
			}
		}
	}
}

// metricVeraPDF is veraPDF 1.30.2's answer for each of metricFixtures, in order, over metricClauses.
var metricVeraPDF = []string{
	"PPP",
	"FPP",
	"PPP",
	"FPP",
	"FPP",
	"FPP",
	"FPP",
	"PPP",
	"FPP",
	"PPP",
	"FFP",
	"FFP",
	"PPF",
	"PPF",
	"PPP",
	"PFP",
	"PFP",
	"PPP",
	"PPF",
	"PPP",
	"PPP",
	"PPP",
	"PPF",
	"PPP",
	"PPF",
	"FFF",
	"PPF",
	"PPF",
	"PFF",
	"PPP",
	"PFP",
	"PPP",
	"PPP",
	"PFP",
	"PPP",
	"PPF",
	"PPF",
	"PFP",
	"PFP",
	"PFP",
	"PPP",
	"PPP",
	"PFP",
	"PPP",
	"PFF",
	"PPP",
	"FPP",
	"PPF",
	"PFF",
	"FPP",
	"PPP",
	"FPP",
	"PPP",
	"PPP",
	"PFF",
	"PPP",
	"PFF",
	"PPP",
	"PPF",
	"PPP",
}

// TestTheShippedFontRulesReadTheNewDoors — two shipped clauses the per-glyph fixtures reach, asserted where veraPDF is
// absent too: 7.21.4.1 t1 fails a CIDFontType2 program veraPDF cannot parse (/pending 677's CID half, measured), and
// 7.21.4.2 t2 refuses a CIDFont pdfcpu dropped, which veraPDF judges (it had answered NotApplicable).
func TestTheShippedFontRulesReadTheNewDoors(t *testing.T) {
	byName := map[string][]byte{}
	for _, f := range metricFixtures() {
		byName[f.name] = f.pdf
	}
	if got := verdictOf(t, byName["CID: a program veraPDF cannot parse"], "7.21.4.1 t1"); got.Verdict != Fail || !strings.Contains(got.Why, "cannot read its embedded TrueType program") {
		t.Errorf("7.21.4.1 t1 over an unparsable CIDFontType2 program: %v (%s), want Fail naming the program", got.Verdict, got.Why)
	}
	if got := verdictOf(t, byName["CID: /W range whose width is not a number"], "7.21.4.2 t2"); got.Verdict != CannotCheck || !strings.Contains(got.Why, "does not resolve") {
		t.Errorf("7.21.4.2 t2 over a CIDFont pdfcpu dropped: %v (%s), want CannotCheck naming the unresolved font", got.Verdict, got.Why)
	}
}

// TestACompositeFontIsReadOnce — the CIDToGIDMap and /W are read once per Type 0 font, not per glyph (the P07.S04a
// review measured a 128 KB map re-decoded at 0.9 ms per glyph).
func TestACompositeFontIsReadOnce(t *testing.T) {
	d, err := open(cidFontDoc("/DW 500 /CIDToGIDMap /Identity", "BT /F0 12 Tf 10 10 Td <0021> Tj ET", ttProgram(sub31, sub10), nil))
	if err != nil {
		t.Fatal(err)
	}
	fonts, why := d.usedFonts()
	if why != "" || len(fonts) == 0 {
		t.Fatalf("no used font (%s)", why)
	}
	font := fonts[0].dict
	a, _, _ := d.cidTrueTypeOf(font)
	b, _, _ := d.cidTrueTypeOf(font)
	if a == nil || a != b {
		t.Fatalf("two readings of one Type 0 font are %p and %p, want one", a, b)
	}
}

// TestAMapEntryIsChargedAtItsCost — every map entry, not only a format 4 run, is charged `mapEntryCost`: 2,100 records
// of one format 0 subtable are 540,000 entries, over the budget at 8 reads each and under it at 1.
func TestAMapEntryIsChargedAtItsCost(t *testing.T) {
	const records = 2100
	c := beBytes(uint16(0), uint16(records))
	for i := 0; i < records; i++ {
		c = append(c, beBytes(uint16(1), uint16(0), uint32(4+8*records))...)
	}
	c = append(c, fmt0With(0x41, 1)...)
	p := readTrueType(sfnt(ttTable{"cmap", c}, ttTable{"head", ttHead()}, ttTable{"hhea", ttHhea(ttGlyphs)}, ttTable{"hmtx", ttHmtx(ttGlyphs)}), maxTrueTypeReads)
	if p.state != ttUnknown {
		t.Fatalf("%d format 0 records: state %v after %d reads, want unknown past the budget at %d reads an entry", records, p.state, p.reads, mapEntryCost)
	}
}

// TestAMetricFailurePointsAtTheVisibleUse — a glyph drawn first invisibly and then visibly fails where it is VISIBLE,
// the use the rule judges.
func TestAMetricFailurePointsAtTheVisibleUse(t *testing.T) {
	pdf := glyphPage("BT /F0 12 Tf 10 10 Td 3 Tr <E9> Tj 0 Tr <E9> Tj ET", "", "",
		mfDict("/FirstChar 65 /LastChar 233 /Widths [500]", "/Encoding /WinAnsiEncoding"), ttObjects("/Flags 32", "FontFile2", ttProgram(sub31, sub10), ""))
	got := verdictOf(t, pdf, "7.21.4.1 t2")
	if got.Verdict != Fail || !strings.Contains(got.Where, "#8") {
		t.Errorf("7.21.4.1 t2 reports %v at %q, want Fail at the visible Tj (operator #8; the invisible one is #6)", got.Verdict, got.Where)
	}
}
