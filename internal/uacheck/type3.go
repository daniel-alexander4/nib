package uacheck

import (
	"fmt"
	"math"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A Type 3 font's glyphs, as `GFGlyph.initForType3` reads them — `PLAN-ua-coverage.md` P07.S04b, every rule below
// measured on veraPDF 1.30.2 before it was written:
//
//   - the font is always judged: a Type 3 font is "parsed" by construction, and both widths are always filled;
//   - presence is `PDType3Font.containsCharString`: the encoding's NAME for the code keyed into /CharProcs, with **no
//     code-0 exemption** (the TrueType path's "every font holds .notdef" is not asked here — an unnamed code 0 fails);
//   - the program width is the glyph procedure's `d0`/`d1` width (`type3Width`), cast to FLOAT, and a float -1 —
//     a width it could not read, or a procedure that really says `-1 0 d0` — is the descriptor's /MissingWidth, else 0;
//   - the dictionary width is `simpleDictWidth`, the same `PDFont.getWidth` every simple font reads.
//
// No FontMatrix scaling: /Widths and `d0` are both in glyph space, and veraPDF compares them raw (measured at 0.01).

// type3Metrics is metricsOf for a Type 3 font.
func (d *Document) type3Metrics(g glyph) glyphMetrics {
	font := g.font.dict
	procs, why := d.charProcs(font)
	if why != "" {
		return glyphMetrics{why: why}
	}
	dw, ok, why := d.simpleDictWidth(font, g.code)
	if !ok {
		return glyphMetrics{why: why}
	}
	if g.font.enc == nil {
		e := d.encodingOf(font)
		g.font.enc = &e
	}
	name, named := g.font.enc.name(g.code)
	var proc types.Object
	present := false
	if named && procs != nil {
		// A direct `null` value is absent in both readers: veraPDF's `COSDictionary.setKey` removes it, and pdfcpu
		// drops it at parse (measured; the red-proof found a separate nil test here dead).
		proc, present = procs[name]
	}
	width := -1.0
	if present {
		if sd, _, err := d.Ctx.DereferenceStreamDict(proc); err == nil && sd != nil {
			r, read := d.t3Widths[dictID(sd.Dict)]
			if !read {
				// Once per procedure, a failed decode included: one token may be the whole stream, and many codes and
				// fonts may name one procedure.
				if sd.Content == nil && sd.Decode() != nil {
					r.undecodable = true
				} else {
					r.w = type3Width(sd.Content)
				}
				if d.t3Widths == nil {
					d.t3Widths = map[uintptr]t3Width{}
				}
				d.t3Widths[dictID(sd.Dict)] = r
			}
			if r.undecodable {
				return glyphMetrics{why: "the glyph procedure for /" + name + " could not be decoded"}
			}
			width = r.w
		}
	}
	if float32(width) == -1 {
		width = d.missingWidth(font)
	} else {
		width = float64(float32(width))
	}
	return glyphMetrics{known: true, valid: true, present: present, program: width, dictionary: dw}
}

// t3Width is one glyph procedure's reading: its width, or that it could not be decoded.
type t3Width struct {
	w           float64
	undecodable bool
}

// charProcs is `PDType3Font.getCharProcs`: the /CharProcs dictionary, nil when there is none. **Anything else is a
// document veraPDF reports nothing on** (measured): its `(COSDictionary)` cast throws a ClassCastException nothing on
// the glyph path catches, for every glyph, visible or not.
func (d *Document) charProcs(font types.Dict) (types.Dict, string) {
	raw := font["CharProcs"]
	switch v := d.resolve(raw).(type) {
	case nil:
		// Absent is no /CharProcs, and so is a reference to an object the file does not hold — `getDirectBase` is null
		// and the cast passes (measured: veraPDF reports, reading it absent). A reference to a stored `null` is a
		// COSNull the cast throws on (measured: no report).
		if ir, indirect := raw.(types.IndirectRef); indirect {
			if e, held := d.Ctx.XRefTable.Table[ir.ObjectNumber.Value()]; held && e != nil && !e.Free {
				return nil, "its /CharProcs is an indirect reference to null, where veraPDF throws an exception it does not " +
					"handle and reports nothing on the document"
			}
		}
		return nil, ""
	case types.Dict:
		return v, ""
	case types.StreamDict:
		return v.Dict, "" // COSStream is a COSDictionary: the cast succeeds and its own entries are read (measured)
	}
	return nil, "its /CharProcs is not a dictionary, where veraPDF throws an exception it does not handle and reports " +
		"nothing on the document"
}

// missingWidth is `PDFontDescriptor.getMissingWidth`: /MissingWidth when it is a number, else 0 (`DEFAULT_WIDTH`) — and
// 0 too with no descriptor, which veraPDF builds from an empty dictionary.
func (d *Document) missingWidth(font types.Dict) float64 {
	if desc := d.dict(font["FontDescriptor"]); desc != nil {
		if v, ok := d.real(desc["MissingWidth"]); ok {
			return v
		}
	}
	return 0
}

// reportsNothing is the ONE door for a document veraPDF reports nothing on because building a glyph throws an
// exception its validator does not handle — measured: the job ends with no validation report at all, so no clause of
// the document can be compared, and `Check` refuses them all. veraPDF builds a glyph for invisible text too, so every
// glyph is asked, and the first answer is kept. Two throws are known and measured here: a Type 3 font whose /CharProcs
// is not a dictionary, and a code its CMap maps to a negative CID; P07.S05a adds a Type1C program veraPDF throws reading
// (asked per FONT, since the program is read when the font object is built) or reading one glyph's width. (P07.S03's two TrueType throws still refuse only
// the font clauses that read them — /pending 682.)
//
// **What the door cannot see, declared:** a content walk that fails (no font and no glyph is asked — every clause that
// reads them refuses on its own), a glyph population nib cannot build, a code whose CMap lookup ran past the
// budget, and a reader that panics are not answers — the walk carries on past each, so a LATER throw is still found,
// but where the only throw sits behind one of them the glyph clauses refuse on their own and the rest answer.
func (d *Document) reportsNothing() (why string) {
	if d.nothingDone {
		return d.nothing
	}
	d.nothingDone = true
	defer func() {
		if recover() != nil {
			why = "" // a reader that panics is the clauses' to report, one at a time (`runOne`)
		}
		d.nothing = why
	}()
	// A font's program is read when veraPDF builds the FONT object, whether or not a glyph is drawn (measured: a Type1C
	// program whose predefined charset overflows, shown by `() Tj`, reports nothing).
	if fonts, why := d.usedFonts(); why == "" {
		for _, f := range fonts {
			if st := d.name(f.dict["Subtype"]); f.dict != nil && (st == "Type1" || st == "MMType1") {
				if _, _, _, throws := d.type1COf(f.dict); throws != "" {
					return fmt.Sprintf("veraPDF reports nothing on this document — %s, font %s: %s", f.where, fontLabel(f.name), throws)
				}
			}
		}
	}
	glyphs, _ := d.glyphsDrawn() // unbuilt: whatever was built is still asked
	for _, g := range glyphs {
		if w := d.throwsBuilding(g); w != "" {
			return fmt.Sprintf("veraPDF reports nothing on this document — %s, %s: %s", g.where, glyphFontLabel(g), w)
		}
	}
	return ""
}

// throwsBuilding is whether veraPDF throws building this one glyph.
func (d *Document) throwsBuilding(g glyph) string {
	font := g.font.dict
	switch d.name(font["Subtype"]) {
	case "Type1", "MMType1":
		return d.type1CThrowsFor(g)
	case "Type3":
		_, why := d.charProcs(font)
		return why
	case "Type0":
		if chain, why := d.cmapChain(font, ""); why == "" {
			if loops := cmapChainLoops(chain); loops != "" {
				return loops
			}
		}
		if g.unread != "" {
			return ""
		}
		if c, known, _ := d.cidTrueTypeOf(font); known && c != nil {
			if _, why := d.toCID(c, g.code); why != "" {
				if cid, asked := c.cids[g.code]; asked && cid < 0 {
					return why
				}
			}
		}
	}
	return ""
}

// type3Width is `Type3CharProcParser.parse` over a glyph procedure: the first token's value when it is a number and
// either the third token is the keyword `d0` or the seventh is `d1`; otherwise -1 (a failed parse throws, and
// `getWidthFromProgram` answers -1 for it). The tokens are veraPDF's `BaseParser`'s, which split differently from a
// content-stream reader: `0d0` is a number then a keyword, `1e1` is `1` then `e1`, `[1 2]` is four tokens.
func type3Width(b []byte) float64 {
	p := bpTokens{b: b}
	var toks [7]bpToken
	for i := range toks {
		t := p.next()
		if t.kind == bpThrow {
			return -1
		}
		toks[i] = t
		if i == 2 && t.text == "d0" { // only a keyword carries text
			break
		}
		if i == 6 && t.text != "d1" {
			return -1
		}
	}
	if k := toks[0].kind; k == bpInteger || k == bpReal {
		return toks[0].value
	}
	return -1
}

type bpKind int

const (
	bpOther bpKind = iota
	bpKeyword
	bpInteger
	bpReal
	bpEOF
	bpThrow // `nextToken` threw an IOException
)

type bpToken struct {
	kind  bpKind
	text  string
	value float64
}

// bpTokens is veraPDF-parser's `BaseParser.nextToken`, as far as a glyph procedure's head needs it: which bytes make
// one token, and a number's value.
type bpTokens struct {
	b []byte
	i int
}

// bpSpace and bpDelimiter are `CharTable`'s SPACE and DELIMITER classes.
func bpSpace(c byte) bool { return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32 }

func bpDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return bpSpace(c)
}

