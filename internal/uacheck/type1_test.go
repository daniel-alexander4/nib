package uacheck

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Type 1 (/FontFile) program — `PLAN-ua-coverage.md` P07.S06. Every fixture's veraPDF letters were measured on
// veraPDF 1.30.2 before they were written here; the live oracle asks veraPDF again on every run.

var type1Clauses = []string{"7.21.5 t1", "7.21.4.1 t2", "7.21.8 t1", "7.21.4.2 t1", "7.21.4.1 t1"}

// t1n is a Type 1 charstring number.
func t1n(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		return []byte{byte((v-108)>>8 + 247), byte((v - 108) & 0xff)}
	case v >= -1131 && v <= -108:
		return []byte{byte((-v-108)>>8 + 251), byte((-v - 108) & 0xff)}
	}
	u := uint32(int32(v))
	return []byte{255, byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u)}
}

// t1cs is a plain Type 1 charstring: ints are numbers, strings operators.
func t1cs(parts ...any) []byte {
	ops := map[string][]byte{"hstem": {1}, "vstem": {3}, "vmoveto": {4}, "rlineto": {5}, "hlineto": {6}, "closepath": {9},
		"callsubr": {10}, "return": {11}, "hsbw": {13}, "endchar": {14}, "rmoveto": {21}, "sbw": {12, 7}, "div": {12, 12}, "hstem3": {12, 2}, "vstem3": {12, 1},
		"pop": {12, 17}, "callothersubr": {12, 16}, "seac": {12, 6}, "dotsection": {12, 0}}
	var out []byte
	for _, p := range parts {
		switch v := p.(type) {
		case int:
			out = append(out, t1n(v)...)
		case string:
			op, ok := ops[v]
			if !ok {
				panic("unknown charstring operator " + v)
			}
			out = append(out, op...)
		case []byte:
			out = append(out, v...)
		}
	}
	return out
}

// eexecEncrypt is the Type 1 cipher's encryption, the inverse of `eexecDecrypt`.
func eexecEncrypt(plain []byte, key uint32) []byte {
	out := make([]byte, len(plain))
	r := key
	for i, p := range plain {
		c := p ^ byte(r>>8)
		out[i] = c
		r = ((uint32(c)+r)*52845 + 22719) & 0xffff
	}
	return out
}

type t1Glyph struct {
	name string
	cs   []byte
}

// t1Spec is a synthetic Type 1 program. Zero values give a well-formed three-glyph font (A, B, C at 500).
type t1Spec struct {
	clear    string    // the whole cleartext through `currentfile eexec\n`; "" is t1Clear(matrix, enc)
	matrix   string    // the /FontMatrix array; "" the default
	enc      string    // the cleartext's /Encoding definition; "" an array naming A B C at 65-67
	pre      string    // cleartext ahead of the font dictionary
	lenIV    string    // the /lenIV line, "" for none
	iv       []byte    // the random bytes ahead of each charstring; nil is four zeros
	subrs    [][]byte  // plain subroutines
	glyphs   []t1Glyph // nil: /.notdef, A, B, C, each `0 500 hsbw endchar`
	count    int       // the /CharStrings count; 0 is len(glyphs)
	noCS     bool      // no /CharStrings at all
	priv     string    // private text ahead of /Subrs
	trailer  string    // after the eexec section
	cut      int       // keep only this many eexec bytes (0: all)
	eexecKey []byte    // the four random bytes of the eexec section; nil is four zeros
	trim     int       // drop this many bytes from the end of the eexec section
	rd       [3]string // the RD, ND and NP tokens; zero is "RD", "ND", "NP"
	clearCS  bool      // charstrings not encrypted at all
}

func t1Clear(pre, matrix, enc string) string {
	if matrix == "" {
		matrix = "[0.001 0 0 0.001 0 0]"
	}
	if enc == "" {
		enc = "/Encoding 256 array\n0 1 255 {1 index exch /.notdef put} for\ndup 65 /A put\ndup 66 /B put\ndup 67 /C put\nreadonly def\n"
	}
	return "%!PS-AdobeFont-1.0: Probe 001.000\n" + pre + "11 dict begin\n/FontInfo 2 dict dup begin\n/Notice (probe \\050c\\051) readonly def\nend readonly def\n" +
		"/FontName /Probe def\n/PaintType 0 def\n/FontType 1 def\n/FontMatrix " + matrix + " readonly def\n" + enc +
		"/FontBBox {0 0 1000 1000} readonly def\ncurrentdict end\ncurrentfile eexec\n"
}

func (s t1Spec) charString(plain []byte) []byte {
	iv := s.iv
	if iv == nil {
		iv = []byte{0, 0, 0, 0}
	}
	if s.clearCS {
		return append(append([]byte(nil), iv...), plain...)
	}
	return eexecEncrypt(append(append([]byte(nil), iv...), plain...), 4330)
}

func (s t1Spec) build() []byte {
	clear := s.clear
	if clear == "" {
		clear = t1Clear(s.pre, s.matrix, s.enc)
	}
	var p bytes.Buffer
	p.WriteString("dup /Private 8 dict dup begin\n/RD{string currentfile exch readstring pop}executeonly def\n" +
		"/ND{noaccess def}executeonly def\n/NP{noaccess put}executeonly def\n/password 5839 def\n")
	rd := s.rd
	if rd[0] == "" {
		rd = [3]string{"RD", "ND", "NP"}
	}
	p.WriteString(s.lenIV)
	p.WriteString(s.priv)
	if len(s.subrs) > 0 {
		fmt.Fprintf(&p, "/Subrs %d array\n", len(s.subrs))
		for i, sub := range s.subrs {
			b := s.charString(sub)
			fmt.Fprintf(&p, "dup %d %d %s ", i, len(b), rd[0])
			p.Write(b)
			p.WriteString(" " + rd[2] + "\n")
		}
		p.WriteString("ND\n")
	}
	glyphs := s.glyphs
	if glyphs == nil {
		glyphs = []t1Glyph{{".notdef", t1cs(0, 500, "hsbw", "endchar")}, {"A", t1cs(0, 500, "hsbw", "endchar")},
			{"B", t1cs(0, 500, "hsbw", "endchar")}, {"C", t1cs(0, 500, "hsbw", "endchar")}}
	}
	if !s.noCS {
		n := s.count
		if n == 0 {
			n = len(glyphs)
		}
		fmt.Fprintf(&p, "2 index /CharStrings %d dict dup begin\n", n)
		for _, g := range glyphs {
			b := s.charString(g.cs)
			fmt.Fprintf(&p, "/%s %d %s ", g.name, len(b), rd[0])
			p.Write(b)
			p.WriteString(" " + rd[1] + "\n")
		}
		p.WriteString("end\n")
	}
	p.WriteString("end\nreadonly put\nnoaccess put\ndup/FontName get exch definefont pop\nmark currentfile closefile\n")
	key := s.eexecKey
	if key == nil {
		key = []byte{0, 0, 0, 0}
	}
	enc := eexecEncrypt(append(append([]byte(nil), key...), p.Bytes()...), 55665)
	if s.cut > 0 && s.cut < len(enc) {
		enc = enc[:s.cut]
	}
	enc = enc[:len(enc)-s.trim]
	return append(append([]byte(clear), enc...), s.trailer...)
}

