package uacheck

import (
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The font rules — `PLAN-accessibility.md` P07.S04.
//
// All three read the fonts that text operators actually select, through the content walk, rather
// than every font dictionary in the file. `pdfops.nonEmbeddedFonts` walks the whole xref table and
// would over-report: measured, a non-embedded Helvetica sitting in a page's resources and never
// selected by a `Tf` fails nothing.

func init() {
	register(Rule{
		Clause:  "7.21.4.1 t1",
		Summary: "the font programs for all fonts used for rendering shall be embedded",
		Check:   checkFontsEmbedded,
	})
	register(Rule{
		Clause:  "7.21.7 t1",
		Summary: "every font shall map all used character codes to Unicode, via ToUnicode or other mechanisms",
		Check:   checkFontsMapToUnicode,
	})
	register(Rule{
		Clause:  "7.21.7 t2",
		Summary: "the Unicode values in a ToUnicode CMap shall be greater than zero, and neither U+FEFF nor U+FFFE",
		Check:   checkToUnicodeValuesValid,
	})
	register(Rule{
		Clause:  "7.21.4.2 t2",
		Summary: "a CIDSet in an embedded CID font's descriptor shall identify all CIDs present in the font program",
		Check:   checkCIDSetsComplete,
	})
}

// usedFont is one font dictionary selected by text, with the first place it was selected.
type usedFont struct {
	dict      types.Dict
	name      string
	where     string
	visible   bool // selected by at least one text operator not in render mode 3
	hidden    bool // selected by at least one text operator in render mode 3
	modes     map[int]bool
	offPage   bool // selected somewhere other than a page's own content stream
	unresolve bool // a text operator named a font resource that does not resolve
}

// usedFonts groups text events by font object, so a font used on forty pages is reported once.
func (d *Document) usedFonts() ([]*usedFont, string) {
	events, errWhy := d.contentEvents()
	if errWhy != "" {
		return nil, errWhy
	}
	byKey := map[string]*usedFont{}
	var order []*usedFont
	for _, ev := range events {
		if !ev.text {
			continue
		}
		key := fmt.Sprintf("obj:%d", ev.fontObj)
		if ev.fontObj == 0 {
			key = "name:" + ev.fontName + "@" + ev.where
		}
		uf := byKey[key]
		if uf == nil {
			uf = &usedFont{dict: ev.font, name: ev.fontName, where: ev.where, unresolve: ev.font == nil}
			byKey[key] = uf
			order = append(order, uf)
		}
		if uf.modes == nil {
			uf.modes = map[int]bool{}
		}
		uf.modes[ev.mode] = true
		uf.offPage = uf.offPage || ev.offPage
		if ev.invisible {
			uf.hidden = true
		} else {
			if !uf.visible {
				uf.where = ev.where
			}
			uf.visible = true
		}
	}
	return order, ""
}

// baseFontName returns /BaseFont without a subset prefix (`ABCDEF+Roboto` → `Roboto`).
func (d *Document) baseFontName(font types.Dict) string {
	n := d.name(font["BaseFont"])
	if i := strings.IndexByte(n, '+'); i == 6 {
		return n[7:]
	}
	return n
}

// descendantOf returns a Type 0 font's descendant CIDFont.
func (d *Document) descendantOf(font types.Dict) types.Dict {
	kids, err := d.Ctx.DereferenceArray(font["DescendantFonts"])
	if err != nil || len(kids) == 0 {
		return nil
	}
	return d.dict(kids[0])
}

// descriptorOf returns the FontDescriptor that holds a font's program — a Type0 font's lives on its
// descendant.
func (d *Document) descriptorOf(font types.Dict) types.Dict {
	if d.name(font["Subtype"]) == "Type0" {
		kids, err := d.Ctx.DereferenceArray(font["DescendantFonts"])
		if err != nil || len(kids) == 0 {
			return nil
		}
		desc := d.dict(kids[0])
		if desc == nil {
			return nil
		}
		return d.dict(desc["FontDescriptor"])
	}
	return d.dict(font["FontDescriptor"])
}

// checkFontsEmbedded evaluates ua1 7.21.4.1 t1, over fonts used VISIBLY.
//
// Invisible text (render mode 3) is not "used for rendering", measured: a non-embedded Helvetica in
// `3 Tr` passes and the same text drawn in `7 Tr` — clipping, which does affect what is painted —
// fails. Type 3 fonts carry their glyphs as content streams and have no font program to embed.
func checkFontsEmbedded(d *Document) Result {
	fonts, errWhy := d.usedFonts()
	if errWhy != "" {
		return Result{Verdict: CannotCheck, Why: errWhy}
	}
	visible := 0
	var unsure *Result
	for _, f := range fonts {
		if !f.visible {
			continue
		}
		visible++
		if f.unresolve {
			// Keep reading: a later font that definitely fails outranks this refusal (the P07.S02 re-review).
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: f.where,
					Why: fmt.Sprintf("text selects font %s, which does not resolve in its resources", fontLabel(f.name))}
			}
			continue
		}
		switch st, why := d.fontEmbedded(f.dict); st {
		case ttParsed:
			continue
		case ttUnknown:
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: f.where,
					Why: fmt.Sprintf("font %s (%s): %s", f.name, d.baseFontName(f.dict), why)}
			}
			continue
		default:
			return Result{
				Verdict: Fail,
				Why: fmt.Sprintf("font %s (%s) draws visible text and %s, so how "+
					"the glyphs look depends on whatever the reader substitutes", f.name, d.baseFontName(f.dict), why),
				Where: f.where,
			}
		}
	}
	// **Invisible text is a subject that PASSES, not an absent one** — veraPDF's test is `… || renderingMode == 3
	// || …` over the font the operator selects, measured at P07.S02: Helvetica drawn only in `3 Tr` passes
	// with one check where nib answered NotApplicable, a strict disagreement no oracle document had reached.
	if visible == 0 {
		// Only a font that RESOLVED is a subject veraPDF is known to see: an unresolved one may be absent (veraPDF has
		// no subject — measured 0/0) or a font pdfcpu dropped (veraPDF has one, and it passes).
		for _, f := range fonts {
			if !f.unresolve {
				return Result{Verdict: Pass}
			}
		}
		if len(fonts) == 0 {
			return Result{Verdict: NotApplicable, Why: "the document shows no text, so no font is used for rendering"}
		}
		return Result{Verdict: CannotCheck, Where: fonts[0].where, Why: fmt.Sprintf("text is drawn only invisibly, in font %s, "+
			"which does not resolve in nib's reading of its resources — veraPDF may have no subject here, or one that passes", fontLabel(fonts[0].name))}
	}
	if unsure != nil {
		return *unsure
	}
	return Result{Verdict: Pass}
}

