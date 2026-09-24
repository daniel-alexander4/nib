package uacheck

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A simple font's Unicode, past its /ToUnicode — veraPDF-parser 1.30.2's `PDSimpleFont.toUnicode`:
//
//	name := encoding.getName(code)                       // the font dictionary's encoding
//	if name == null && fontProgram != null { name = fontProgram.getGlyphName(code) }
//	if name == null { return null }
//	if AdobeGlyphList has name { return its value }
//	if ZapfDingbats or Symbol has name { return " " }   // "indicates that toUnicode should not be checked"
//	return null
//
// The encoding is `PDFont.getEncodingMappingFromCOSObject`, with `PDType1Font.calculateEncodingMapping`'s
// override for a standard face read from veraPDF's own metrics. The tables are veraPDF's
// (`glyphnames_tables.go`, generated from `TrueTypePredefined`, `SymbolSet` and `ZapfDingbats`), and the glyph
// list is Adobe's (`agl/glyphlist.txt`, BSD-3-Clause, vendored unmodified) with the one entry veraPDF adds.

//go:embed agl/glyphlist.txt
var aglText string

var (
	aglOnce sync.Once
	aglMap  map[string]string
)

// adobeGlyphList is the glyph list veraPDF reads: Adobe's AGL 2.0, which veraPDF-parser's own copy
// (`resources/font/AdobeGlyphList.txt`) matches entry for entry but one — `.notdef 0000`. **That entry is
// not cosmetic**: every code a standard encoding leaves undefined is named `.notdef`, so it maps to U+0000
// and FAILS 7.21.7 t2 while passing t1.
func adobeGlyphList() map[string]string {
	aglOnce.Do(func() {
		aglMap = map[string]string{".notdef": "\x00"}
		for _, line := range strings.Split(aglText, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == '#' {
				continue
			}
			name, codes, ok := strings.Cut(line, ";")
			if !ok {
				continue
			}
			var v []rune
			for _, h := range strings.Fields(codes) {
				n, err := strconv.ParseUint(h, 16, 32)
				if err != nil {
					v = nil
					break
				}
				v = append(v, rune(n))
			}
			if v != nil {
				aglMap[name] = string(v)
			}
		}
	})
	return aglMap
}

// simpleEncoding is veraPDF's `Encoding`: a predefined table (possibly none) and, when the font's encoding is
// a dictionary or a standard face's metrics, a `/Differences` map — nil and empty are different there, and
// `name` is where the difference shows.
type simpleEncoding struct {
	predefined []string
	diffs      map[int]string
}

// name is `Encoding.getName`.
func (e simpleEncoding) name(code int) (string, bool) {
	if e.diffs == nil {
		if code < len(e.predefined) {
			return e.predefined[code], true
		}
		if len(e.predefined) != 0 {
			return ".notdef", true
		}
		return "", false
	}
	if n, ok := e.diffs[code]; ok {
		return n, true
	}
	if len(e.predefined) == 0 {
		return "", false
	}
	if code < len(e.predefined) {
		return e.predefined[code], true
	}
	return ".notdef", true
}

// namedEncoding is `new Encoding(ASAtom)`: only these three names are tables. `/StandardEncoding` is NOT one —
// named in a font dictionary it is an empty encoding, and only a standard face's metrics reach the Standard table.
func namedEncoding(n string) []string {
	switch n {
	case "MacRomanEncoding":
		return macRomanEncoding[:]
	case "MacExpertEncoding":
		return macExpertEncoding[:]
	case "WinAnsiEncoding":
		return winAnsiEncoding[:]
	}
	return nil
}

// standardFaceNames is `PDType1Font.STANDARD_FONT_NAMES`, matched against the WHOLE /BaseFont (a subset prefix
// makes it another font).
var standardFaceNames = map[string]bool{
	"Courier": true, "Courier-Bold": true, "Courier-BoldOblique": true, "Courier-Oblique": true,
	"Helvetica": true, "Helvetica-Bold": true, "Helvetica-BoldOblique": true, "Helvetica-Oblique": true,
	"Symbol": true, "Times-Bold": true, "Times-BoldItalic": true, "Times-Italic": true, "Times-Roman": true,
	"ZapfDingbats": true,
}

// encodingOf builds a simple font's encoding the way veraPDF does.
func (d *Document) encodingOf(font types.Dict) simpleEncoding {
	var e simpleEncoding
	encObj := d.resolve(font["Encoding"])
	switch v := encObj.(type) {
	case types.Name:
		e.predefined = namedEncoding(v.Value())
	case types.Dict:
		base, _ := d.nameOf(v["BaseEncoding"])
		e.predefined = namedEncoding(base)
		e.diffs = d.differences(v)
	}
	// `PDType1Font`: a standard face with no font descriptor is read from veraPDF's own metrics, and where the
	// dictionary's encoding holds no table the metrics' encoding scheme stands in — AdobeStandardEncoding for the
	// Latin faces, FontSpecific (no table at all) for Symbol and ZapfDingbats — with the dictionary's differences.
	if d.readsStandardMetrics(font) && len(e.predefined) == 0 {
		base := d.name(font["BaseFont"])
		e.predefined = nil
		if base != "Symbol" && base != "ZapfDingbats" {
			e.predefined = standardEncoding[:]
		}
		if dict, ok := encObj.(types.Dict); ok {
			e.diffs = d.differences(dict)
		} else {
			e.diffs = map[int]string{}
		}
	}
	return e
}