// t1Doc is a tagged page drawing `show` in a simple Type 1 font over the /FontFile program `prog` (none when nil), with
// `font` entries (/Widths, /Encoding, …), `desc` entries (/CharSet, /MissingWidth, …) and the stream's `filter`.
func t1Doc(base, font, desc, show string, prog []byte, filter string) []byte {
	objs := map[int]string{
		12: "<< /Type /FontDescriptor /FontName /" + base + " /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 " +
			"/Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 " + desc,
	}
	if prog != nil {
		objs[12] += " /FontFile 20 0 R"
		if !strings.Contains(filter, "/Length1") { // pdfcpu requires them; veraPDF reads none of the three
			l1 := bytes.Index(prog, []byte("eexec")) + 6
			if l1 < 6 || l1 > len(prog) {
				l1 = len(prog)
			}
			filter += fmt.Sprintf(" /Length1 %d /Length2 %d /Length3 0", l1, len(prog)-l1)
		}
		objs[20] = spStream(filter, string(prog))
	}
	objs[12] += " >>"
	return glyphDoc("<< /Type /Font /Subtype /Type1 /BaseFont /"+base+" /FontDescriptor 12 0 R "+font+" >>", show, objs)
}

// realT1 is a pdfLaTeX-embedded program's stream: `raw` its Flate-compressed bytes, `dec` them decoded.
func realT1(b64 string) (raw, dec []byte) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(err)
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		panic(err)
	}
	dec, err = io.ReadAll(zr)
	if err != nil {
		panic(err)
	}
	return raw, dec
}

// t1Edit rewrites a program's private part: `edit` receives it decrypted (its four random bytes included) and
// returns the new text, which is encrypted again.
// t1Swap is an edit replacing `old` with `new` once, which panics where `old` is absent: a fixture named for a shape
// it does not have would be measured, and asserted, as something else.
func t1Swap(old, new string) func([]byte) []byte {
	return func(p []byte) []byte {
		if !bytes.Contains(p, []byte(old)) {
			panic(fmt.Sprintf("t1Swap: %q is not in the private part", old))
		}
		return bytes.Replace(p, []byte(old), []byte(new), 1)
	}
}

// t1ToEnd sets the length after `head` (a "... 9 RD " the program holds) so that its data runs EXACTLY to the last byte
// of the decrypted private part veraPDF reads: `getStreamUntilToken` keeps all but the eexec section's last byte (there
// is no cleartomark), zero-padded to 10240, of which decryption drops 4 — so the part is 10236 bytes, and `rest` is
// what lies past the data's start. It panics unless the program is that shape.
func t1ToEnd(prog []byte, head string) []byte {
	i := bytes.Index(prog, []byte("currentfile eexec\n")) + len("currentfile eexec\n")
	if bytes.Contains(prog[i:], []byte("cleartomark")) || len(prog)-i-1 >= 10240 {
		panic("t1ToEnd: not an in-memory private part without cleartomark")
	}
	n := 9
	for {
		priv := eexecDecrypt(prog[i:], 55665)
		at := bytes.Index(priv, []byte(head))
		if at < 0 {
			panic("t1ToEnd: no " + head)
		}
		next := strings.Replace(head, " 9 RD ", fmt.Sprintf(" %d RD ", n), 1)
		begin := at + len(next) - 4 // in the decoded part, which has lost its 4 random bytes
		if want := 10236 - begin; want != n {
			n = want
			continue
		}
		return t1Edit(prog, t1Swap(head, next))
	}
}

func t1Edit(prog []byte, edit func(priv []byte) []byte) []byte {
	i := bytes.Index(prog, []byte("currentfile eexec\n")) + len("currentfile eexec\n")
	priv := eexecDecrypt(prog[i:], 55665)
	return append(append([]byte(nil), prog[:i]...), eexecEncrypt(edit(priv), 55665)...)
}

// t1EditGlyph rewrites one charstring (by its /name) through `edit`, which receives and returns it decrypted without
// its four random bytes.
func t1EditGlyph(prog []byte, name string, edit func(cs []byte) []byte) []byte {
	return t1Edit(prog, func(priv []byte) []byte {
		head := []byte("/" + name + " ")
		at := bytes.Index(priv, head)
		if at < 0 {
			panic("no glyph " + name)
		}
		j := at + len(head)
		var n int
		k := bytes.Index(priv[j:], []byte(" RD "))
		fmt.Sscanf(string(priv[j:j+k]), "%d", &n)
		start := j + k + 4
		cs := eexecDecrypt(priv[start:start+n], 4330)
		ne := eexecEncrypt(append(cs[:4:4], edit(cs[4:])...), 4330)
		var out []byte
		out = append(out, priv[:j]...)
		out = append(out, fmt.Sprintf("%d RD ", len(ne))...)
		out = append(out, ne...)
		return append(out, priv[start+n:]...)
	})
}

// t1Hex is a program with its eexec section written in hexadecimal, as a PFA may carry it.
func t1Hex(prog []byte) []byte {
	i := bytes.Index(prog, []byte("currentfile eexec\n")) + len("currentfile eexec\n")
	out := append([]byte(nil), prog[:i]...)
	for k, c := range prog[i:] {
		out = append(out, fmt.Sprintf("%02x", c)...)
		if k%32 == 31 {
			out = append(out, '\n')
		}
	}
	return out
}