// fontEmbedded is 7.21.4.1 t1's question for one resolved font — veraPDF's `containsFontFile`, "a program exists AND
// was parsed" — as ONE door over every font type: ttParsed where the font passes, ttFailed with the words for why it
// does not, ttUnknown with the words for why nib cannot say.
//
//   - A Type 3 font carries its glyphs as content streams and has no program to embed.
//   - **A simple font's program is embedded only under a key the font's TYPE reads**: veraPDF's Type 1 font opens
//     `/FontFile` or `/FontFile3` and never `/FontFile2` (measured at P07.S02), and counts a program only if it PARSES
//     it — a Type 1 program (P07.S06), a Type1C one (P07.S05a), and an OpenType one through its "CFF " table (R2-1:
//     measured failing on junk nib had passed unread).
//   - A TrueType program counts only if veraPDF parses it (/pending 677, P07.S03), share group applied.
//   - A Type 0 font passes by its Subtype; its DESCENDANT is the subject that must embed, through the keys `PDCIDFont`
//     opens — `cidProgramParsed`, the door 7.21.3.2 t1 and 7.21.4.2 t2 ask too.
//   - A font of any other subtype is one veraPDF builds no font for (`unreadSubtype`, R2-9).
func (d *Document) fontEmbedded(font types.Dict) (ttState, string) {
	const none = "its program is not embedded"
	switch st := d.name(font["Subtype"]); st {
	case "Type3":
		return ttParsed, ""
	case "TrueType":
		switch st, why := d.trueTypeEmbedded(font); st {
		case ttParsed:
			return ttParsed, ""
		case ttUnknown:
			return ttUnknown, "whether veraPDF parses its TrueType program is not known — " + why
		default:
			if why == "" {
				why = none
			}
			return ttFailed, why
		}
	case "Type1", "MMType1":
		kind := d.embeddedProgram(font)
		switch {
		case kind == "":
			return ttFailed, none
		case kind == "CFF" && d.fontFile3Subtype(font) == "OpenType":
			switch st, why := d.openTypeProgram(d.dict(font["FontDescriptor"])); st {
			case ttFailed:
				return ttFailed, "veraPDF has no parsed program for it (" + why + ")"
			default:
				return ttUnknown, "whether veraPDF parses its OpenType program is not known — " + why
			}
		}
		sp, known, why, throws := d.simpleProgramOf(font)
		switch {
		case throws != "" || !known:
			if throws != "" {
				why = throws
			}
			return ttUnknown, "whether veraPDF parses its " + kind + " program is not known — " + why
		case sp.parsed():
			return ttParsed, ""
		case kind == "Type 1":
			return ttFailed, "veraPDF has no parsed program for it (its Type 1 program does not parse)"
		}
		return ttFailed, "veraPDF has no parsed program for it (its CFF program does not parse, or its /FontFile3 is of a " +
			"subtype veraPDF does not open)"
	case "Type0":
		cid := d.descendantOf(font)
		if cid == nil {
			return ttFailed, none
		}
		program := d.cidProgram(cid)
		kind := map[string]string{"CFF": "CFF", "OpenType": "OpenType"}[program]
		if kind == "" {
			kind = "TrueType"
		}
		switch st, why := d.cidProgramParsed(cid); {
		case st == ttParsed:
			return ttParsed, ""
		case st == ttUnknown:
			return ttUnknown, "whether veraPDF parses its " + kind + " program is not known — " + why
		case program == "":
			return ttFailed, none
		default:
			return ttFailed, "veraPDF cannot read its embedded " + kind + " program (" + why + ")"
		}
	default:
		return ttUnknown, unreadSubtype(st)
	}
}

