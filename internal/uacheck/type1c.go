package uacheck

import (
	"fmt"
	"unicode/utf8"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/fontcode"
)

// A simple Type 1 font's CFF program (/FontFile3 /Subtype /Type1C) as `PDType1Font` opens it — `PLAN-ua-coverage.md`
// P07.S05a. `PDType1Font.getFontProgram` opens /FontFile before /FontFile3, a /FontFile3 only of subtype Type1C or
// OpenType (any other is "Invalid subtype" and no program), keys its cache by the stream AND the font's subset-ness
// (`FontProgramIDGenerator.getCFFFontProgramID`), and swallows any exception CONSTRUCTING it as "no program" — the
// parse runs later, in `GFPDType1Font`, where only an IOException is caught.

// type1CRead is one program's reading for one subset-ness, kept: several fonts may share a stream.
type type1CRead struct {
	c     *cffProgram
	known bool
	why   string
}

type type1CKey struct {
	stream uintptr
	subset bool
}

// simpleProgram is the program a simple Type 1 font's glyphs are read through: its /FontFile Type 1 program (P07.S06)
// or its /FontFile3 Type1C one (P07.S05a) — at most one is set, and neither where veraPDF has no parsed program.
type simpleProgram struct {
	kind string // "Type 1" or "CFF": which one `PDType1Font.getFontProgram` opens, "" for none
	t1   *type1Program
	cff  *cffProgram
}

// parsed is whether veraPDF holds a parsed program — `containsFontFile`.
func (s simpleProgram) parsed() bool { return s.t1 != nil || s.cff != nil }

// glyphName is the program's `getGlyphName(code)`, false for null (a CID-keyed CFF program's is always null).
func (s simpleProgram) glyphName(code int) (string, bool) {
	switch {
	case s.t1 != nil:
		return s.t1.glyphName(code)
	case s.cff != nil && !s.cff.cid:
		return s.cff.glyphName(code), true
	}
	return "", false
}

// charSet is the program's `getCharSet()` names — nil where there is no parsed program or it is CID-keyed.
func (s simpleProgram) charSet() map[string]int {
	switch {
	case s.t1 != nil:
		out := make(map[string]int, len(s.t1.widths))
		for n := range s.t1.widths {
			out[n] = 0
		}
		return out
	case s.cff != nil && !s.cff.cid:
		return s.cff.charSet
	}
	return nil
}

// simpleProgramOf is the ONE door for a simple Type 1 font's embedded program (ADR-009): /FontFile before /FontFile3,
// as `embeddedProgram` says, each read by its own reader — `type1Of`, `type1CProgramOf` — which nothing else calls.
// `known` is false where nib refuses (with `why`), `throws` is set where veraPDF throws reading it.
func (d *Document) simpleProgramOf(font types.Dict) (s simpleProgram, known bool, why, throws string) {
	s.kind = d.embeddedProgram(font)
	switch s.kind {
	case "Type 1":
		s.t1, known, why, throws = d.type1Of(font)
	case "CFF":
		s.cff, known, why, throws = d.type1CProgramOf(font)
	default:
		known = true
	}
	return s, known, why, throws
}

// type1CProgramOf is a simple Type 1 font's Type1C program (read only through `simpleProgramOf`): nil and known where
// veraPDF has no parsed program (none, one of another subtype, or one its parse failed — the metrics then stay null),
// not known where nib does not read what veraPDF would. `throws` is set where veraPDF throws reading it, so that it
// reports nothing on the document.
func (d *Document) type1CProgramOf(font types.Dict) (c *cffProgram, known bool, why, throws string) {
	desc := d.dict(font["FontDescriptor"])
	sd, _, err := d.Ctx.DereferenceStreamDict(desc["FontFile3"])
	if err != nil || sd == nil {
		return nil, true, "", ""
	}
	switch d.name(sd.Dict["Subtype"]) {
	case "Type1C":
	case "OpenType":
		// The SAME door `fontEmbedded` asks (the P07 phase-close re-review, RR1-5: this refused unread where 7.21.4.1 t1
		// answered): with no "CFF " table veraPDF has no program, and its metrics stay null — measured, the metric and
		// glyph clauses pass junk, a two-byte program and a TrueType one, as they pass a font with no program at all.
		if st, why := d.openTypeProgram(desc); st != ttFailed {
			return nil, false, "its embedded program is OpenType, which nib does not read under a Type 1 font — " + why, ""
		}
		return nil, true, "", ""
	default:
		return nil, true, "", "" // "Invalid subtype of the embedded font stream": no program
	}
	base, isName := d.nameOf(font["BaseFont"])
	if !isName {
		return nil, true, "", "" // `getName()` is null and `isSubset` throws inside the catch: no program
	}
	if !utf8.ValidString(base) {
		return nil, false, "its /BaseFont is not valid UTF-8, where Java's decoder and nib's count its characters differently", ""
	}
	subset, ok := subsetFont(base)
	if !ok {
		return nil, true, "", ""
	}
	return d.cffRead(sd, subset)
}