func type1Fixtures() []measuredFixture {
	cmRaw, cm := realT1(cmType1)
	tmRaw, tm := realT1(timesType1)
	cmW := "/FirstChar 72 /LastChar 105 /Widths [750 361.1 513.9 777.8 625 916.7 750 777.8 680.6 777.8 736.1 555.6 722.2 750 750 1027.8 750 750 611.1 277.8 500 277.8 500 277.8 277.8 500 555.6 444.4 555.6 444.4 305.6 500 555.6 277.8]"
	tmW := "/FirstChar 72 /LastChar 105 /Widths [722 333 389 722 611 889 722 722 556 722 667 556 611 722 722 944 722 722 611 333 278 333 469 500 333 444 500 444 500 444 333 500 500 278] /Encoding << /Type /Encoding /Differences [72 /H 105 /i] >>"
	cmBase, tmBase := "PKDNZC+CMR10", "NCEITK+NimbusRomNo9L-Regu"
	hi := "/CharSet (/H/i)"
	flate := "/Filter /FlateDecode /Length1 1378 /Length2 8167 /Length3 0"
	flateTm := "/Filter /FlateDecode /Length1 1630 /Length2 5668 /Length3 0"
	tmEdit := func(name string, edit func([]byte) []byte) []byte { return t1EditGlyph(tm, name, edit) }

	sub := "ABCDEF+Probe"
	w500 := "/FirstChar 65 /LastChar 67 /Widths [500 500 500]"
	w500E := w500 + " /Encoding /WinAnsiEncoding"
	full := "/CharSet (/A/B/C)"
	show := "(ABC) Tj"
	std := t1Spec{}.build()
	g := func(name string, cs []byte) t1Glyph { return t1Glyph{name, cs} }
	hsbw := func(w int) []byte { return t1cs(0, w, "hsbw", "endchar") }
	nd := g(".notdef", hsbw(0))
	abc := func(a, b, c []byte) []t1Glyph { return []t1Glyph{nd, g("A", a), g("B", b), g("C", c)} }
	pad := "/Pad (" + strings.Repeat("x", 12000) + ") def\n"

	opDoc := func(pre, put string) []byte {
		enc := "/Encoding 256 array\n0 1 255 {1 index exch /.notdef put} for\n" + put + "dup 66 /B put\ndup 67 /C put\nreadonly def\n"
		return t1Doc(sub, w500, full, show, t1Spec{pre: pre, enc: enc}.build(), "")
	}
	return []measuredFixture{
		// pdfLaTeX output, as produced and mutated.
		{name: "PDFTEX: Computer Modern", vera: "PPPPP", pdf: t1Doc(cmBase, cmW, hi, "(Hi) Tj", cmRaw, flate)},
		{name: "PDFTEX: Nimbus Roman (times)", vera: "PPPPP", pdf: t1Doc(tmBase, tmW, hi, "(Hi) Tj", tmRaw, flateTm)},
		{name: "PDFTEX: CM, uncompressed", vera: "PPPPP", pdf: t1Doc(cmBase, cmW, hi, "(Hi) Tj", cm, "")},
		{name: "PDFTEX: CM, a CharSet missing /i", vera: "PPPFP", pdf: t1Doc(cmBase, cmW, "/CharSet (/H)", "(Hi) Tj", cmRaw, flate)},
		{name: "PDFTEX: CM, a CharSet listing /.notdef", vera: "PPPPP", pdf: t1Doc(cmBase, cmW, "/CharSet (/.notdef/H/i)", "(Hi) Tj", cmRaw, flate)},
		{name: "PDFTEX: CM, /Widths H 850", vera: "FPPPP", pdf: t1Doc(cmBase, strings.Replace(cmW, "[750 ", "[850 ", 1), hi, "(Hi) Tj", cmRaw, flate)},
		{name: "PDFTEX: CM, draws a glyph the subset lacks", vera: "PFPPP", pdf: t1Doc(cmBase, cmW, hi, "(Ha) Tj", cmRaw, flate)},
		{name: "PDFTEX: CM, eexec cut to 200 bytes", vera: "PPPPF", pdf: t1Doc(cmBase, cmW, hi, "(Hi) Tj", cm[:1378+200], "")},
		{name: "PDFTEX: CM, the 512-zero trailer and cleartomark", vera: "PPPPP", pdf: t1Doc(cmBase, cmW, hi, "(Hi) Tj", append(append([]byte(nil), cm...), "\n"+strings.Repeat(strings.Repeat("0", 64)+"\n", 8)+"cleartomark\n"...), "")},
		{name: "PDFTEX: CM, eexec in hexadecimal", vera: "PPPPF", pdf: t1Doc(cmBase, cmW, hi, "(Hi) Tj", t1Hex(cm), "")},
		{name: "PDFTEX: times, H's hsbw 900", vera: "FPPPP", pdf: t1Doc(tmBase, tmW, hi, "(Hi) Tj", tmEdit("H", func(cs []byte) []byte {
			// pdfLaTeX writes `sbx wx hsbw` first: replace it.
			i := bytes.IndexByte(cs, 13)
			return append(t1cs(0, 900, "hsbw"), cs[i+1:]...)
		}), "")},
		{name: "PDFTEX: times, i's charstring with no width", vera: "FFPFP", pdf: t1Doc(tmBase, tmW, hi, "(Hi) Tj", tmEdit("i", func([]byte) []byte { return t1cs("endchar") }), "")},

		// Synthetic programs, one branch each.
		{name: "T1: widths agree", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, std, "")},
		{name: "T1: a width 100 off", vera: "FPPPP", pdf: t1Doc(sub, "/FirstChar 65 /LastChar 67 /Widths [600 500 500] /Encoding /WinAnsiEncoding", full, show, std, "")},
		{name: "T1: a name the program lacks", vera: "FFPPP", pdf: t1Doc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 500 500] /Encoding /WinAnsiEncoding", full, "(D) Tj", std, "")},
		{name: "T1: sbw", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{glyphs: abc(t1cs(0, 0, 500, 0, "sbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: sbw, a width 100 off", vera: "FPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{glyphs: abc(t1cs(0, 0, 600, 0, "sbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: a width through callsubr", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{subrs: [][]byte{t1cs("return"), t1cs(0, 500, "hsbw", "return")}, glyphs: abc(t1cs(1, "callsubr", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: callsubr to a subroutine with no width", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{subrs: [][]byte{t1cs("return")}, glyphs: abc(t1cs(0, "callsubr", 0, 500, "hsbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: callsubr, the width 100 off", vera: "FPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{subrs: [][]byte{t1cs(0, 600, "hsbw", "return")}, glyphs: abc(t1cs(0, "callsubr", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: a subroutine whose ciphertext starts with a space", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{iv: []byte{0x30, 0, 0, 0}, subrs: [][]byte{t1cs(0, 600, "hsbw", "return")}, glyphs: abc(t1cs(0, "callsubr", 0, 500, "hsbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: a width computed by div", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{glyphs: abc(t1cs(0, 1000, 2, "div", "hsbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: a five-byte width", vera: "PPPPP", pdf: t1Doc(sub, "/FirstChar 65 /LastChar 67 /Widths [70000 500 500] /Encoding /WinAnsiEncoding", full, show, t1Spec{glyphs: abc(hsbw(70000), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: lenIV 0", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV 0 def\n", iv: []byte{}}.build(), "")},
		{name: "T1: lenIV -1, charstrings in the clear", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV -1 def\n", iv: []byte{}}.build(), "")},
		{name: "T1: lenIV 5000", vera: "FFPFP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV 5000 def\n"}.build(), "")},
		{name: "T1: a FontMatrix of 0.002", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{matrix: "[0.002 0 0 0.002 0 0]", glyphs: abc(hsbw(250), hsbw(250), hsbw(250))}.build(), "")},
		{name: "T1: a FontMatrix of 0.002, unscaled widths", vera: "FPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{matrix: "[0.002 0 0 0.002 0 0]"}.build(), "")},
		{name: "T1: a FontMatrix of seven numbers", vera: "XXXXX", pdf: t1Doc(sub, w500E, full, show, t1Spec{matrix: "[0.001 0 0 0.001 0 0 0]"}.build(), "")},
		{name: "T1: a FontMatrix of four numbers", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{matrix: "[0.001 0 0 0.001]"}.build(), "")},
		{name: "T1: StandardEncoding, no PDF encoding", vera: "PPPPP", pdf: t1Doc(sub, w500, full, show, t1Spec{enc: "/Encoding StandardEncoding def\n"}.build(), "")},
		{name: "T1: StandardEncoding, an unnamed code 0x44", vera: "FFPPP", pdf: t1Doc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 500 500]", full, "(ABCD) Tj", t1Spec{enc: "/Encoding StandardEncoding def\n"}.build(), "")},
		{name: "T1: its own array encoding, no PDF encoding", vera: "PPPPP", pdf: t1Doc(sub, w500, full, show, std, "")},
		{name: "T1: its own array encoding, code 0x44 unnamed", vera: "PFPPP", pdf: t1Doc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 500 500]", full, "(ABCD) Tj", std, "")},
		{name: "T1: its own array encoding, a width 100 off", vera: "FPPPP", pdf: t1Doc(sub, "/FirstChar 65 /LastChar 67 /Widths [600 500 500]", full, show, std, "")},
		{name: "T1: an encoding naming no StandardEncoding", vera: "FFPPP", pdf: t1Doc(sub, w500, full, show, t1Spec{enc: "/Encoding /Other def\n"}.build(), "")},
		{name: "T1: an encoding entry that is a string", vera: "PPPPP", pdf: t1Doc(sub, w500, full, show, t1Spec{enc: "/Encoding 256 array\n0 1 255 {1 index exch /.notdef put} for\ndup 65 (A) put\ndup 66 /B put\ndup 67 /C put\nreadonly def\n"}.build(), "")},
		{name: "T1: a radix code 8#101", vera: "PPPPP", pdf: t1Doc(sub, w500, full, show, t1Spec{enc: "/Encoding 256 array\n0 1 255 {1 index exch /.notdef put} for\ndup 8#101 /A put\ndup 66 /B put\ndup 67 /C put\nreadonly def\n"}.build(), "")},
		{name: "T1: a radix code 16#41", vera: "PPPPP", pdf: t1Doc(sub, w500, full, show, t1Spec{enc: "/Encoding 256 array\n0 1 255 {1 index exch /.notdef put} for\ndup 16#41 /A put\ndup 66 /B put\ndup 67 /C put\nreadonly def\n"}.build(), "")},
		{name: "T1: a code named /.notdef", vera: "PFFPP", pdf: t1Doc(sub, w500+" /Encoding << /Differences [65 /.notdef] >>", full, show, std, "")},
		{name: "T1: code 0 named /.notdef", vera: "FPFPP", pdf: t1Doc(sub, "/FirstChar 0 /LastChar 67 /Widths ["+strings.Repeat("0 ", 65)+"500 500 500] /Encoding << /Differences [0 /.notdef] >>", full, "(\\000ABC) Tj", std, "")},
		{name: "T1: no /CharStrings", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{noCS: true}.build(), "")},
		{name: "T1: a charstring count past the glyphs, then end", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{count: 9}.build(), "")},
		{name: "T1: a glyph with no width", vera: "FFPFP", pdf: t1Doc(sub, w500E, full, show, t1Spec{glyphs: abc(t1cs("endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: a glyph with no width, not drawn, CharSet lists it", vera: "PPPFP", pdf: t1Doc(sub, w500E, full, "(BC) Tj", t1Spec{glyphs: abc(t1cs("endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "CS: a CharSet missing a glyph", vera: "PPPFP", pdf: t1Doc(sub, w500E, "/CharSet (/A/B)", show, std, "")},
		{name: "CS: a CharSet with a glyph too many", vera: "PPPFP", pdf: t1Doc(sub, w500E, "/CharSet (/A/B/C/D)", show, std, "")},
		{name: "CS: a CharSet with .notdef, the program holding it", vera: "PPPPP", pdf: t1Doc(sub, w500E, "/CharSet (/.notdef/A/B/C)", show, std, "")},
		{name: "CS: a CharSet with .notdef, the program not holding it", vera: "PPPFP", pdf: t1Doc(sub, w500E, "/CharSet (/.notdef/A/B/C)", show, t1Spec{glyphs: abc(hsbw(500), hsbw(500), hsbw(500))[1:]}.build(), "")},
		{name: "CS: unparsed program, CharSet short", vera: "PPPPF", pdf: t1Doc(sub, w500E, "/CharSet (/A)", show, t1Spec{noCS: true}.build(), "")},
		{name: "PS: the prologue real fonts carry", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "FontDirectory/Probe known{/Probe findfont dup/UniqueID known{dup\n/UniqueID get 5000793 eq exch/FontType get 1 eq and}{pop false}ifelse\n{save true}{false}ifelse}{false}ifelse\n"}.build(), "")},
		{name: "PS: pop on an empty stack", vera: "XXXXX", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "pop\n"}.build(), "")},
		{name: "PS: dup on an empty stack", vera: "XXXXX", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "dup\n"}.build(), "")},
		{name: "PS: idiv by zero", vera: "XXXXX", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "1 0 idiv pop\n"}.build(), "")},
		{name: "PS: add on an empty stack", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "clear add\n"}.build(), "")},
		{name: "PS: cleartomark with no mark", vera: "XXXXX", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "1 2 cleartomark\n"}.build(), "")},
		{name: "PS: a for of 20000 iterations", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "0 1 20000 {pop} for\n"}.build(), "")},
		{name: "PS: a stray >", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "> \n"}.build(), "")},
		{name: "PS: an unclosed array", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{pre: "[1 2 )\n"}.build(), "")},
		{name: "PS: eexec with too few bytes after it", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, append([]byte(t1Clear("", "", "")), "abc"...), "")},
		{name: "PS: a PFB segment header", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, append([]byte{0x80, 1, 0, 0, 0, 0}, std...), "")},
		{name: "SIZE: a charstring past the end, in memory", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Edit(std, t1Swap("/C 9 RD ", "/C 90000 RD ")), "")},
		{name: "SIZE: a charstring past the end, from a temporary file", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Edit(t1Spec{priv: pad}.build(), t1Swap("/C 9 RD ", "/C 90000 RD ")), "")},
		{name: "T1: a radix code 16#4F, read as 4", vera: "PFPPP", pdf: t1Doc(sub, w500, full, show, t1Spec{enc: "/Encoding 256 array\n0 1 255 {1 index exch /.notdef put} for\ndup 16#4F /A put\ndup 66 /B put\ndup 67 /C put\nreadonly def\n"}.build(), "")},
		{name: "T1: -| and |- for RD and ND", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{rd: [3]string{"-|", "|-", "|"}, subrs: [][]byte{t1cs(0, 500, "hsbw", "return")}, glyphs: abc(t1cs(0, "callsubr", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: lenIV -1, charstrings not encrypted", vera: "FFPFP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV -1 def\n", iv: []byte{}, clearCS: true}.build(), "")},
		{name: "T1: the eexec section ending at closefile's last byte", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{trim: 1}.build(), "")},
		{name: "T1: the eexec section ending two bytes into closefile", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{trim: 3}.build(), "")},
		{name: "T1: /Encoding redefined after eexec", vera: "PPPPP", pdf: t1Doc(sub, w500, full, show, t1Spec{trailer: "\n/Encoding StandardEncoding def\n"}.build(), "")},
		{name: "T1: a FontMatrix given as a procedure", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{matrix: "{0.002 0 0 0.002 0 0}", glyphs: abc(hsbw(250), hsbw(250), hsbw(250))}.build(), "")},
		{name: "BUF: the last refill brings no bytes", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Refill(-1), "")},
		{name: "BUF: the last refill brings 5 bytes", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Refill(5), "")},
		{name: "BUF: the last refill brings 11 bytes", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Refill(11), "")},
		{name: "BUF: the last refill brings 12 bytes", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Refill(12), "")},
		{name: "T1: lenIV 2100, past the first read", vera: "FFPFP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV 2100 def\n", iv: make([]byte, 2100)}.build(), "")},
		{name: "T1: lenIV 2000, inside the first read", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV 2000 def\n", iv: make([]byte, 2000)}.build(), "")},
		{name: "T1: a stray > after cleartomark", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{trailer: "\n" + strings.Repeat("0", 64) + "\ncleartomark\n> \n"}.build(), "")},
		{name: "T1: /Encoding redefined after cleartomark", vera: "FFPPP", pdf: t1Doc(sub, "/FirstChar 65 /LastChar 68 /Widths [500 500 500 500]", full, "(ABCD) Tj", t1Spec{trailer: "\ncleartomark\n/Encoding StandardEncoding def\n"}.build(), "")},
		// Each operator's RESULT decides where /A lands in the program's own encoding: at 65 the font draws A (P), anywhere
		// else code 65 is .notdef there (F on the width and presence). Top-level `if`, `ifelse` and `bitshift` are not in
		// OPERATORS_KEYWORDS, so they run inside a one-pass `for`.
		{name: "OP: roll 2 1", vera: "PPPPP", pdf: opDoc("", "dup 66 65 2 1 roll pop /A put\n")},
		{name: "OP: roll 3 -1", vera: "PPPPP", pdf: opDoc("", "dup 70 65 70 3 -1 roll pop pop /A put\n")},
		{name: "OP: copy", vera: "PPPPP", pdf: opDoc("", "dup 65 1 copy pop /A put\n")},
		{name: "OP: copy past the stack", vera: "PPPPF", pdf: opDoc("", "dup 65 50 copy pop /A put\n")},
		{name: "OP: if true", vera: "PFPPP", pdf: opDoc("", "0 1 0 {pop dup 65 true {pop 66} if /A put} for\n")},
		{name: "OP: if false", vera: "PPPPP", pdf: opDoc("", "0 1 0 {pop dup 65 false {pop 66} if /A put} for\n")},
		{name: "OP: ifelse true", vera: "PPPPP", pdf: opDoc("", "0 1 0 {pop dup true {65} {66} ifelse /A put} for\n")},
		{name: "OP: ifelse false", vera: "PFPPP", pdf: opDoc("", "0 1 0 {pop dup false {65} {66} ifelse /A put} for\n")},
		{name: "OP: counttomark", vera: "PPPPP", pdf: opDoc("mark 1 2 3 counttomark /k exch def cleartomark pop\n", "dup k 62 add /A put\n")},
		{name: "OP: counttomark with no mark", vera: "PPPPP", pdf: opDoc("clear 7 8 9 counttomark /k exch def clear\n", "dup k 63 add /A put\n")},
		{name: "OP: bitshift", vera: "PPPPP", pdf: opDoc("", "0 1 0 {pop dup 130 bitshift /A put} for\n")},
		{name: "OP: bitshift of a negative", vera: "PPPPF", pdf: opDoc("", "0 1 0 {pop dup -130 bitshift /A put} for\n")},
		{name: "OP: put out of range", vera: "PPPPF", pdf: opDoc("", "dup 300 /A put\n")},
		{name: "OP: def with a string key", vera: "PPPPP", pdf: opDoc("(k) 65 def\n", "dup k /A put\n")},
		{name: "OP: def with exactly two operands", vera: "PPPPP", pdf: opDoc("clear /k 65 def\n", "dup k /A put\n")},
		{name: "T1: Subrs ending noaccess put", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{rd: [3]string{"RD", "ND", "noaccess put"}, subrs: [][]byte{t1cs("return"), t1cs(0, 500, "hsbw", "return")}, glyphs: abc(t1cs(1, "callsubr", 0, 600, "hsbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: a one-byte subroutine", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV 0 def\n", iv: []byte{}, subrs: [][]byte{t1cs("return"), t1cs(0, 500, "hsbw", "return")}, glyphs: abc(t1cs(1, "callsubr", 0, 600, "hsbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: seac ahead of hsbw", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{glyphs: abc(t1cs(0, 500, 1, 2, 3, 4, 5, "seac", "hsbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: hstem3 ahead of hsbw", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{glyphs: abc(t1cs(0, 500, 1, 2, 3, 4, 5, 6, "hstem3", "hsbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: vstem3 ahead of hsbw", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{glyphs: abc(t1cs(0, 500, 1, 2, 3, 4, 5, 6, "vstem3", "hsbw", "endchar"), hsbw(500), hsbw(500))}.build(), "")},
		{name: "T1: lenIV -6144", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV -6144 def\n", iv: []byte{}}.build(), "")},
		{name: "T1: lenIV -6145, a first read short of 2048", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV -6145 def\n", iv: []byte{}}.build(), "")},
		{name: "T1: lenIV -8191", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV -8191 def\n", iv: []byte{}}.build(), "")},
		{name: "T1: lenIV 1048576", vera: "FFPFP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV 1048576 def\n"}.build(), "")},
		{name: "T1: lenIV -8192", vera: "FFPFP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV -8192 def\n", iv: []byte{}}.build(), "")},
		{name: "T1: lenIV -8193", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV -8193 def\n", iv: []byte{}}.build(), "")},
		{name: "T1: lenIV 2147475456", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV 2147475456 def\n"}.build(), "")},
		{name: "T1: lenIV 2147475455", vera: "FFPFP", pdf: t1Doc(sub, w500E, full, show, t1Spec{lenIV: "/lenIV 2147475455 def\n"}.build(), "")},
		{name: "SIZE: a subroutine length just short of 2^63, in memory", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Edit(t1Spec{subrs: [][]byte{t1cs("return")}}.build(), t1Swap("dup 0 5 RD ", "dup 0 9223372032559808517 RD ")), "")},
		{name: "SIZE: a subroutine of 2^63-1 bytes, in memory", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Edit(t1Spec{subrs: [][]byte{t1cs("return")}}.build(), t1Swap("dup 0 5 RD ", "dup 0 99999999999999999999 RD ")), "")},
		{name: "SIZE: a subroutine of 2^63-1 bytes, from a temporary file", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Edit(t1Spec{priv: pad, subrs: [][]byte{t1cs("return")}}.build(), t1Swap("dup 0 5 RD ", "dup 0 99999999999999999999 RD ")), "")},
		{name: "SIZE: a subroutine past the end, in memory", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Edit(t1Spec{subrs: [][]byte{t1cs("return")}}.build(), t1Swap("dup 0 5 RD ", "dup 0 90000 RD ")), "")},
		{name: "SIZE: a charstring of negative length, in memory", vera: "PPPPF", pdf: t1Doc(sub, w500E, full, show, t1Edit(std, t1Swap("/C 9 RD ", "/C -9 RD ")), "")},
		{name: "SIZE: a charstring ending at the private part's last byte, in memory", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1ToEnd(std, "/C 9 RD "), "")},
		{name: "SIZE: a subroutine ending at the private part's last byte, in memory", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1ToEnd(t1Edit(std, t1Swap("ND\nend\n", "ND\nend\n/Subrs 1 array\ndup 0 9 RD ")), "dup 0 9 RD "), "")},
		{name: "OP: a mark put into a procedure run by if", vera: "PPPPP", pdf: opDoc("{0} dup 0 mark put /P exch def\n", "0 1 0 {pop dup 5 true P if pop 60 add /A put} for\n")},
		{name: "OP: a for of exactly 10000 iterations", vera: "PPPPP", pdf: opDoc("0 1 9999 {pop} for\n", "dup 65 /A put\n")},
		{name: "OP: a for of 10001 iterations", vera: "PPPPF", pdf: opDoc("0 1 10000 {pop} for\n", "dup 65 /A put\n")},
		{name: "T1: a FontMatrix of 0.001004, a width of 750 read as 753", vera: "PPPPP", pdf: t1Doc(sub, "/FirstChar 65 /LastChar 67 /Widths [754 754 754] /Encoding /WinAnsiEncoding", full, show, t1Spec{matrix: "[0.001004 0 0 0.001004 0 0]", glyphs: abc(hsbw(750), hsbw(750), hsbw(750))}.build(), "")},
		{name: "SIZE: a large private part", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Spec{priv: pad}.build(), "")},
	}
}