// checkFontsMapToUnicode evaluates ua1 7.21.7 t1, `toUnicode != null`, over every GLYPH — veraPDF's object
// is `Glyph`, so a font whose /ToUnicode exists but does not map a code the content draws fails, which a
// per-font presence test passed (`/pending 657`). Invisible text is not exempt, measured: ZapfDingbats in
// `3 Tr` still fails, because a reader extracting text needs the mapping whether or not the glyph is painted.
//
// "Or other mechanisms" is where a checker could guess, and this one does not: a glyph whose mapping nib
// cannot compute (`toUnicode`'s `uniUnknown`) is `CannotCheck` naming why, and a definite failure anywhere
// outranks it.
func checkFontsMapToUnicode(d *Document) Result {
	return glyphClause(d, func(_ string, st uniState) bool { return st != uniNull },
		func(g glyph, _ string) string {
			return fmt.Sprintf("%s draws code %#x, which nothing maps to Unicode — no /ToUnicode entry and no fallback "+
				"veraPDF reads — so the text cannot be extracted as characters", glyphFontLabel(g), g.code)
		})
}

// checkToUnicodeValuesValid evaluates ua1 7.21.7 t2, veraPDF's test transcribed:
//
//	toUnicode == null || (toUnicode.indexOf("\u0000") == -1 && toUnicode.indexOf("\uFFFE") == -1 &&
//	toUnicode.indexOf("\uFEFF") == -1)
//
// It is a substring test on the whole mapped TEXT, so a destination of several characters fails if any of
// them is one of the three. A glyph with no mapping passes; a glyph whose mapping nib cannot compute cannot.
func checkToUnicodeValuesValid(d *Document) Result {
	return glyphClause(d, func(s string, st uniState) bool {
		return st == uniNull || !strings.ContainsAny(s, "\u0000\uFFFE\uFEFF")
	}, func(g glyph, s string) string {
		return fmt.Sprintf("%s maps code %#x to %+q, which holds U+0000, U+FEFF or U+FFFE — none of them a character "+
			"a reader can extract", glyphFontLabel(g), g.code, s)
	})
}

