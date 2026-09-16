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
	for _, f := range fonts {
		if !f.visible {
			continue
		}
		visible++
		if f.unresolve {
			return Result{
				Verdict: CannotCheck,
				Why:     fmt.Sprintf("text selects font %s, which does not resolve in its resources", f.name),
				Where:   f.where,
			}
		}
		if d.name(f.dict["Subtype"]) == "Type3" {
			continue
		}
		fd := d.descriptorOf(f.dict)
		if fd != nil {
			if _, ok := fd["FontFile"]; ok {
				continue
			}
			if _, ok := fd["FontFile2"]; ok {
				continue
			}
			if _, ok := fd["FontFile3"]; ok {
				continue
			}
		}
		return Result{
			Verdict: Fail,
			Why: fmt.Sprintf("font %s (%s) draws visible text and its program is not embedded, so how "+
				"the glyphs look depends on whatever the reader substitutes", f.name, d.baseFontName(f.dict)),
			Where: f.where,
		}
	}
	if visible == 0 {
		return Result{Verdict: NotApplicable, Why: "no text is drawn visibly, so no font is used for rendering"}
	}
	return Result{Verdict: Pass}
}

// standardLatinFaces are the twelve non-symbolic Base-14 faces. Their glyphs map to Unicode through
// the standard Latin glyph names without a /ToUnicode, measured for Helvetica (with and without an
// /Encoding) and Courier; Times is the same family of standard-encoded faces.
var standardLatinFaces = map[string]bool{
	"Helvetica": true, "Helvetica-Bold": true, "Helvetica-Oblique": true, "Helvetica-BoldOblique": true,
	"Times-Roman": true, "Times-Bold": true, "Times-Italic": true, "Times-BoldItalic": true,
	"Courier": true, "Courier-Bold": true, "Courier-Oblique": true, "Courier-BoldOblique": true,
}

// checkFontsMapToUnicode evaluates ua1 7.21.7 t1, over EVERY text operator — visible or not.
//
// **Unlike 7.21.4.1, invisible text is not exempt, measured:** ZapfDingbats in `3 Tr` still fails.
// A reader extracting text needs the mapping whether or not the glyph is painted, which is the whole
// point of an OCR layer.
//
// "Or other mechanisms" is where a checker could guess. This rule does not: a `/ToUnicode` maps, a
// standard Latin Base-14 face maps (measured), Symbol and ZapfDingbats without one do not (measured),
// and a font in any other state is `CannotCheck` naming what nib has not measured — never a pass.
func checkFontsMapToUnicode(d *Document) Result {
	fonts, errWhy := d.usedFonts()
	if errWhy != "" {
		return Result{Verdict: CannotCheck, Why: errWhy}
	}
	if len(fonts) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document shows no text"}
	}
	var unmeasured *Result
	for _, f := range fonts {
		if f.unresolve {
			r := Result{Verdict: CannotCheck, Why: fmt.Sprintf("text selects font %s, which does not resolve", f.name), Where: f.where}
			if unmeasured == nil {
				unmeasured = &r
			}
			continue
		}
		if _, ok := f.dict["ToUnicode"]; ok {
			continue
		}
		base := d.baseFontName(f.dict)
		st := d.name(f.dict["Subtype"])
		if st == "Type1" && standardLatinFaces[base] {
			continue
		}
		if base == "Symbol" || base == "ZapfDingbats" {
			return Result{
				Verdict: Fail,
				Why: fmt.Sprintf("font %s (%s) has no /ToUnicode, and its symbolic glyphs have no standard "+
					"Unicode names, so the text it draws cannot be extracted as characters", f.name, base),
				Where: f.where,
			}
		}
		if unmeasured == nil {
			r := Result{
				Verdict: CannotCheck,
				Why: fmt.Sprintf("font %s (%s, %s) has no /ToUnicode, and whether its encoding maps every "+
					"used code to Unicode is not something nib has measured against veraPDF", f.name, base, st),
				Where: f.where,
			}
			unmeasured = &r
		}
	}
	if unmeasured != nil {
		return *unmeasured
	}
	return Result{Verdict: Pass}
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
		_, ff2 := fd["FontFile2"]
		_, ff3 := fd["FontFile3"]
		if !ff2 && !ff3 {
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
		if _, isCFF := fd["FontFile3"]; isCFF {
			return Result{Verdict: CannotCheck, Why: "the CID font's program is CFF, whose glyph count nib does not read", Where: where}
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