// t1Refill is a program whose cleartext is padded so that `ASBufferedInFilter`'s last refill before `eexec` brings
// `r` bytes (-1: none, the stream exhausted), which sends `getStreamUntilToken`'s first `read` down its second path when
// r is 11 or fewer.
func t1Refill(r int) []byte {
	base := t1Spec{}.build()
	target := r
	if r < 0 {
		target = 0
	}
	// (L - 1024) mod 513 == target, with the pad a comment line of at least two bytes.
	pad := ((target-(len(base)+2-1024))%513 + 513) % 513
	prog := t1Spec{pre: "%" + strings.Repeat("x", pad) + "\n"}.build()
	// The shape the name claims: past the 1024 bytes the first read takes, `target` more past a whole number of
	// 513-byte refills, and `eexec` read after the last of them.
	if (len(prog)-1024)%513 != target || len(prog) < 1024 || bytes.Index(prog, []byte("eexec\n"))+6 < (len(prog)-1024)/513*513+513 {
		panic(fmt.Sprintf("t1Refill(%d): a %d-byte program is not the shape named", r, len(prog)))
	}
	return prog
}

// type1Refusals are the (fixture, clause) pairs nib deliberately refuses, each with the reason it must give. The
// fixtures are measured like the rest but kept out of the oracle, which would read the refusal as a regression.
var type1Refusals = map[string]string{}