// glyphClause runs one 7.21.7 test over the glyph population: the first glyph failing `ok` fails the clause,
// and otherwise the first glyph nib could not judge refuses it.
func glyphClause(d *Document, ok func(string, uniState) bool, fail func(glyph, string) string) Result {
	glyphs, why := d.glyphsDrawn()
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	var unsure *Result
	for _, g := range glyphs {
		s, st, why := d.toUnicode(g)
		switch {
		case st == uniUnknown:
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: g.where, Why: fmt.Sprintf("%s: %s", glyphFontLabel(g), why)}
			}
		case !ok(s, st):
			return Result{Verdict: Fail, Where: g.where, Why: fail(g, s)}
		}
	}
	// **A font that does not resolve may draw glyphs nib never saw** — pdfcpu's validator drops fonts it judges
	// invalid (P07.S01) — so an unresolved selection is a refusal, never "nothing drawn".
	fonts, why := d.usedFonts()
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	for _, f := range fonts {
		if f.unresolve && unsure == nil {
			unsure = &Result{Verdict: CannotCheck, Where: f.where,
				Why: fmt.Sprintf("text selects font %s, which does not resolve in nib's reading of its resources, so the glyphs it draws were never read", fontLabel(f.name))}
		}
	}
	if unsure != nil {
		return *unsure
	}
	if len(glyphs) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document draws no glyph with Tj, TJ or '"}
	}
	return Result{Verdict: Pass}
}

// fontLabel names a font by the resource name that selected it — or says that no `Tf` selected one, which is how
// text set through an ExtGState's `/Font` reads here (nib does not read that key).
func fontLabel(name string) string {
	if name == "" {
		return "(none selected by Tf — an ExtGState /Font, which nib does not read)"
	}
	return name
}

func glyphFontLabel(g glyph) string {
	return "font " + fontLabel(g.font.name)
}

// checkCIDSetsComplete evaluates ua1 7.21.4.2 t2 over the CID fonts text selects — veraPDF's `PDCIDFont` test, in its
// order (`cidSetResult`):
//
//	containsFontFile == false || fontName.search(/[A-Z]{6}\+/) != 0 || containsCIDSet == false || cidSetListsAllGlyphs == true
//
// The clause is conditional on a /CIDSet existing, but its SUBJECT is every CID font: a font with no /CIDSet, no
// program, or a program veraPDF did not parse satisfies it and passes, which is what veraPDF reports. nib's own output
// never carries a /CIDSet (P04.S02 removes pdfcpu's), so its embedded fonts pass.
func checkCIDSetsComplete(d *Document) Result {
	fonts, errWhy := d.usedFonts()
	if errWhy != "" {
		return Result{Verdict: CannotCheck, Why: errWhy}
	}
	withSet := 0
	// **A font nib cannot settle is held, and the later fonts are still judged** (the package's convention, `heldRefusal`;
	// the P07 phase-close re-review RR1-1): returning at the first refusal reported "nib could not look" about a document
	// whose next font plainly fails.
	var held heldRefusal
	for _, f := range fonts {
		if f.unresolve {
			// **An unresolved font is held whatever the other fonts say** (/pending 722): it may be a CID font pdfcpu
			// DROPPED (measured at P07.S04a: veraPDF judges one nib had called absent), and the refusal used to be reached
			// only when no other CID font resolved — so one resolved font with an exact /CIDSet answered Pass over a font
			// nib never read. Every sibling font door (`checkFontsEmbedded`, `type0Fonts`, `trueTypeFonts`) refuses.
			held.hold(fmt.Sprintf("text selects font %s, which does not resolve in nib's reading of its resources, so "+
				"whether it is an embedded CID font was never read", fontLabel(f.name)), f.where)
			continue
		}
		if d.name(f.dict["Subtype"]) != "Type0" {
			continue
		}
		// **The subject is the DESCENDANT, and a missing descriptor does not remove it** — measured on veraPDF for
		// /pending 722: a Type 0 font with no /DescendantFonts has no 7.21.4.2 t2 check (0/0), and one whose CIDFont has
		// no /FontDescriptor has one that PASSES (1/0, `containsFontFile` false), which `cidSetResult` answers since a
		// nil descriptor holds no /CIDSet. pdfcpu's validator drops both shapes today, so both reach here as unresolved.
		if d.descendantOf(f.dict) == nil {
			continue
		}
		fd := d.descriptorOf(f.dict)
		// **A NON-embedded CID font is a subject too, and passes** (measured at P07.S01, on six documents), and so is
		// one with no /CIDSet (law 5 at S05): the conditions are inside the check, not a filter on its population.
		withSet++
		if r := d.cidSetResult(f, fd); r != nil {
			if r.Verdict != CannotCheck {
				return *r
			}
			held.hold(r.Why, r.Where)
		}
	}
	if r, ok := held.result(); ok {
		return r
	}
	if withSet == 0 {
		return Result{Verdict: NotApplicable, Why: "no embedded CID font is used by text"}
	}
	return Result{Verdict: Pass}
}