func (p *bpTokens) next() bpToken {
	// `skipSpaces(true)`: whitespace, and comments to the end of the line.
	for p.i < len(p.b) {
		switch c := p.b[p.i]; {
		case bpSpace(c):
			p.i++
		case c == '%':
			for p.i < len(p.b) && p.b[p.i] != '\n' && p.b[p.i] != '\r' {
				p.i++
			}
		default:
			goto token
		}
	}
	return bpToken{kind: bpEOF}
token:
	c := p.b[p.i]
	p.i++
	switch c {
	case '(':
		depth := 0
		for p.i < len(p.b) {
			ch := p.b[p.i]
			p.i++
			switch {
			case ch == '\\':
				p.i++
			case ch == '(':
				depth++
			case ch == ')':
				if depth == 0 {
					return bpToken{}
				}
				depth--
			}
		}
		return bpToken{}
	case '<':
		if p.i < len(p.b) && p.b[p.i] == '<' {
			p.i++
			return bpToken{}
		}
		if p.i < len(p.b) && p.b[p.i] == '~' {
			// `readASCII85`: everything up to `~>` (or the end), which a bad payload only LOGS about (measured: an
			// ASCII85 string as the first token and as the second both answer as veraPDF does).
			for p.i++; p.i < len(p.b) && !(p.b[p.i] == '~' && p.i+1 < len(p.b) && p.b[p.i+1] == '>'); p.i++ {
			}
			p.i += 2
			return bpToken{}
		}
		for p.i < len(p.b) {
			p.i++
			if p.b[p.i-1] == '>' {
				break
			}
		}
		return bpToken{}
	case '>':
		if p.i < len(p.b) && p.b[p.i] == '>' {
			p.i++
			return bpToken{}
		}
		p.i++ // `readByte` consumed it before the throw
		return bpToken{kind: bpThrow}
	case ')', '[', ']', '{', '}':
		return bpToken{}
	case '/':
		for p.i < len(p.b) && !bpDelimiter(p.b[p.i]) {
			p.i++
		}
		return bpToken{}
	case '+':
		return p.number(false)
	case '-':
		return p.number(true)
	}
	if c >= '0' && c <= '9' || c == '.' {
		p.i--
		return p.number(false)
	}
	start := p.i - 1
	for p.i < len(p.b) && !bpDelimiter(p.b[p.i]) {
		p.i++
	}
	return bpToken{kind: bpKeyword, text: string(p.b[start:p.i])}
}

// number is `readNumber`: digits and dots up to the first other byte (which starts the next token), an INTEGER unless a
// dot was seen, and Double.MAX_VALUE where Java's parse throws (no digits, two dots, a long overflowing) — negated for
// a leading `-`.
func (p *bpTokens) number(neg bool) bpToken {
	start, real := p.i, false
	for p.i < len(p.b) {
		c := p.b[p.i]
		if c == '.' {
			real = true
		} else if c < '0' || c > '9' {
			break
		}
		p.i++
	}
	s := string(p.b[start:p.i])
	t := bpToken{kind: bpInteger}
	var err error
	if real {
		t.kind = bpReal
		// Java's parseDouble answers Infinity past the range rather than throwing; Go reports it as an error.
		t.value, err = strconv.ParseFloat(s, 64)
		if math.IsInf(t.value, 0) {
			err = nil
		}
	} else {
		var n int64
		n, err = strconv.ParseInt(s, 10, 64)
		t.value = float64(n)
	}
	if err != nil {
		t.value = math.MaxFloat64
	}
	if neg {
		t.value = -t.value
	}
	return t
}