func init() {
	for _, f := range []string{"REFUSED: a charstring of negative length, from a temporary file",
		"REFUSED: a charstring past 32 bits, from a temporary file"} {
		for _, c := range []string{"7.21.5 t1", "7.21.4.1 t2", "7.21.4.2 t1", "7.21.4.1 t1"} {
			type1Refusals[f+" / "+c] = "temporary-file reader"
		}
	}
	for _, c := range []string{"7.21.5 t1", "7.21.4.1 t2", "7.21.4.2 t1", "7.21.4.1 t1"} {
		type1Refusals["REFUSED: lenIV -8193, from a temporary file / "+c] = "/lenIV is"
	}
}

// type1RefusedFixtures are the shapes nib refuses on purpose, with veraPDF's measured letters beside them.
func type1RefusedFixtures() []measuredFixture {
	sub := "ABCDEF+Probe"
	w500E := "/FirstChar 65 /LastChar 67 /Widths [500 500 500] /Encoding /WinAnsiEncoding"
	full, show := "/CharSet (/A/B/C)", "(ABC) Tj"
	pad := "/Pad (" + strings.Repeat("x", 12000) + ") def\n"
	return []measuredFixture{
		{name: "REFUSED: a charstring of negative length, from a temporary file", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Edit(t1Spec{priv: pad}.build(), t1Swap("/C 9 RD ", "/C -9 RD ")), "")},
		{name: "REFUSED: a charstring past 32 bits, from a temporary file", vera: "PPPPP", pdf: t1Doc(sub, w500E, full, show, t1Edit(t1Spec{priv: pad}.build(), t1Swap("/C 9 RD ", "/C 4294967305 RD ")), "")},
		{name: "REFUSED: lenIV -8193, from a temporary file", vera: "FFPFP", pdf: t1Doc(sub, w500E, full, show, t1Spec{priv: pad, lenIV: "/lenIV -8193 def\n", iv: []byte{}}.build(), "")},
	}
}