// cidSetMaxBytes is `GFPDCIDFont.maxSize`: veraPDF reads no more of a /CIDSet than this, so a bit past it is no claim
// (measured, R1-2: a stray bit past byte 16,384 passes).
const cidSetMaxBytes = 16384

// maxCIDSetDecoded bounds what nib DECODES of a /CIDSet before cutting it at `cidSetMaxBytes` (the P07 phase-close
// re-review, RR1-3: a plain `Decode()` let a compressed set expand as far as the document chose). A set naming every
// 16-bit CID is 8 KiB, so a stream decoding past a mebibyte is refused through the one door for capped decodes
// (`decodeWithin`) rather than cut: veraPDF reads such a stream's first 16 KiB, and nib does not decode the rest to reach them.
const maxCIDSetDecoded = 1 << 20

// cidSetRead is one descendant's 7.21.4.2 t2 answer without its location: nil where it passes.
type cidSetRead struct{ r *Result }

// cidSetResult is 7.21.4.2 t2 for one Type 0 font, the ONE door for both programs (/pending 684: the CFF branch applied
// veraPDF's short-circuits and the TrueType one none): nil where it passes. Each short-circuit is measured on a
// CIDFontType2 at the P07 phase close — a program veraPDF did not parse (no hhea, a cmap subtable past the end), a
// descendant whose /BaseFont is not subset-named, and a /CIDSet that is not a stream all pass an incomplete set — and
// only then is the set read, cut at `cidSetMaxBytes`, and judged against the program's population.
//
// The answer is the DESCENDANT's — its name, its descriptor's /CIDSet, its program — so it is read once per descendant
// however many Type 0 fonts share it (the re-review, RR1-3: it was re-read per font); `cidSetJudged` counts the reads.
func (d *Document) cidSetResult(f *usedFont, fd types.Dict) *Result {
	cid := d.descendantOf(f.dict)
	// **veraPDF's terms do not throw, so ITS order decides nothing — but nib's program term can be unknown**, and a
	// disjunction is settled by any one true term (the re-review, RR1-2: the program was asked first, so a font its name
	// alone passes was refused). So the terms nib always settles go first — no /CIDSet stream, a /BaseFont that is not
	// subset-named — and the program, the one term nib may not know, is asked only when neither has passed the font.
	cs, _, err := d.Ctx.DereferenceStreamDict(fd["CIDSet"])
	if err != nil || cs == nil || cid == nil {
		return nil // `getCIDSet` answers only a stream: anything else is containsCIDSet == false
	}
	if base, _ := d.nameOf(cid["BaseFont"]); !subsetNamed(base) {
		return nil
	}
	read, done := d.cidSets[dictID(cid)]
	if !done {
		read = cidSetRead{d.readCIDSet(cid, cs)}
		if d.cidSets == nil {
			d.cidSets = map[uintptr]cidSetRead{}
		}
		d.cidSets[dictID(cid)] = read
	}
	if read.r == nil {
		return nil
	}
	r := *read.r
	r.Where = fmt.Sprintf("%s, font %s (%s)", f.where, f.name, d.baseFontName(f.dict))
	return &r
}

