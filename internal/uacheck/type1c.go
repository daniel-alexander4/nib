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

// type1COf is a simple Type 1 font's Type1C program: nil and known where veraPDF has no parsed program (none, one of
// another subtype, or one its parse failed — the metrics then stay null), not known where nib does not read what
// veraPDF would. `throws` is set where veraPDF throws reading it, so that it reports nothing on the document.
func (d *Document) type1COf(font types.Dict) (c *cffProgram, known bool, why, throws string) {
	desc := d.dict(font["FontDescriptor"])
	switch d.embeddedProgram(font) {
	case "":
		return nil, true, "", ""
	case "Type 1":
		return nil, false, "its embedded Type 1 program is one nib does not read yet (P07.S06)", ""
	}
	sd, _, err := d.Ctx.DereferenceStreamDict(desc["FontFile3"])
	if err != nil || sd == nil {
		return nil, true, "", ""
	}
	switch d.name(sd.Dict["Subtype"]) {
	case "Type1C":
	case "OpenType":
		return nil, false, "its embedded program is OpenType, which nib does not read under a Type 1 font", ""
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
	key := type1CKey{dictID(sd.Dict), subset}
	if r, done := d.type1CReads[key]; done {
		return r.c, r.known, r.why, d.type1CThrows[key]
	}
	r := type1CRead{known: true}
	throws = ""
	if sd.Content == nil && sd.Decode() != nil {
		r = type1CRead{why: "its embedded CFF program could not be decoded"}
	} else {
		prog := readCFF(sd.Content, subset)
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
	if named {
		_, present = c.charSet[name]
		w, err = c.widthOfName(name)
	} else {
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

// type1CThrowsFor is the door's question for one glyph: does veraPDF throw reading this glyph's width (a full font's
// widths are read when a glyph asks, so a charstring whose INDEX runs backwards throws only then), or reading the
// program at all?
func (d *Document) type1CThrowsFor(g glyph) string {
	c, known, _, throws := d.type1COf(g.font.dict)
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
// A font that is not subset-named, or has no /CharSet, passes whatever its program — so a Type 1 program nib does not
// read yet is asked only where the test reaches it. `charSetListsAllGlyphs` (`GFPDType1Font`) is a set comparison: the
// /CharSet's names D against the program charset's names P, |D| equal to |P| or one short, and each side's names other
// than `.notdef` found in the other.
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
		c, known, why, throws := d.type1COf(f.dict)
		switch {
		case throws != "":
			return Result{Verdict: CannotCheck, Where: f.where, Why: throws}
		case !known:
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: f.where, Why: fmt.Sprintf("font %s: %s", fontLabel(f.name), why)}
			}
			continue
		case c == nil:
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
		if missing, extra := charSetDiff(names, c.charSet); missing != "" || extra != "" {
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
		// veraPDF also asks |D| ∈ {|P|, |P|-1} first; the two inclusions already force it (P always holds .notdef), so
		// the red-proof found that test dead here and it is not repeated.
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