// cffRead is the ONE read of a /FontFile3 CFF stream for one subset-ness — a simple Type 1 font's Type1C and a CIDFont's
// CIDFontType0C alike — kept, since several fonts may share a stream. veraPDF's cache adds the Type 0 font's CMap to the
// key; the parse reads no CMap, so one reading serves both.
func (d *Document) cffRead(sd *types.StreamDict, subset bool) (c *cffProgram, known bool, why, throws string) {
	key := type1CKey{dictID(sd.Dict), subset}
	if r, done := d.type1CReads[key]; done {
		return r.c, r.known, r.why, d.type1CThrows[key]
	}
	r := type1CRead{known: true}
	throws = ""
	if sd.Content == nil && sd.Decode() != nil {
		r = type1CRead{why: "its embedded CFF program could not be decoded"}
	} else {
		prog := readCFF(sd.Content, subset, &d.cffSpent)
		switch {
		case prog.throws != "":
			throws = prog.throws
			r = type1CRead{why: prog.throws}
		case prog.state == ttParsed:
			r.c = &prog
		case prog.state == ttUnknown:
			r = type1CRead{why: prog.why}
		}
	}
	if d.type1CReads == nil {
		d.type1CReads = map[type1CKey]type1CRead{}
		d.type1CThrows = map[type1CKey]string{}
	}
	d.type1CReads[key] = r
	d.type1CThrows[key] = throws
	return r.c, r.known, r.why, throws
}

// type1CMetrics is `GFGlyph` over a Type1C font (`PDType1Font.glyphIsPresent` / `getWidthFromProgram`): the PDF's own
// encoding names the glyph where it can — present if the program's charset holds the name, the width that name's glyph —
// and where it names nothing the PROGRAM's built-in encoding decides (`containsCode`, `getWidth(int)`).
//
// A CID-keyed program answers a NAME with no glyph and width 0 (`containsGlyph`, `getWidth(String)`), and a code its PDF
// encoding does not name — through a CMap a simple font does not have — with an exception veraPDF does not handle
// (measured: WinAnsi-named codes are judged, a font with no /Encoding reports nothing).
func (d *Document) type1CMetrics(g glyph, c *cffProgram) (m glyphMetrics, throws string) {
	font := g.font.dict
	if g.font.enc == nil {
		e := d.encodingOf(font)
		g.font.enc = &e
	}
	name, named := g.font.enc.name(g.code)
	var present bool
	var w float32
	var err *cffErr
	switch {
	case c.cid && !named:
		return glyphMetrics{why: cidUnderSimpleFont}, cidUnderSimpleFont
	case c.cid:
		// present stays false, w 0
	case named:
		_, present = c.charSet[name]
		w, err = c.widthOfName(name)
	default:
		present = c.containsCode(g.code)
		w, err = c.widthOfCode(g.code)
	}
	if err != nil {
		if err.kind == errThrow {
			return glyphMetrics{why: err.why}, err.why
		}
		return glyphMetrics{why: err.why}, ""
	}
	dw, ok, why := d.simpleDictWidth(font, g.code)
	if !ok {
		return glyphMetrics{why: why}, ""
	}
	program := float64(w)
	if w == -1 { // `GFGlyph`: a program width of -1 is the descriptor's /MissingWidth, else 0
		program = d.missingWidth(font)
	}
	return glyphMetrics{known: true, valid: true, present: g.code == 0 || present, program: program, dictionary: dw}, ""
}

// cidUnderSimpleFont is a CID-keyed CFF program under a simple Type 1 font: veraPDF parses it (so the font is embedded),
// then looks a glyph its PDF encoding does not name up through the CMap a simple font does not have, and compares a
// /CharSet against it by a cast to the Type1-keyed program — both exceptions it does not handle (measured: an unnamed
// code drawn, or a subset-named font with a /CharSet, and it reports nothing on the document).
const cidUnderSimpleFont = "its CFF program is CID-keyed, which veraPDF parses and then reads through a CMap a simple " +
	"font does not have, throwing an exception it does not handle"

