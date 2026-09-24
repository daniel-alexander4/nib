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
		if !ev.invisible {
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
		if d.name(f.dict["Subtype"]) == "Type3" {
			continue
		}
		// **A simple font's program is embedded only under a key the font's TYPE reads** — the door the glyph fallback
		// also asks (`embeddedProgram`): veraPDF's Type 1 font opens `/FontFile` or `/FontFile3` and never `/FontFile2`,
		// so a Type 1 program filed there is no program and the font fails (measured at P07.S02, where nib had passed it
		// on the key's presence). A Type 0 font is judged by its descendant's descriptor, as it was measured before.
		if d.name(f.dict["Subtype"]) != "Type0" {
			if d.embeddedProgram(f.dict) != "" {
				continue
			}
		} else if cid := d.descendantOf(f.dict); cid != nil && d.cidProgram(cid) != "" {
			// A Type 0 font passes by its Subtype; its DESCENDANT is the subject that must embed — through the keys
			// `PDCIDFont` opens (`cidProgram`).
			continue
		}
		return Result{
			Verdict: Fail,
			Why: fmt.Sprintf("font %s (%s) draws visible text and its program is not embedded, so how "+
				"the glyphs look depends on whatever the reader substitutes", f.name, d.baseFontName(f.dict)),
			Where: f.where,
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

// checkCIDSetsComplete evaluates ua1 7.21.4.2 t2 over the CID fonts text selects — the set must be
// EXACT, neither omitting a glyph slot nor claiming one the program does not hold.
//
// The clause is conditional on a /CIDSet existing, but its SUBJECT is every embedded CID font: a
// font with no /CIDSet satisfies it and passes, which is what veraPDF reports. nib's own output never
// carries a /CIDSet (P04.S02 removes pdfcpu's), so its embedded fonts pass. For a document that does, the
// population the set must cover is `maxp.numGlyphs` — measured: a set of the non-empty glyphs FAILS
// and a set of every glyph slot PASSES — so the rule reads only the program's `maxp` table.
func checkCIDSetsComplete(d *Document) Result {
	fonts, errWhy := d.usedFonts()
	if errWhy != "" {
		return Result{Verdict: CannotCheck, Why: errWhy}
	}
	withSet := 0
	for _, f := range fonts {
		if f.unresolve {
			continue
		}
		if d.name(f.dict["Subtype"]) != "Type0" {
			continue
		}
		fd := d.descriptorOf(f.dict)
		if fd == nil {
			continue
		}
		program := ""
		if cid := d.descendantOf(f.dict); cid != nil {
			program = d.cidProgram(cid)
		}
		if program == "" {
			// **A NON-embedded CID font is a subject too, and passes** (measured at P07.S01, on six documents):
			// the profile's test opens with `containsFontFile == false ||`, so veraPDF runs the check and it
			// passes; nib answered NotApplicable, which the oracle scores as a different state.
			withSet++
			continue
		}
		// **The subject is the embedded CID font, not the /CIDSet — measured by law 5.** veraPDF
		// runs this check on every embedded CID font descriptor and PASSES one with no /CIDSet: the
		// condition is inside the check, not a filter on its population. nib answered NotApplicable
		// here until the S05 guard found four documents disagreeing.
		withSet++
		csObj, has := fd["CIDSet"]
		if !has {
			continue
		}
		where := fmt.Sprintf("%s, font %s (%s)", f.where, f.name, d.baseFontName(f.dict))
		cs, _, err := d.Ctx.DereferenceStreamDict(csObj)
		if err != nil || cs == nil || cs.Decode() != nil {
			return Result{Verdict: CannotCheck, Why: "the /CIDSet stream could not be read", Where: where}
		}
		if program != "TrueType" {
			return Result{Verdict: CannotCheck, Why: "the CID font's program is " + program + ", whose glyph count nib does not read", Where: where}
		}
		prog, _, perr := d.Ctx.DereferenceStreamDict(fd["FontFile2"])
		if perr != nil || prog == nil || prog.Decode() != nil {
			return Result{Verdict: CannotCheck, Why: "the CID font carries a /CIDSet but no readable TrueType program", Where: where}
		}
		kids, _ := d.Ctx.DereferenceArray(f.dict["DescendantFonts"])
		if len(kids) > 0 {
			if desc := d.dict(kids[0]); desc != nil {
				if m, ok := desc["CIDToGIDMap"]; ok {
					if d.name(m) != "Identity" {
						return Result{Verdict: CannotCheck, Why: "the CID font maps CIDs to glyphs through a /CIDToGIDMap stream, which nib does not resolve", Where: where}
					}
				}
			}
		}
		n, gerr := trueTypeGlyphCount(prog.Content)
		if gerr != nil {
			return Result{Verdict: CannotCheck, Why: gerr.Error(), Where: where}
		}
		if ok, missing, extra := cidSetExact(cs.Content, n); !ok {
			why := fmt.Sprintf("the /CIDSet does not identify CID %d, and the font program holds %d glyphs — "+
				"a set covering only the glyphs a page uses is exactly what veraPDF rejects", missing, n)
			if missing < 0 {
				why = fmt.Sprintf("the /CIDSet claims CID %d, and the font program holds only %d glyphs — "+
					"a set that over-claims misidentifies the program as surely as one that omits", extra, n)
			}
			return Result{Verdict: Fail, Why: why, Where: where}
		}
	}
	if withSet == 0 {
		return Result{Verdict: NotApplicable, Why: "no embedded CID font is used by text"}
	}
	return Result{Verdict: Pass}
}