// readCIDSet is cidSetResult's read for one subset-named descendant carrying a /CIDSet stream: the program term, then
// the set against the program's population.
func (d *Document) readCIDSet(cid types.Dict, cs *types.StreamDict) *Result {
	switch st, why := d.cidProgramParsed(cid); st {
	case ttFailed:
		return nil
	case ttUnknown:
		return &Result{Verdict: CannotCheck, Why: "whether veraPDF parses the CID font's program is not known — " + why}
	}
	d.cidSetJudged++
	if why := decodeWithin(cs, maxCIDSetDecoded, "the /CIDSet stream"); why != "" {
		return &Result{Verdict: CannotCheck, Why: why}
	}
	set := cs.Content
	if len(set) > cidSetMaxBytes {
		set = set[:cidSetMaxBytes]
	}
	bit := func(i int) bool { return i >= 0 && i/8 < len(set) && set[i/8]&(0x80>>uint(i%8)) != 0 }
	if d.cidProgram(cid) == "CFF" {
		c, _, _, _ := d.cidCFFOf(cid) // parsed: cidProgramParsed has answered
		return cffCIDSetResult(c, bit, len(set), "")
	}
	sd, _ := d.cidTrueTypeStream(cid)
	return d.trueTypeCIDSetResult(cid, d.parseProgramStream(sd), bit, len(set), "")
}

// trueTypeCIDSetResult is `getcidSetListsAllGlyphs` over a `CIDFontType2Program` (R1-2, measured): every CID of its
// `getCIDList` but 0 must be set — under the identity, every CID below the hhea numberOfHMetrics; under a
// /CIDToGIDMap stream, every CID the map holds whose glyph the program has — and every set bit from 1 up must be a CID
// the program holds (`cidType2Contains`, by the LAST maxp).
func (d *Document) trueTypeCIDSetResult(cid types.Dict, p trueTypeProgram, bit func(int) bool, setBytes int, where string) *Result {
	m, why := d.cidGIDMapOf(cid)
	if why != "" {
		return &Result{Verdict: CannotCheck, Where: where, Why: why}
	}
	listed := len(p.advances)
	if m != nil {
		listed = m.size()
	}
	for i := 1; i < listed; i++ {
		if (m == nil || cidType2Contains(m, p, i)) && !bit(i) {
			return &Result{Verdict: Fail, Where: where, Why: fmt.Sprintf("the /CIDSet does not identify CID %d, and the font "+
				"program holds it — a set covering only the glyphs a page uses is exactly what veraPDF rejects", i)}
		}
	}
	for i := 1; i < setBytes*8; i++ {
		if bit(i) && !cidType2Contains(m, p, i) {
			return &Result{Verdict: Fail, Where: where, Why: fmt.Sprintf("the /CIDSet claims CID %d, and the font program "+
				"holds only %d glyphs — a set that over-claims misidentifies the program as surely as one that omits", i, p.numGlyphs)}
		}
	}
	return nil
}

// cffCIDSetResult is the population half of 7.21.4.2 t2 over a CIDFontType0C program veraPDF parsed (P07.S05b): every
// CID the program's charset names (`getCIDList`) must be set, CID 0 aside, and every set bit from 1 up must be a CID the
// program holds (`containsCID`). A Type1-keyed program names no CID and holds none, so any bit past 0 fails it (measured).
func cffCIDSetResult(c *cffProgram, bit func(int) bool, setBytes int, where string) *Result {
	if c.cid {
		missing := -1
		for k := range c.cidCharSet {
			if k != 0 && !bit(k) && (missing < 0 || k < missing) {
				missing = k
			}
		}
		if missing >= 0 {
			return &Result{Verdict: Fail, Where: where, Why: fmt.Sprintf("the /CIDSet does not identify CID %d, which the "+
				"CFF program's charset holds — a set covering only the glyphs a page uses is exactly what veraPDF rejects", missing)}
		}
	}
	for i := 1; i < setBytes*8; i++ {
		if bit(i) && !(c.cid && c.containsCID(i)) {
			return &Result{Verdict: Fail, Where: where, Why: fmt.Sprintf("the /CIDSet claims CID %d, which the CFF program "+
				"does not hold — a set that over-claims misidentifies the program as surely as one that omits", i)}
		}
	}
	return nil
}
