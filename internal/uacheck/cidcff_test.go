package uacheck

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// P07.S05b's fixture generator: a CID-keyed CFF program, laid out header → Name → Top DICT → String → Global Subr INDEX
// → charset → FDSelect → CharStrings → FDArray → each font dict's Private DICT and local Subrs, every offset a five-byte
// integer so the layout is computed in one pass. ROS is the Top DICT's first operator, its SIDs two bytes each, as
// `CFFFontProgram.isCIDFont`'s shortcut expects.

// cidFD is one font dict of the FDArray.
type cidFD struct {
	private   []byte   // Private DICT operators (Subrs is appended when lsubrs is set)
	lsubrs    [][]byte // local subroutines; nil: no Subrs operator
	extra     []byte   // further font dict operators (FontMatrix, …), before Private
	noPrivate bool     // the font dict carries no Private operator
	privSize  int      // the Private operator's size when not 0, whatever the DICT's length
}

// cidSpec is one CID-keyed program. GID 0 is CID 0; `cids` names GID 1's CID onward.
type cidSpec struct {
	cids        []int
	charstrings [][]byte // GID 0..; nil: every glyph `0 endchar` (width nominalWidthX)
	charset     []byte   // raw charset bytes; nil: format 0 over cids
	noCharset   bool     // the Top DICT carries no charset operator
	fdselect    []byte   // raw FDSelect bytes; nil: format 0, every glyph FD 0
	fds         []cidFD  // nil: one font dict with an empty Private DICT
	gsubrs      [][]byte
	topExtra    []byte // Top DICT operators after the generated ones
}

func (s cidSpec) build() []byte {
	css := s.charstrings
	if css == nil {
		css = make([][]byte, len(s.cids)+1)
		for i := range css {
			css[i] = cs(0, "endchar")
		}
	}
	charset := s.charset
	if charset == nil {
		charset = []byte{0}
		for _, c := range s.cids {
			charset = append(charset, byte(c>>8), byte(c))
		}
	}
	fdselect := s.fdselect
	if fdselect == nil {
		fdselect = append([]byte{0}, make([]byte, len(css))...)
	}
	fds := s.fds
	if fds == nil {
		fds = []cidFD{{}}
	}
	header := []byte{1, 0, 4, 1}
	nameIdx := cffIndexBytes([][]byte{[]byte("Probe")})
	strIdx := cffIndexBytes([][]byte{[]byte("Adobe"), []byte("Identity")})
	gsubIdx := cffIndexBytes(s.gsubrs)
	csIdx := cffIndexBytes(css)
	// 391 and 392 (the String INDEX's two) as two-byte operands, supplement 0 as one.
	ros := []byte{248, 27, 248, 28, 139, 12, 30}
	top := func(charsetOff, fdselOff, csOff, fdarrOff int) []byte {
		t := append([]byte{}, ros...)
		if !s.noCharset {
			t = append(append(t, dictInt(charsetOff)...), 15)
		}
		t = append(append(t, dictInt(csOff)...), 17)
		t = append(append(t, dictInt(fdarrOff)...), 12, 36)
		t = append(append(t, dictInt(fdselOff)...), 12, 37)
		return append(t, s.topExtra...)
	}
	topLen := len(cffIndexBytes([][]byte{top(0, 0, 0, 0)}))
	charsetOff := len(header) + len(nameIdx) + topLen + len(strIdx) + len(gsubIdx)
	fdselOff := charsetOff + len(charset)
	csOff := fdselOff + len(fdselect)
	fdarrOff := csOff + len(csIdx)
	// Each font dict's Private DICT (and its Subrs, relative to it) follows the FDArray.
	privs := make([][]byte, len(fds))
	for i, fd := range fds {
		p := append([]byte{}, fd.private...)
		if fd.lsubrs != nil {
			p = append(append(p, dictInt(0)...), 19)
			copy(p[len(p)-6:], dictInt(len(p)))
		}
		privs[i] = p
	}
	fdDict := func(i, privOff int) []byte {
		d := append([]byte{}, fds[i].extra...)
		if !fds[i].noPrivate {
			size := len(privs[i])
			if fds[i].privSize != 0 {
				size = fds[i].privSize
			}
			d = append(append(append(d, dictInt(size)...), dictInt(privOff)...), 18)
		}
		return d
	}
	dicts := make([][]byte, len(fds))
	for i := range fds {
		dicts[i] = fdDict(i, 0)
	}
	pos := fdarrOff + len(cffIndexBytes(dicts))
	for i, fd := range fds {
		dicts[i] = fdDict(i, pos)
		pos += len(privs[i])
		if fd.lsubrs != nil {
			pos += len(cffIndexBytes(fd.lsubrs))
		}
	}
	out := append([]byte{}, header...)
	out = append(out, nameIdx...)
	out = append(out, cffIndexBytes([][]byte{top(charsetOff, fdselOff, csOff, fdarrOff)})...)
	out = append(out, strIdx...)
	out = append(out, gsubIdx...)
	out = append(out, charset...)
	out = append(out, fdselect...)
	out = append(out, csIdx...)
	out = append(out, cffIndexBytes(dicts)...)
	for i, fd := range fds {
		out = append(out, privs[i]...)
		if fd.lsubrs != nil {
			out = append(out, cffIndexBytes(fd.lsubrs)...)
		}
	}
	return out
}