// type1NameClauses are the clauses 7.21.7's glyph-name fallback reaches: a code the PDF's encoding does not name is
// named by the program's own encoding (`getGlyphName`), then looked up in the Adobe Glyph List.
var type1NameClauses = []string{"7.21.7 t1", "7.21.4.1 t1"}

func type1NameFixtures() []measuredFixture {
	sub := "ABCDEF+Probe"
	w := "/FirstChar 65 /LastChar 67 /Widths [500 500 500]"
	named := func(entry string) string {
		return "/Encoding 256 array\n0 1 255 {1 index exch /.notdef put} for\ndup 65 " + entry + " put\nreadonly def\n"
	}
	return []measuredFixture{
		{name: "NAME: the program names A", vera: "PP", pdf: t1Doc(sub, w, "", "(A) Tj", t1Spec{enc: named("/A")}.build(), "")},
		{name: "NAME: the program names foo", vera: "FP", pdf: t1Doc(sub, w, "", "(A) Tj", t1Spec{enc: named("/foo")}.build(), "")},
		{name: "NAME: the program's entry is a number", vera: "FP", pdf: t1Doc(sub, w, "", "(A) Tj", t1Spec{enc: named("5")}.build(), "")},
		{name: "NAME: the program's code is null", vera: "FP", pdf: t1Doc(sub, w, "", "(A) Tj", t1Spec{enc: "/Encoding /Other def\n"}.build(), "")},
		{name: "NAME: the program's StandardEncoding", vera: "PP", pdf: t1Doc(sub, w, "", "(A) Tj", t1Spec{enc: "/Encoding StandardEncoding def\n"}.build(), "")},
		{name: "NAME: the PDF's WinAnsi names it, the program foo", vera: "PP", pdf: t1Doc(sub, w+" /Encoding /WinAnsiEncoding", "", "(A) Tj", t1Spec{enc: named("/foo")}.build(), "")},
		{name: "NAME: no charstrings, the program names A", vera: "FF", pdf: t1Doc(sub, w, "", "(A) Tj", t1Spec{enc: named("/A"), noCS: true}.build(), "")},
		{name: "NAME: a parse that throws, the program names A", vera: "FF", pdf: t1Doc(sub, w, "", "(A) Tj", t1Spec{enc: named("/A"), pre: "> \n"}.build(), "")},
		{name: "NAME: text after the eexec section, with no cleartomark", vera: "PP", pdf: t1Doc(sub, w, "", "(A) Tj", t1Spec{enc: named("/A"), trailer: "\n> \n"}.build(), "")},
	}
}

