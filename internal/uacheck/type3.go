package uacheck

import (
	"fmt"

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
				if d.decodeFontStream(sd, "the glyph procedure") != "" {
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
// (asked per FONT, since the program is read when the font object is built) or reading one glyph's width; P07.S05b the
// same for a CIDFontType0C program (`cidFontThrows`, and a full font's width per glyph), and a CID-keyed program under a
// simple font — an unnamed code drawn, or a subset name with a /CharSet (`cidUnderSimpleFontThrows`). P07.S03's two
// TrueType throws — a table past the end of a streamed program, a negative /Differences code — joined at /pending 682.
//
// A document whose CFF or Type 1 programs spent a document budget is refused here too, since the programs refused past
// it were never asked whether veraPDF throws on them (RR1-6).
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
				if _, _, _, throws := d.simpleProgramOf(f.dict); throws != "" {
					return fmt.Sprintf("veraPDF reports nothing on this document — %s, font %s: %s", f.where, fontLabel(f.name), throws)
				}
				if throws := d.cidUnderSimpleFontThrows(f.dict); throws != "" {
					return fmt.Sprintf("veraPDF reports nothing on this document — %s, font %s: %s", f.where, fontLabel(f.name), throws)
				}
			}
			if f.dict != nil && d.name(f.dict["Subtype"]) == "Type0" {
				if throws := d.cidFontThrows(f.dict); throws != "" {
					return fmt.Sprintf("veraPDF reports nothing on this document — %s, font %s: %s", f.where, fontLabel(f.name), throws)
				}
			}
		}
	}
	// The two TrueType throws P07.S03 measured (`/pending 682`): they refused only the clauses that read the program,
	// and the other hundred answered a document veraPDF gives no report for. A read that could not finish is not an
	// answer here — the clauses that read TrueType fonts refuse on their own, as for the walk above.
	if tts, why := d.trueTypeFonts(); why == "" {
		for _, f := range tts {
			if f.nothing != "" {
				return fmt.Sprintf("veraPDF reports nothing on this document — %s, font %s: %s", f.where, fontLabel(f.name), f.nothing)
			}
		}
	}
	glyphs, _ := d.glyphsDrawn() // unbuilt: whatever was built is still asked
	for _, g := range glyphs {
		if w := d.throwsBuilding(g); w != "" {
			return fmt.Sprintf("veraPDF reports nothing on this document — %s, %s: %s", g.where, glyphFontLabel(g), w)
		}
	}
	// A parent-tree lookup veraPDF makes that meets a loop throws, and veraPDF reports nothing (RR3-2).
	if why := d.parentTreeNothing(); why != "" {
		return why
	}
	// **A spent document budget is itself the refusal** (the P07 phase-close re-review, RR1-6). Past one, the program
	// being read and every later read charging that counter are refused UNREAD, so a throw veraPDF meets in one of them
	// is invisible to the loops above — font A's cost hid font B's throw, and the other clauses answered a document
	// veraPDF reports nothing on. Asked last, so every read above has charged what it will.
	if why := d.cffSpent.exhausted(); why != "" {
		return "nib stopped reading this document's font programs at its budget, so whether veraPDF reports nothing on " +
			"it — a program it throws reading — is not known: " + why
	}
	if why := d.type1Spent.exhausted(); why != "" {
		return "nib stopped reading this document's font programs at its budget, so whether veraPDF reports nothing on " +
			"it — a program it throws reading — is not known: " + why
	}
	return ""
}

// throwsBuilding is whether veraPDF throws building this one glyph.
func (d *Document) throwsBuilding(g glyph) string {
	if w := d.throwsReadingFont(g); w != "" {
		return w
	}
	return d.widthsThrow(g)
}

// firstLastThrows and wOpenerThrows are the two width reads veraPDF throws on (the P07 phase-close review, R2-4 —
// measured on veraPDF 1.30.2): `getIntegerKey` is null for a value that is neither a number nor a STRING, and
// `intValue()` on it throws — a name, a boolean or an array as /FirstChar or /LastChar (either one, whatever the code),
// or as a /W entry's opening CID. A string is read as some number and reports; nib refuses that clause by clause.
const (
	firstLastThrows = "its /FirstChar or /LastChar is neither a number nor a string, where veraPDF throws an exception it " +
		"does not handle"
	wOpenerThrows = "its /W array opens an entry with something that is neither a number nor a string, where veraPDF " +
		"throws an exception it does not handle"
)

// intKeyNull is whether veraPDF's `getIntegerKey` reads o as null: it is neither a number nor a string.
func (d *Document) intKeyNull(o types.Object) bool {
	switch d.resolve(o).(type) {
	case types.Integer, types.Float, types.StringLiteral, types.HexLiteral:
		return false
	}
	return true
}

// widthsThrow is where building a glyph reads a width veraPDF throws on. **It asks the metric door itself**, because
// the throw comes only where veraPDF reads the dictionary width — a glyph drawn (measured: `() Tj` reports), in a font
// whose program it parsed (measured: the same TrueType font with no program reports) — and `metricsOf` is where nib
// reads it in exactly those cases. A cheap test of the dictionary goes first, so an ordinary font is not read twice.
//
// Neither shape reaches the door through a file today: pdfcpu's validator refuses a /FirstChar or /LastChar that is not
// an integer (the document does not open) and drops a CIDFont whose /W holds one (the font does not resolve, and the
// glyph clauses refuse on that). The door is written for the day either reader stops — measured, not reasoned.
func (d *Document) widthsThrow(g glyph) string {
	if g.unread != "" {
		return ""
	}
	font := g.font.dict
	switch d.name(font["Subtype"]) {
	case "TrueType", "Type1", "MMType1", "Type3":
		_, hasW := font["Widths"]
		_, hasF := font["FirstChar"]
		_, hasL := font["LastChar"]
		if !hasW || !hasF || !hasL || (!d.intKeyNull(font["FirstChar"]) && !d.intKeyNull(font["LastChar"])) {
			return ""
		}
		d.trueTypeFonts() // the TrueType metric path reads its font through that population
		if m := d.metricsOf(g); m.why == firstLastThrows {
			return m.why
		}
	case "Type0":
		if c, known, _ := d.cidFontOf(font); known && c != nil && c.w != nil && !c.w.ok && c.w.why == wOpenerThrows {
			return c.w.why
		}
	}
	return ""
}

// throwsReadingFont is whether veraPDF throws reading this glyph's font for it: its program, its /CharProcs, its CMap.
func (d *Document) throwsReadingFont(g glyph) string {
	font := g.font.dict
	switch d.name(font["Subtype"]) {
	case "Type1", "MMType1":
		return d.type1CThrowsFor(g)
	case "Type3":
		_, why := d.charProcs(font)
		return why
	case "Type0":
		if chain, why := d.chainOf(g.font); why == "" {
			if loops := cmapChainLoops(chain); loops != "" {
				return loops
			}
		}
		if g.unread != "" {
			return ""
		}
		if c, known, _ := d.cidFontOf(font); known && c != nil {
			cid, why := d.toCID(c, g.code)
			if why != "" {
				if cid, asked := c.cids[g.code]; asked && cid < 0 {
					return why
				}
				return ""
			}
			// A full CFF font's width is read when the glyph is built, and a charstring INDEX running backwards throws.
			if c.cff != nil {
				if _, err := c.cffWidth(cid); err != nil && err.kind == errThrow {
					return err.why
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