// cid0Doc is a tagged page drawing `show` (hex codes) in a Type 0 font over Identity-H (or the CMap object `cmap` when
// set, as object 21) whose descendant is a `cidType` CIDFont named `base`, carrying `cid` entries (/W, /DW, …), whose
// descriptor carries `desc` entries (/CIDSet 22 0 R, …) and the program `prog` as /FontFile3 of `progSubtype` (none when
// nil). `extra` adds objects.
func cid0Doc(cidType, base, cid, desc, show string, prog []byte, progSubtype, cmap string, extra map[int]string) []byte {
	enc := "/Identity-H"
	objs := map[int]string{
		11: "<< /Type /Font /Subtype /" + cidType + " /BaseFont /" + base + " /CIDSystemInfo << /Registry (Adobe) " +
			"/Ordering (Identity) /Supplement 0 >> /FontDescriptor 12 0 R " + cid + " >>",
		12: "<< /Type /FontDescriptor /FontName /" + base + " /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 " +
			"/Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 " + desc,
	}
	if cmap != "" {
		enc = "21 0 R"
		objs[21] = cmap
	}
	if prog != nil {
		objs[12] += " /FontFile3 20 0 R"
		objs[20] = spStream("/Subtype /"+progSubtype, string(prog))
	}
	objs[12] += " >>"
	for k, v := range extra {
		objs[k] = v
	}
	return glyphDoc("<< /Type /Font /Subtype /Type0 /BaseFont /"+base+" /Encoding "+enc+" /DescendantFonts [11 0 R] >>", show, objs)
}

// cidSetBytes is a /CIDSet stream marking `cids`.
func cidSetBytes(cids ...int) string {
	n := 0
	for _, c := range cids {
		n = max(n, c/8+1)
	}
	b := make([]byte, n)
	for _, c := range cids {
		b[c/8] |= 0x80 >> (c % 8)
	}
	return spStream("", string(b))
}

// hexCodes is a `<…> Tj` of two-byte codes.
func hexCodes(codes ...int) string {
	var sb strings.Builder
	sb.WriteByte('<')
	for _, c := range codes {
		fmt.Fprintf(&sb, "%04X", c)
	}
	sb.WriteString("> Tj")
	return sb.String()
}

// cidCFFClauses are the clauses P07.S05b's fixtures are measured over, in the order of their `vera` strings.
var cidCFFClauses = []string{"7.21.5 t1", "7.21.4.1 t2", "7.21.8 t1", "7.21.4.2 t2", "7.21.4.1 t1", "7.21.4.2 t1"}

// fdMatrix is a FontMatrix operator of six reals.
func fdMatrix(vals ...string) []byte {
	var b []byte
	for _, v := range vals {
		b = append(b, dictRealBytes(v)...)
	}
	return append(b, 12, 7)
}