// cidUnderSimpleFontThrows is whether veraPDF throws judging 7.21.4.2 t1 on this simple font, whatever is drawn: a parsed
// CID-keyed program, a subset name, and a /CharSet string — exactly the conditions under which the test reaches the cast.
func (d *Document) cidUnderSimpleFontThrows(font types.Dict) string {
	sp, known, _, throws := d.simpleProgramOf(font)
	c := sp.cff
	if throws != "" || !known || c == nil || !c.cid {
		return ""
	}
	base, _ := d.nameOf(font["BaseFont"])
	if _, has := d.text(d.dict(font["FontDescriptor"])["CharSet"]); !subsetNamed(base) || !has {
		return ""
	}
	return cidUnderSimpleFont
}

// cidCFFOf is a CIDFont's CIDFontType0C program (`PDCIDFont.getFontProgram`): subset-ness is the DESCENDANT's /BaseFont
// (the CIDFont is the dictionary `PDType0Font` is built over), and anything failing before the parse — no name, a name
// `isSubset` throws on — is no program, the exception caught there.
func (d *Document) cidCFFOf(cid types.Dict) (c *cffProgram, known bool, why, throws string) {
	sd, _, err := d.Ctx.DereferenceStreamDict(d.dict(cid["FontDescriptor"])["FontFile3"])
	if err != nil || sd == nil {
		return nil, true, "", ""
	}
	base, isName := d.nameOf(cid["BaseFont"])
	if !isName {
		return nil, true, "", ""
	}
	if !utf8.ValidString(base) {
		return nil, false, "its CIDFont's /BaseFont is not valid UTF-8, where Java's decoder and nib's count its characters differently", ""
	}
	subset, ok := subsetFont(base)
	if !ok {
		return nil, true, "", ""
	}
	return d.cffRead(sd, subset)
}

// type1CThrowsFor is the door's question for one glyph: does veraPDF throw reading this glyph's width (a full font's
// widths are read when a glyph asks, so a charstring whose INDEX runs backwards throws only then), or reading the
// program at all?
func (d *Document) type1CThrowsFor(g glyph) string {
	sp, known, _, throws := d.simpleProgramOf(g.font.dict)
	c := sp.cff
	if throws != "" {
		return throws
	}
	if !known || c == nil || g.unread != "" {
		return ""
	}
	_, throws = d.type1CMetrics(g, c)
	return throws
}

func init() {
	register(Rule{Clause: "7.21.4.2 t1", Summary: "an embedded Type 1 font's CharSet shall list every glyph its program holds", Check: checkCharSets})
}