// readsStandardMetrics is `PDType1Font`'s constructor condition: a standard name and an empty descriptor.
func (d *Document) readsStandardMetrics(font types.Dict) bool {
	if st := d.name(font["Subtype"]); st != "Type1" && st != "MMType1" {
		return false
	}
	if !standardFaceNames[d.name(font["BaseFont"])] {
		return false
	}
	desc := d.dict(font["FontDescriptor"])
	return len(desc) == 0
}

// differences is `PDFont.getDifferencesFromCosEncoding`: an integer sets the next code, each name takes it and
// advances; anything that is not an array is no differences at all.
func (d *Document) differences(enc types.Dict) map[int]string {
	out := map[int]string{}
	arr, ok := d.resolve(enc["Differences"]).(types.Array)
	if !ok {
		return out
	}
	idx := 0
	for _, o := range arr {
		switch v := d.resolve(o).(type) {
		case types.Integer:
			idx = int(int32(v.Value())) // `getInteger().intValue()`: a code past 2^31 wraps (measured: 4294967361 is 65)
		case types.Name:
			if idx != -1 {
				out[idx] = v.Value()
				idx = int(int32(idx + 1)) // a Java int: 2147483647 steps to -2147483648 (the re-review; veraPDF then throws)
			}
		}
	}
	return out
}

// simpleFallback is `PDSimpleFont.toUnicode` past the /ToUnicode.
func (d *Document) simpleFallback(f *glyphFont, code int) (string, uniState, string) {
	if f.enc == nil {
		e := d.encodingOf(f.dict)
		f.enc = &e
	}
	name, ok := f.enc.name(code)
	if !ok && d.name(f.dict["Subtype"]) == "TrueType" {
		// A TrueType program's name comes from its own table, which P07.S03's door reads (/pending 677).
		n, known, why := d.trueTypeFallbackName(f.dict, code)
		if !known {
			return "", uniUnknown, fmt.Sprintf("its encoding names no glyph for code %#x, and what its TrueType program "+
				"names is not known: %s", code, why)
		}
		if n == "" {
			return "", uniNull, ""
		}
		name, ok = n, true
	}
	if !ok {
		// `fontProgram.getGlyphName(code)` — a Type 3 font has no program, and neither does a font that embeds none.
		if kind := d.embeddedProgram(f.dict); kind != "" {
			return "", uniUnknown, fmt.Sprintf("its encoding names no glyph for code %#x, so veraPDF asks the embedded %s "+
				"program for the name, which nib does not read", code, kind)
		}
		return "", uniNull, ""
	}
	if v, ok := adobeGlyphList()[name]; ok {
		return v, uniMapped, ""
	}
	if symbolGlyphNames[name] || zapfDingbatsGlyphNames[name] {
		return " ", uniMapped, ""
	}
	return "", uniNull, ""
}

// embeddedProgram names the program veraPDF would open for a Type 1 font, or "" for none: `/FontFile` then
// `/FontFile3` (`PDType1Font.getFontProgram`, through `canParseFontFile` — the key present AND a stream); a Type 3 font
// has none. A program under another key is no program to veraPDF, so the glyph's name is null. **A TrueType font is
// not asked here** — its program, and whether veraPDF parses it, is `trueTypeFonts`' (P07.S03).
func (d *Document) embeddedProgram(font types.Dict) string {
	var keys []struct{ key, kind string }
	switch d.name(font["Subtype"]) {
	case "Type1", "MMType1":
		keys = []struct{ key, kind string }{{"FontFile", "Type 1"}, {"FontFile3", "CFF"}}
	}
	desc := d.dict(font["FontDescriptor"])
	for _, k := range keys {
		if sd, _, err := d.Ctx.DereferenceStreamDict(desc[k.key]); err == nil && sd != nil {
			return k.kind
		}
	}
	return ""
}

// cidProgram names the program veraPDF would open for a CIDFont, or "" for none — `PDCIDFont.getFontProgram`:
// `/FontFile2` (a stream) only for a CIDFontType2, else a `/FontFile3` stream whose `/Subtype` is `CIDFontType0C` or
// `OpenType`; `/FontFile` never, and a `/FontFile3` of any other subtype is "Invalid subtype" and no program. The
// P07.S02 re-review measured a CIDFontType2 carrying only `/FontFile` failing 7.21.4.1 t1 where nib passed it.
func (d *Document) cidProgram(cid types.Dict) string {
	desc := d.dict(cid["FontDescriptor"])
	if d.name(cid["Subtype"]) == "CIDFontType2" {
		if sd, _, err := d.Ctx.DereferenceStreamDict(desc["FontFile2"]); err == nil && sd != nil {
			return "TrueType"
		}
	}
	if sd, _, err := d.Ctx.DereferenceStreamDict(desc["FontFile3"]); err == nil && sd != nil {
		switch d.name(sd.Dict["Subtype"]) {
		case "CIDFontType0C":
			return "CFF"
		case "OpenType":
			return "OpenType"
		}
	}
	return ""
}