func cidCFFFixtures() []measuredFixture {
	sub, full := "ABCDEF+Probe", "Probe"
	w := func(ws ...int) [][]byte {
		out := [][]byte{cs(0, "endchar")}
		for _, x := range ws {
			out = append(out, cs(x, "endchar"))
		}
		return out
	}
	std := cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500)}.build()
	w500 := "/W [1 [500 500 500]]"
	show := hexCodes(1, 2, 3)
	doc := func(base, cid, desc, show string, prog []byte) []byte {
		return cid0Doc("CIDFontType0", base, cid, desc, show, prog, "CIDFontType0C", "", nil)
	}
	nom := func(v int) []byte { return append(dictInt(v), 21) }
	twoFD := []cidFD{{private: nom(100)}, {private: nom(200)}}
	ident := cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), charset: []byte{3}}.build()
	cid0map := cidSpec{cids: []int{5, 0, 3}, charstrings: w(500, 500, 500)}.build()
	t1 := cffSpec{names: []string{"A", "B", "C"}, charstrings: [][]byte{cs(300, "endchar"), cs(500, "endchar"), cs(500, "endchar"), cs(500, "endchar")}}.build()
	return []measuredFixture{
		{name: "CID: widths agree", vera: "PPPPP-", pdf: doc(sub, w500, "", show, std)},
		{name: "CID: a width 100 off", vera: "FPPPP-", pdf: doc(sub, "/W [1 [600 500 500]]", "", show, std)},
		{name: "CID: a width 1 off", vera: "PPPPP-", pdf: doc(sub, "/W [1 [501 500 500]]", "", show, std)},
		{name: "CID: a CID the charset lacks, /W 900", vera: "FFFPP-", pdf: doc(sub, "/W [7 [900]]", "", hexCodes(7), std)},
		{name: "CID: a CID the charset lacks, /DW", vera: "PFFPP-", pdf: doc(sub, "", "", hexCodes(7), std)},
		{name: "CID: code 0", vera: "PPFPP-", pdf: doc(sub, "/W [0 [0 500 500 500]]", "", hexCodes(0), std)},
		{name: "CID: code 0 against /DW", vera: "FPFPP-", pdf: doc(sub, "", "", hexCodes(0), std)},
		{name: "CID: an unknown charset format, a CID in range", vera: "PFFPP-", pdf: doc(sub, w500, "", hexCodes(2), ident)},
		{name: "CID: an unknown charset format, a CID past the glyphs, subset", vera: "FFFPP-", pdf: doc(sub, "/W [5 [700]]", "", hexCodes(5), ident)},
		{name: "CID: an unknown charset format, a CID past the glyphs, full", vera: "FFFPP-", pdf: doc(full, "/W [5 [700]]", "", hexCodes(5), ident)},
		{name: "CID: two font dicts' nominal widths", vera: "PPPPP-", pdf: doc(sub, "/W [1 [500 600 600]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(400, 400, 400), fdselect: []byte{0, 0, 0, 1, 1}, fds: twoFD}.build())},
		{name: "CID: two font dicts, the second's width read with the first's", vera: "FPPPP-", pdf: doc(sub, "/W [1 [500 500 500]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(400, 400, 400), fdselect: []byte{0, 0, 0, 1, 1}, fds: twoFD}.build())},
		{name: "CID: FDSelect format 3", vera: "PPPPP-", pdf: doc(sub, "/W [1 [500 600 600]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(400, 400, 400), fdselect: []byte{3, 0, 2, 0, 0, 0, 0, 2, 1, 0, 4}, fds: twoFD}.build())},
		{name: "CID: FDSelect format 2", vera: "PPPPF-", pdf: doc(sub, "/W [1 [900 900 900]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), fdselect: []byte{2, 0, 0, 0, 0}}.build())},
		{name: "CID: an FDSelect entry past the FDArray, /W 500", vera: "FPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), fdselect: []byte{0, 0, 5, 0, 0}}.build())},
		{name: "CID: an FDSelect entry past the FDArray, /W 1000", vera: "PPPPP-", pdf: doc(sub, "/W [1 [1000 500 500]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), fdselect: []byte{0, 0, 5, 0, 0}}.build())},
		{name: "CID: a font dict's FontMatrix 0.002", vera: "PPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(250, 250, 250), fds: []cidFD{{extra: fdMatrix("0.002", "0", "0", "0.002", "0", "0")}}}.build())},
		{name: "CID: Top FontMatrix 1, font dict 0.002", vera: "PPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(250, 250, 250), topExtra: fdMatrix("1", "0", "0", "1", "0", "0"), fds: []cidFD{{extra: fdMatrix("0.002", "0", "0", "0.002", "0", "0")}}}.build())},
		{name: "CID: Top FontMatrix 0.002, no font dict matrix", vera: "PPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(250, 250, 250), topExtra: fdMatrix("0.002", "0", "0", "0.002", "0", "0")}.build())},
		{name: "CID: Top FontMatrix 0.002, font dict 0.001", vera: "FPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(250, 250, 250), topExtra: fdMatrix("0.002", "0", "0", "0.002", "0", "0"), fds: []cidFD{{extra: fdMatrix("0.001", "0", "0", "0.001", "0", "0")}}}.build())},
		{name: "CID: Top FontMatrix 1, no font dict matrix", vera: "FPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), topExtra: fdMatrix("1", "0", "0", "1", "0", "0")}.build())},
		{name: "CID: a font dict with no Subrs uses the one before's", vera: "PPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs(0, "endchar"), cs(-107, "callsubr"), cs(-107, "callsubr"), cs(-107, "callsubr")}, fdselect: []byte{0, 0, 0, 1, 1}, fds: []cidFD{{lsubrs: [][]byte{cs(500, "endchar")}}, {}}}.build())},
		{name: "CID: a subroutine call with no Subrs, subset", vera: "FPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs(0, "endchar"), cs(-107, "callsubr"), cs(500, "endchar"), cs(500, "endchar")}}.build())},
		{name: "CID: a subroutine call with no Subrs, full", vera: "FPPPP-", pdf: doc(full, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs(0, "endchar"), cs(-107, "callsubr"), cs(500, "endchar"), cs(500, "endchar")}}.build())},
		{name: "CID: a subroutine call with no Subrs, full, not drawn", vera: "PPPPP-", pdf: doc(full, w500, "", hexCodes(2), cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs(0, "endchar"), cs(-107, "callsubr"), cs(500, "endchar"), cs(500, "endchar")}}.build())},
		{name: "CID: a format 1 range past the glyphs", vera: "PFFPP-", pdf: doc(sub, "", "", hexCodes(4), cidSpec{cids: []int{1, 2, 3}, charstrings: w(1000, 1000, 1000), charset: []byte{1, 0, 1, 10}}.build())},
		{name: "CID: a duplicate CID, the later wins", vera: "PPPPP-", pdf: doc(sub, "/W [1 [500]]", "", hexCodes(1), cidSpec{cids: []int{1, 1, 3}, charstrings: w(300, 500, 500)}.build())},
		{name: "CID: no charset operator reads the header", vera: "PPPPP-", pdf: doc(sub, "/W [4 [500]]", "", hexCodes(4), cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 300, 300), noCharset: true}.build())},
		{name: "CID: a font dict with no Private uses the one before's", vera: "PPPPP-", pdf: doc(sub, "/W [1 [500 500 500]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(400, 400, 400), fdselect: []byte{0, 0, 0, 1, 1}, fds: []cidFD{{private: nom(100)}, {noPrivate: true}}}.build())},
		{name: "CID: an empty FDArray", vera: "PPPPP-", pdf: doc(sub, "/W [1 [1000 1000 1000]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), fds: []cidFD{}}.build())},
		{name: "CID: a truncated program", vera: "PPPPF-", pdf: doc(sub, "/W [1 [900 900 900]]", "", show, std[:40])},
		{name: "CID: charstring type 1", vera: "FPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), topExtra: []byte{140, 12, 6}}.build())},
		{name: "CID: under a CIDFontType2", vera: "PPPPP-", pdf: cid0Doc("CIDFontType2", sub, w500, "", show, std, "CIDFontType0C", "", nil)},
		{name: "CID: an embedded CMap to CID 2", vera: "PPPPP-", pdf: cid0Doc("CIDFontType0", sub, "/W [2 [500]]", "", "<0041> Tj", std, "CIDFontType0C", cidCMap("", "1 begincidchar <0041> 2 endcidchar"), nil)},
		{name: "CID: CID 0 remapped, a code the CMap does not hold", vera: "PFFPP-", pdf: cid0Doc("CIDFontType0", sub, "/W [0 [500] 5 [500]]", "", "<0042> Tj", cid0map, "CIDFontType0C", cidCMap("", "1 begincidchar <0041> 5 endcidchar"), nil)},
		{name: "CID: CID 0 remapped, a code the CMap maps to CID 0", vera: "PPPPP-", pdf: cid0Doc("CIDFontType0", sub, "/W [0 [500] 5 [500]]", "", "<0043> Tj", cid0map, "CIDFontType0C", cidCMap("", "2 begincidchar <0041> 5 <0043> 0 endcidchar"), nil)},
		{name: "CID: CID 0 remapped, code 0 unheld", vera: "PPFPP-", pdf: cid0Doc("CIDFontType0", sub, "/W [0 [500] 5 [500]]", "", "<0000> Tj", cid0map, "CIDFontType0C", cidCMap("", "1 begincidchar <0041> 5 endcidchar"), nil)},
		{name: "CID: a format 2 charset", vera: "PPPPP-", pdf: doc(sub, "/W [1 [500 500 500]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), charset: []byte{2, 0, 1, 0, 2}}.build())},
		{name: "CID: a format 2 charset, a two-byte count past the glyphs", vera: "PFFPP-", pdf: doc(sub, "", "", hexCodes(4), cidSpec{cids: []int{1, 2, 3}, charstrings: w(1000, 1000, 1000), charset: []byte{2, 0, 1, 1, 0}}.build())},
		{name: "CID: a skewed Top and font dict matrix, top times dict", vera: "PPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(250, 250, 250), topExtra: fdMatrix("0.001", "0.001", "0", "0.001", "0", "0"), fds: []cidFD{{extra: fdMatrix("1", "0", "1", "1", "0", "0")}}}.build())},
		{name: "CID: Top FontMatrix 2, font dict 0.001", vera: "PPPPP-", pdf: doc(sub, "/W [1 [1000 1000 1000]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), topExtra: fdMatrix("2", "0", "0", "2", "0", "0"), fds: []cidFD{{extra: fdMatrix("0.001", "0", "0", "0.001", "0", "0")}}}.build())},
		{name: "CID: a later font dict's nominal width resets to 0", vera: "PPPPP-", pdf: doc(sub, "/W [1 [500 400 400]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(400, 400, 400), fdselect: []byte{0, 0, 0, 1, 1}, fds: []cidFD{{private: nom(100)}, {private: append(dictInt(0), 20)}}}.build())},
		{name: "CID: a font dict's defaultWidthX", vera: "PPPPP-", pdf: doc(sub, "/W [1 [500 300 300]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs("endchar"), cs(500, "endchar"), cs("endchar"), cs("endchar")}, fdselect: []byte{0, 0, 0, 1, 1}, fds: []cidFD{{private: append(dictInt(700), 20)}, {private: append(dictInt(300), 20)}}}.build())},
		{name: "CID: a later font dict's defaultWidthX resets to 0", vera: "PPPPP-", pdf: doc(sub, "/W [1 [700 0 0]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs("endchar"), cs("endchar"), cs("endchar"), cs("endchar")}, fdselect: []byte{0, 0, 0, 1, 1}, fds: []cidFD{{private: append(dictInt(700), 20)}, {private: nom(0)}}}.build())},
		{name: "CID: a CID the charset lacks, /DW 700", vera: "PFFPP-", pdf: doc(sub, "/DW 700", "", hexCodes(7), std)},
		{name: "CID: a non-empty Private without Subrs uses the one before's", vera: "PPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs(0, "endchar"), cs(-107, "callsubr"), cs(-107, "callsubr"), cs(-107, "callsubr")}, fdselect: []byte{0, 0, 0, 1, 1}, fds: []cidFD{{lsubrs: [][]byte{cs(500, "endchar")}}, {private: nom(0)}}}.build())},
		{name: "CID: FDSelect format 3, a range past the glyphs", vera: "PPPPP-", pdf: doc(sub, "/W [1 [500 600 600]]", "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(400, 400, 400), fdselect: []byte{3, 0, 2, 0, 0, 0, 0, 2, 1, 0, 40}, fds: twoFD}.build())},
		{name: "CID: truncated in the FDArray", vera: "PPPPF-", pdf: doc(sub, "/W [1 [900 900 900]]", "", show, std[:len(std)-2])},
		{name: "CID: truncated in the CharStrings", vera: "PPPPF-", pdf: doc(sub, "/W [1 [900 900 900]]", "", show, std[:len(std)-12])},
		{name: "CID: truncated in the charset", vera: "PPPPF-", pdf: doc(sub, "/W [1 [900 900 900]]", "", show, std[:len(std)-24])},
		{name: "CID: truncated in a Private DICT", vera: "PPPPF-", pdf: doc(sub, "/W [1 [900 900 900]]", "", show, func() []byte {
			b := cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), fds: []cidFD{{private: append(nom(0), nom(0)...)}}}.build()
			return b[:len(b)-3]
		}())},
		{name: "CID: subset, a charstring INDEX running backwards", vera: "XXXXXX", pdf: doc(sub, w500, "", hexCodes(1), cidBackwards())},
		{name: "CID: full, a drawn charstring running backwards", vera: "XXXXXX", pdf: doc(full, w500, "", hexCodes(2), cidBackwards())},
		{name: "CID: full, an undrawn charstring running backwards", vera: "PPPPP-", pdf: doc(full, w500, "", hexCodes(1), cidBackwards())},
		{name: "CID: a bare operator 16 in the Top DICT", vera: "PPPPP-", pdf: doc(sub, w500, "", show, cidSpec{cids: []int{1, 2, 3}, charstrings: w(500, 500, 500), topExtra: []byte{16}}.build())},
		{name: "CID: /CIDSet past 16 KB, a bit beyond it", vera: "PPPPP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", show, std, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0, 1, 2, 3, 16384*8+5)})},
		{name: "CID: a format 1 range ending exactly at the glyph count", vera: "PFFPP-", pdf: doc(sub, "", "", hexCodes(4), cidSpec{cids: []int{1, 2, 3}, charstrings: w(1000, 1000, 1000), charset: []byte{1, 0, 1, 3}}.build())},
		{name: "CID: /CIDSet exact", vera: "PPPPP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", show, std, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0, 1, 2, 3)})},
		{name: "CID: /CIDSet missing a CID", vera: "PPPFP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", show, std, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0, 1, 2)})},
		{name: "CID: /CIDSet with a CID too many", vera: "PPPFP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", show, std, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0, 1, 2, 3, 9)})},
		{name: "CID: /CIDSet without CID 0", vera: "PPPPP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", show, std, "CIDFontType0C", "", map[int]string{22: cidSetBytes(1, 2, 3)})},
		{name: "CID: /CIDSet short, not subset-named", vera: "PPPPP-", pdf: cid0Doc("CIDFontType0", full, w500, "/CIDSet 22 0 R", show, std, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0, 1)})},
		{name: "CID: /CIDSet short, program not parsed", vera: "PPPPF-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", show, std[:40], "CIDFontType0C", "", map[int]string{22: cidSetBytes(0, 1)})},
		{name: "CID: /CIDSet over an unknown charset format", vera: "PFFFP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", hexCodes(2), ident, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0, 1, 2, 3)})},
		{name: "CID: /CIDSet empty over an unknown charset format", vera: "PFFPP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", hexCodes(2), ident, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0)})},
		{name: "CID-keyed under a simple Type 1 font", vera: "XXXXXX", pdf: t1cDoc(sub, "/FirstChar 1 /LastChar 3 /Widths [500 500 500]", "", "<010203> Tj", std, "Type1C")},
		{name: "CID-keyed under a simple Type 1 font, a CharSet", vera: "XXXXXX", pdf: t1cDoc(sub, "/FirstChar 1 /LastChar 3 /Widths [500 500 500]", "/CharSet (/A)", "<010203> Tj", std, "Type1C")},
		{name: "CID-keyed under a simple Type 1 font, nothing drawn", vera: "----PP", pdf: t1cDoc(sub, "/FirstChar 1 /LastChar 3 /Widths [500 500 500]", "", "() Tj", std, "Type1C")},
		{name: "CID-keyed under a simple Type 1 font, nothing drawn, a CharSet", vera: "XXXXXX", pdf: t1cDoc(sub, "/FirstChar 1 /LastChar 3 /Widths [500 500 500]", "/CharSet (/A)", "() Tj", std, "Type1C")},
		{name: "CID-keyed under a simple Type 1 font, nothing drawn, not subset-named, a CharSet", vera: "----PP", pdf: t1cDoc(full, "/FirstChar 1 /LastChar 3 /Widths [500 500 500]", "/CharSet (/A)", "() Tj", std, "Type1C")},
		{name: "CID-keyed under a simple Type 1 font, WinAnsi-named codes", vera: "FFP-PP", pdf: t1cDoc(full, "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding", "", "(ABC) Tj", std, "Type1C")},
		{name: "CID-keyed under a simple Type 1 font, WinAnsi-named, widths 0", vera: "PFP-PP", pdf: t1cDoc(sub, "/FirstChar 65 /LastChar 67 /Widths [0 0 0] /Encoding /WinAnsiEncoding", "", "(ABC) Tj", std, "Type1C")},
		{name: "CID-keyed under a simple Type 1 font, WinAnsi-named and an unnamed code", vera: "PFF-PP", pdf: t1cDoc(full, "/FirstChar 1 /LastChar 67 /Encoding /WinAnsiEncoding", "", "<0141> Tj", std, "Type1C")},
		{name: "T1C in CID: codes to GIDs", vera: "PPPPP-", pdf: doc(sub, w500, "", show, t1)},
		{name: "T1C in CID: code 0", vera: "PPPPP-", pdf: doc(sub, "/W [0 [300]]", "", hexCodes(0), t1)},
		{name: "T1C in CID: the CID equal to the glyph count", vera: "FFFPP-", pdf: doc(sub, "/W [4 [900]]", "", hexCodes(4), t1)},
		{name: "T1C in CID: a CID past the glyphs", vera: "FFFPP-", pdf: doc(sub, "/W [5 [900]]", "", hexCodes(5), t1)},
		{name: "T1C in CID: /CIDSet", vera: "PPPFP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", show, t1, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0, 1, 2, 3)})},
		{name: "T1C in CID: /CIDSet of CID 0 only", vera: "PPPPP-", pdf: cid0Doc("CIDFontType0", sub, w500, "/CIDSet 22 0 R", show, t1, "CIDFontType0C", "", map[int]string{22: cidSetBytes(0)})},
	}
}

// cidCFFRefusals are the (fixture, clause) pairs nib deliberately refuses, as knownCannotCheck records them: Type 1
// charstrings inside a CFF go through veraPDF's `Type1CharStringParser`, which nib does not mirror, and the width's
// refusal takes the glyph's presence with it (one metrics door, as P07.S05a declared).
var cidCFFRefusals = map[string]bool{
	"CID: charstring type 1 / 7.21.5 t1":   true,
	"CID: charstring type 1 / 7.21.4.1 t2": true,
}

// TestTheCIDKeyedCFFClausesAgreeWithVeraPDF — every CIDFontType0C shape against veraPDF 1.30.2's verdicts, measured before
// the reader was written (P07.S05b); the live oracle asks veraPDF again on every run. `X` is a document veraPDF reports
// nothing on, which nib must refuse on every clause through the one door.
func TestTheCIDKeyedCFFClausesAgreeWithVeraPDF(t *testing.T) {
	want := map[byte]Verdict{'P': Pass, 'F': Fail, '-': NotApplicable}
	for _, f := range cidCFFFixtures() {
		if len(f.vera) != len(cidCFFClauses) {
			t.Fatalf("%s: %d verdict letters for %d clauses", f.name, len(f.vera), len(cidCFFClauses))
		}
		for j, clause := range cidCFFClauses {
			got := verdictOf(t, f.pdf, clause)
			if f.vera[j] == 'X' {
				if got.Verdict != CannotCheck || !strings.Contains(got.Why, "veraPDF reports nothing") {
					t.Errorf("%s: %s reports %v (%s), and veraPDF reports nothing on the document", f.name, clause, got.Verdict, got.Why)
				}
				continue
			}
			if cidCFFRefusals[f.name+" / "+clause] && got.Verdict == CannotCheck && strings.Contains(got.Why, "type 1") {
				continue
			}
			w, ok := want[f.vera[j]]
			if !ok {
				t.Fatalf("%s: verdict letter %q is not one of P, F, -, X", f.name, f.vera[j])
			}
			if got.Verdict != w {
				t.Errorf("%s: %s reports %v (%s), veraPDF %c", f.name, clause, got.Verdict, got.Why, f.vera[j])
			}
		}
	}
}

// verdictIn is one clause's verdict in a report; a clause the report does not hold is fatal, as in `verdictOf` — a
// default verdict would let a clause that stopped running pass any assertion shaped like its default.
func verdictIn(t *testing.T, r Report, clause string) Verdict {
	t.Helper()
	for _, res := range r.Results {
		if res.Clause == clause {
			return res.Verdict
		}
	}
	t.Fatalf("clause %q is not in the report", clause)
	return CannotCheck
}

// TestTheCIDReaderBoundsWhatAHostileProgramCosts — the three places a CID-keyed program's own counts multiply nib's
// work, none reachable by a veraPDF-measured fixture (veraPDF performs the work, or runs out of memory): a Subrs index
// carried through many font dicts is read ONCE, a Private DICT carried through many font dicts is charged to one DICT
// budget, and FDSelect ranges that refill the array are charged to theirs.
func TestTheCIDReaderBoundsWhatAHostileProgramCosts(t *testing.T) {
	lsubrs := make([][]byte, 2000)
	for i := range lsubrs {
		lsubrs[i] = cs(500, "endchar")
	}
	fds := make([]cidFD, 300)
	fds[0] = cidFD{lsubrs: lsubrs}
	shared := readCFF(cidSpec{cids: []int{1, 2, 3}, fds: fds}.build(), true, nil)
	if shared.state != ttParsed {
		t.Fatalf("300 font dicts carrying one Subrs index: %v (%s), want parsed", shared.state, shared.why)
	}
	first := &shared.widths.fds[0].lsubrs.offsets[0]
	for i, fd := range shared.widths.fds {
		if fd.lsubrs.count != 2000 || &fd.lsubrs.offsets[0] != first {
			t.Fatalf("font dict %d holds its own copy of the carried Subrs index (count %d) — one per dict is 0.5 MB each at 65,535 entries", i, fd.lsubrs.count)
		}
	}

	big := make([]byte, 1<<20) // a Private DICT of a million one-byte numbers, carried by every later font dict
	for i := range big {
		big[i] = 139
	}
	carried := make([]cidFD, 20)
	carried[0] = cidFD{private: big}
	for i := 1; i < len(carried); i++ {
		carried[i] = cidFD{noPrivate: true}
	}
	if c := readCFF(cidSpec{cids: []int{1, 2, 3}, fds: carried}.build(), true, nil); c.state != ttUnknown || !strings.Contains(c.why, "DICT bytes") {
		t.Errorf("20 font dicts re-reading a 1 MiB Private DICT: %v (%s), want a refusal naming the DICT budget", c.state, c.why)
	}
	own := make([]cidFD, 17) // each font dict's OWN Top DICT a mebibyte of numbers: the same budget
	for i := range own {
		own[i] = cidFD{extra: big}
	}
	if c := readCFF(cidSpec{cids: []int{1, 2, 3}, fds: own}.build(), true, nil); c.state != ttUnknown || !strings.Contains(c.why, "DICT bytes") {
		t.Errorf("17 font dicts of 1 MiB each: %v (%s), want a refusal naming the DICT budget", c.state, c.why)
	}
	// A Private DICT declared a gigabyte long is charged only what the program holds: it reads to the end and does not
	// parse, as veraPDF's does — never a refusal in veraPDF's place.
	long := readCFF(cidSpec{cids: []int{1, 2, 3}, fds: []cidFD{{private: append(dictInt(0), 21), privSize: 1 << 30}}}.build(), true, nil)
	if long.state != ttFailed {
		t.Errorf("a Private DICT declared 1 GiB long: %v (%s), want not parsed (the program ends first)", long.state, long.why)
	}
	carried = carried[:10] // 10 MiB is under the budget, and parses
	if c := readCFF(cidSpec{cids: []int{1, 2, 3}, fds: carried}.build(), true, nil); c.state != ttParsed {
		t.Errorf("10 font dicts re-reading a 1 MiB Private DICT: %v (%s), want parsed", c.state, c.why)
	}

	cids := make([]int, 1023)
	for i := range cids {
		cids[i] = i + 1
	}
	sel := func(ranges int) []byte { // ranges from 0, alternating up to 1024 and back to 0
		b := []byte{3, byte(ranges >> 8), byte(ranges), 0, 0}
		for r := 0; r < ranges; r++ {
			end := 1024
			if r%2 == 1 {
				end = 0
			}
			b = append(b, 0, byte(end>>8), byte(end))
		}
		return b
	}
	if c := readCFF(cidSpec{cids: cids, fdselect: sel(0xFFFF)}.build(), true, nil); c.state != ttUnknown || !strings.Contains(c.why, "FDSelect") {
		t.Errorf("FDSelect ranges refilling 1,024 glyphs 32,768 times: %v (%s), want a refusal naming FDSelect", c.state, c.why)
	}
	if c := readCFF(cidSpec{cids: cids, fdselect: sel(2000)}.build(), true, nil); c.state != ttParsed {
		t.Errorf("2,000 such ranges (a million writes): %v (%s), want parsed", c.state, c.why)
	}
}

// cidBackwards is a CID-keyed program whose CharStrings entry 2 runs backwards (its start past its end), which veraPDF
// reads with `Arrays.copyOfRange` and throws an IllegalArgumentException it does not handle.
func cidBackwards() []byte {
	p := cidSpec{cids: []int{1, 2, 3}, charstrings: [][]byte{cs(0, "endchar"), cs(500, "endchar"), cs(500, 1, "hmoveto"), cs(500, "endchar")}}.build()
	i := bytes.Index(p, []byte{0, 4, 1, 1})
	p[i+5] = p[i+6] + 1
	return p
}

// TestTheCIDBudgetsAreTheDocuments — the DICT and FDSelect bounds are the DOCUMENT's (`cffSpend`): two programs, each
// under the bound alone (the control), together over it — the second refused naming the document's CFF programs.
func TestTheCIDBudgetsAreTheDocuments(t *testing.T) {
	big := make([]byte, 1<<20)
	for i := range big {
		big[i] = 139
	}
	carried := make([]cidFD, 10) // ten font dicts re-reading a 1 MiB Private DICT: 10 MiB of the 16 MiB bound
	carried[0] = cidFD{private: big}
	for i := 1; i < len(carried); i++ {
		carried[i] = cidFD{noPrivate: true}
	}
	cids := make([]int, 1023)
	for i := range cids {
		cids[i] = i + 1
	}
	sel := []byte{3, 0x4E, 0x20, 0, 0} // 20,000 ranges alternating up to 1024 and back: ~10.2 million writes of 16.8
	for r := 0; r < 20000; r++ {
		end := 1024
		if r%2 == 1 {
			end = 0
		}
		sel = append(sel, 0, byte(end>>8), byte(end))
	}
	for _, c := range []struct {
		name, want string
		prog       []byte
		spent      func(*cffSpend) (int, int)
	}{
		{"DICT bytes", "DICT bytes", cidSpec{cids: []int{1, 2, 3}, fds: carried}.build(), func(s *cffSpend) (int, int) { return s.dict, cffMaxDictBytes }},
		{"FDSelect writes", "FDSelect", cidSpec{cids: cids, fdselect: sel}.build(), func(s *cffSpend) (int, int) { return s.fill, cffMaxFDSelectFill }},
	} {
		spend := &cffSpend{}
		if p := readCFF(c.prog, true, spend); p.state != ttParsed {
			t.Fatalf("%s: one program: %v (%s), want parsed", c.name, p.state, p.why)
		}
		if n, bound := c.spent(spend); n < bound/2 || n > bound*3/4 {
			t.Fatalf("%s: one program spent %d, want between half and three quarters of the bound %d", c.name, n, bound)
		}
		if p := readCFF(c.prog, true, nil); p.state != ttParsed {
			t.Fatalf("%s: the second program alone: %v (%s), want parsed (the control)", c.name, p.state, p.why)
		}
		if p := readCFF(c.prog, true, spend); p.state != ttUnknown || !strings.Contains(p.why, "the document's CFF programs") || !strings.Contains(p.why, c.want) {
			t.Errorf("%s: the second program after the first: %v (%s), want refused naming the document's CFF programs and %q", c.name, p.state, p.why, c.want)
		}
	}
}