// checkCharSets evaluates ua1 7.21.4.2 t1 — veraPDF's `PDType1Font` test, in its order:
//
//	containsFontFile == false || fontName.search(/[A-Z]{6}\+/) != 0 || CharSet == null || charSetListsAllGlyphs == true
//
// A font that is not subset-named, or has no /CharSet, passes whatever its program, which is asked only where the test
// reaches it. `charSetListsAllGlyphs` (`GFPDType1Font`) is a set comparison: the /CharSet's names D against the program
// charset's names P — a Type1C program's charset, or a Type 1 program's glyphs WITH A WIDTH (`getCharSet` is the
// widths' keys, P07.S06) — |D| equal to |P| or one short, and each side's names other than `.notdef` found in the other.
func checkCharSets(d *Document) Result {
	fonts, why := d.usedFonts()
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	subjects := 0
	var unsure *Result
	for _, f := range fonts {
		if f.unresolve {
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: f.where,
					Why: fmt.Sprintf("text selects font %s, which does not resolve in its resources", fontLabel(f.name))}
			}
			continue
		}
		if st := d.name(f.dict["Subtype"]); st != "Type1" && st != "MMType1" {
			continue
		}
		subjects++
		base, _ := d.nameOf(f.dict["BaseFont"])
		desc := d.dict(f.dict["FontDescriptor"])
		// `getStringKey`: a STRING only — a name is null to veraPDF (measured; pdfcpu refuses such a file today).
		charSet, has := d.text(desc["CharSet"])
		if !subsetNamed(base) || !has {
			continue
		}
		sp, known, why, throws := d.simpleProgramOf(f.dict)
		c, program := sp.cff, sp.charSet()
		switch {
		case throws != "":
			return Result{Verdict: CannotCheck, Where: f.where, Why: throws}
		case !known:
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: f.where, Why: fmt.Sprintf("font %s: %s", fontLabel(f.name), why)}
			}
			continue
		case c != nil && c.cid:
			return Result{Verdict: CannotCheck, Where: f.where, Why: cidUnderSimpleFont}
		case program == nil:
			continue // containsFontFile is false
		}
		names, ok := charSetNames(charSet)
		if !ok {
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: f.where, Why: fmt.Sprintf("font %s: its /CharSet holds "+
					"something other than glyph names, which nib does not parse as veraPDF's object parser does", fontLabel(f.name))}
			}
			continue
		}
		missing, extra := charSetDiff(names, program)
		// |D| is |P| or one short: forced by the two inclusions where the program holds `.notdef` (every CFF charset
		// does), but a Type 1 program may not — then a CharSet listing `.notdef` is one too many (measured).
		_, pNotdef := program[".notdef"]
		if !pNotdef && names[".notdef"] && missing == "" && extra == "" {
			return Result{Verdict: Fail, Where: f.where, Why: fmt.Sprintf("font %s (%s): the CharSet lists /.notdef, "+
				"which its program does not hold, so the CharSet is one entry longer than the program's", fontLabel(f.name), base)}
		}
		if missing != "" || extra != "" {
			detail := ""
			switch {
			case extra != "":
				detail = fmt.Sprintf("its program holds /%s, which the CharSet does not list", extra)
			case missing != "":
				detail = fmt.Sprintf("the CharSet lists /%s, which its program does not hold", missing)
			}
			return Result{Verdict: Fail, Where: f.where, Why: fmt.Sprintf("font %s (%s): %s, so the descriptor "+
				"misdescribes the embedded subset", fontLabel(f.name), base, detail)}
		}
		// Given the two inclusions, |D| ∈ {|P|, |P|-1} can fail only where D lists `.notdef` and P does not, which the
		// test above answers — every CFF charset holds `.notdef`, so it reaches Type 1 programs only.
	}
	if unsure != nil {
		return *unsure
	}
	if subjects == 0 {
		return Result{Verdict: NotApplicable, Why: "the document draws with no Type 1 font"}
	}
	return Result{Verdict: Pass}
}

// subsetNamed is `fontName.search(/[A-Z]{6}\+/) == 0`: six capitals and a `+` at the START of the name.
func subsetNamed(base string) bool {
	if len(base) < 7 || base[6] != '+' {
		return false
	}
	for i := 0; i < 6; i++ {
		if base[i] < 'A' || base[i] > 'Z' {
			return false
		}
	}
	return true
}

// charSetNames is `PDType1Font.getDescriptorCharSet` for a /CharSet that holds only names (the shape every producer
// writes): each name `#`-decoded, as veraPDF's object parser reads one. Anything else in the string — which that parser
// reads as other objects, or as a keyword that ends its loop — is not mirrored, and `ok` is false.
func charSetNames(s string) (map[string]bool, bool) {
	src := []byte(s)
	out := map[string]bool{}
	for _, tk := range contentstream.Tokenize(src) {
		switch b := tk.Bytes(src); {
		case tk.Kind == contentstream.Whitespace:
		case tk.Kind == contentstream.Operand && len(b) > 0 && b[0] == '/':
			out[fontcode.Name(b[1:])] = true
		default:
			return nil, false
		}
	}
	return out, true
}

// charSetDiff names a CharSet name the program does not hold and a program name the CharSet does not list, `.notdef`
// exempt both ways.
func charSetDiff(names map[string]bool, program map[string]int) (missing, extra string) {
	for n := range names {
		if _, ok := program[n]; !ok && n != ".notdef" && (missing == "" || n < missing) {
			missing = n
		}
	}
	for n := range program {
		if !names[n] && n != ".notdef" && (extra == "" || n < extra) {
			extra = n
		}
	}
	return missing, extra
}

// fontFile3Subtype is the /Subtype of a font's /FontFile3 stream, "" for none.
func (d *Document) fontFile3Subtype(font types.Dict) string {
	sd, _, err := d.Ctx.DereferenceStreamDict(d.dict(font["FontDescriptor"])["FontFile3"])
	if err != nil || sd == nil {
		return ""
	}
	return d.name(sd.Dict["Subtype"])
}