// TestTheType1ClausesAgreeWithVeraPDF — every Type 1 (/FontFile) shape against veraPDF 1.30.2's verdicts, measured
// before the fixture was written (P07.S06). `X` is a document veraPDF reports nothing on, which nib must refuse on
// every clause through the one door.
func TestTheType1ClausesAgreeWithVeraPDF(t *testing.T) {
	agreeWithVeraPDF(t, type1Fixtures(), type1Clauses)
	agreeWithVeraPDF(t, type1NameFixtures(), type1NameClauses)
	agreeWithVeraPDF(t, type1RefusedFixtures(), type1Clauses)
}

func agreeWithVeraPDF(t *testing.T, fixtures []measuredFixture, clauses []string) {
	t.Helper()
	want := map[byte]Verdict{'P': Pass, 'F': Fail, '-': NotApplicable}
	for _, f := range fixtures {
		if len(f.vera) != len(clauses) {
			t.Fatalf("%s: %d verdict letters for %d clauses", f.name, len(f.vera), len(clauses))
		}
		for j, clause := range clauses {
			got := verdictOf(t, f.pdf, clause)
			if f.vera[j] == 'X' {
				if got.Verdict != CannotCheck || !strings.Contains(got.Why, "veraPDF reports nothing") {
					t.Errorf("%s: %s reports %v (%s), and veraPDF reports nothing on the document", f.name, clause, got.Verdict, got.Why)
				}
				continue
			}
			// A declared refusal must BE one, for the reason declared: an answer there is a change to review, not a pass.
			if why, declared := type1Refusals[f.name+" / "+clause]; declared {
				if got.Verdict != CannotCheck || !strings.Contains(got.Why, why) {
					t.Errorf("%s: %s reports %v (%s), and nib declares it refuses (%q)", f.name, clause, got.Verdict, got.Why, why)
				}
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

// cmType1 is the /FontFile stream pdfLaTeX (TeX Live) embeds for "Hi" in Computer Modern: a CMR10 subset of /H and /i, Flate-compressed.
const cmType1 = "" +
	"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"

// timesType1 is the same for `\\usepackage{times}`: a NimbusRomNo9L-Regu (URW) subset of /H and /i.
const timesType1 = "" +
	"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"

// TestTheType1ReaderBoundsWhatAHostileProgramCosts — veraPDF runs a Type 1 program's cleartext with no bound but its own
// 10000-iteration `for` and 64-deep user-dictionary lookup, so a program nesting loops, filling the stack or allocating
// arrays costs what its author chooses; and a private part whose subroutines have negative lengths decrypts the rest of
// the part once per subroutine. None is reachable by a veraPDF-measured fixture (veraPDF performs the work); nib refuses
// past its bounds, and each case here must be refused naming the ONE charge that fired — its counter past its bound
// (the stimulus) — and having done no more than twice that bound's work (the response, as a work count: a clock
// measures the machine, not the reader).
func TestTheType1ReaderBoundsWhatAHostileProgramCosts(t *testing.T) {
	// The slot bound leaves room for what fonts do: 31 arrays of veraPDF's largest size parse, a 32nd is refused below,
	// and both pdfLaTeX programs parse.
	if p := readType1(t1Spec{pre: "0 1 30 {pop 65536 array pop} for\n"}.build(), nil); p.state != ttParsed {
		t.Errorf("31 arrays of 65536: state %v (%s), want parsed", p.state, p.why)
	}
	for _, b64 := range []string{cmType1, timesType1} {
		if _, dec := realT1(b64); readType1(dec, nil).state != ttParsed {
			t.Errorf("a pdfLaTeX program does not parse under the bounds")
		}
	}
	ops := func(s *t1Spend) (int, int) { return s.ops, t1MaxOps }
	alloc := func(s *t1Spend) (int, int) { return s.alloc, t1MaxAlloc }
	decrypted := func(s *t1Spend) (int, int) { return s.decrypted, t1MaxDecrypt }
	negSubrs := "/Subrs 20000 array\n" + strings.Repeat("dup 0 -1 RD NP\n", 20000) + "ND\n"
	for _, c := range []struct {
		name   string
		prog   []byte
		want   string
		charge func(*t1Spend) (int, int) // the counter that must be past its bound; nil for a bound on one program's state
	}{
		{"nested loops", t1Spec{pre: "0 1 9999 {pop 0 1 9999 {pop} for} for\n"}.build(), "PostScript objects", ops},
		{"a filled stack", t1Spec{pre: strings.Repeat("0 1 9999 {} for\n", 110)}.build(), "operand stack", nil},
		{"arrays allocated in a loop", t1Spec{pre: "0 1 9999 {pop 65536 array pop} for\n"}.build(), "array slots", alloc},
		{"arrays nested past the bound", t1Spec{pre: strings.Repeat("[", 300) + "\n"}.build(), "nests arrays", nil},
		{"subroutines of negative length", t1Spec{priv: negSubrs}.build(), "decrypt to more than", decrypted},
		// Each O(n) stack operator is charged its n: measured unbounded, 200 rolls of a deep stack took 10 s and 30 GB.
		// A roll charges 2n objects and n slots, and the objects' bound is twice the slots', so the objects' (checked
		// first, and already carrying the loop that built the stack) is the one that fires.
		{"rolls of a deep stack", t1Spec{pre: "0 1 9999 {} for\n" + strings.Repeat("10000 1 roll\n", 300)}.build(), "PostScript objects", ops},
		{"counttomark over a deep stack", t1Spec{pre: "0 1 9999 {} for\n" + strings.Repeat("counttomark pop\n", 500)}.build(), "PostScript objects", ops},
		// A copy charges its n slots, and each rebuild of the stack ~20,000 objects: the slots cross first.
		{"copies of a deep stack", t1Spec{pre: "0 1 9999 {} for\n" + strings.Repeat("10000 copy clear 0 1 9999 {} for\n", 400)}.build(), "array slots", alloc},
		// A dictionary access costs its key's length (measured unbounded: 63 s for a 2 MiB key looked up 10^8 times).
		{"lookups of a long key", t1Spec{pre: "/K" + strings.Repeat("k", 2<<20) + " 1 def 0 1 9999 {pop 0 1 9999 {pop /K" + strings.Repeat("k", 2<<20) + " load pop} for} for\n"}.build(), "PostScript objects", ops},
		{"arrays one past the slot bound", t1Spec{pre: "0 1 31 {pop 65536 array pop} for\n"}.build(), "array slots", alloc},
	} {
		spend := &t1Spend{}
		p := readType1(c.prog, spend)
		if p.state != ttUnknown || !strings.Contains(p.why, c.want) {
			t.Errorf("%s: state %v (%s), want a refusal naming %q", c.name, p.state, p.why, c.want)
		}
		if c.charge == nil {
			continue
		}
		if n, bound := c.charge(spend); n <= bound {
			t.Errorf("%s: the charge named spent %d, not past its bound %d — some other bound refused it", c.name, n, bound)
		}
		for _, k := range []func(*t1Spend) (int, int){ops, alloc, decrypted} {
			if n, bound := k(spend); n > 2*bound {
				t.Errorf("%s: a counter reached %d, past twice its bound %d, before the refusal", c.name, n, bound)
			}
		}
	}
}

// TestTheType1BudgetIsTheDocuments — the Type 1 bounds are the DOCUMENT's, as the TrueType budget is
// (`TestTheReadBudgetIsTheDocuments`): two different programs, each well under the objects' bound alone, together over
// it — the first is read and the second refused naming the document's Type 1 programs. As a program's bound, a
// document of N programs each just under it cost N times it (66 ms each, measured at the P07 phase close).
func TestTheType1BudgetIsTheDocuments(t *testing.T) {
	heavy := func(tag string) []byte {
		return t1Spec{pre: "/" + tag + " 0 def 0 1 119 {pop 0 1 9999 {pop} for} for\n"}.build()
	}
	a, b := heavy("a"), heavy("b")
	spend := &t1Spend{}
	if p := readType1(a, spend); p.state != ttParsed || spend.ops < t1MaxOps/2 || spend.ops > t1MaxOps*3/4 {
		t.Fatalf("one heavy program: %v (%s) after %d objects, want parsed between half and three quarters of the bound", p.state, p.why, spend.ops)
	}
	if p := readType1(b, nil); p.state != ttParsed {
		t.Fatalf("the second program alone: %v (%s), want parsed (the control)", p.state, p.why)
	}
	doc := func(progA, progB []byte) []byte {
		objs := map[int]string{}
		for i, prog := range [][]byte{progA, progB} {
			l1 := bytes.Index(prog, []byte("eexec")) + 6
			objs[12+i] = fmt.Sprintf("<< /Type /FontDescriptor /FontName /P%d /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle 0 "+
				"/Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile %d 0 R >>", i, 20+i)
			objs[20+i] = spStream(fmt.Sprintf("/Length1 %d /Length2 %d /Length3 0", l1, len(prog)-l1), string(prog))
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
		if sp.kind != "Type 1" {
			t.Fatalf("font %d: program kind %q, want Type 1", n, sp.kind)
		}
		known, whys = append(known, ok), append(whys, why)
	}
	if len(d.type1Reads) != 2 {
		t.Fatalf("%d programs read, want both", len(d.type1Reads))
	}
	if !known[0] || known[1] || !strings.Contains(whys[1], "the document's Type 1 programs") {
		t.Fatalf("two heavy programs in one document: known %v (%q), want the first read and the second refused naming the document's Type 1 programs", known, whys)
	}
}

// TestASimpleType1FontsProgramHasOneDoor — ADR-009: the choice between a /FontFile Type 1 program and a /FontFile3
// Type1C one is made once, in `simpleProgramOf`, and neither reader is called anywhere else — a site asking one reader
// alone would answer a font of the other kind as having no program.
func TestASimpleType1FontsProgramHasOneDoor(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, reader := range []string{"d.type1Of(", "d.type1CProgramOf("} {
		calls := 0
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			n := strings.Count(string(b), reader)
			if n > 0 && f != "type1c.go" {
				t.Errorf("%s calls %s; only simpleProgramOf may", f, reader)
			}
			calls += n
		}
		if calls != 1 {
			t.Errorf("%s is called %d times, want once (from simpleProgramOf)", reader, calls)
		}
	}
}
