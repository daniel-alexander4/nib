package uacheck

import (
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
	"nib/internal/fontcode"
)

// The glyph population — veraPDF's `Glyph`, the object 7.21.7 t1 and t2 run on (`PLAN-ua-coverage.md` P07.S02a).
//
// veraPDF builds one glyph per character code a text-showing operator draws (`GFOpTextShow.getUsedGlyphs`):
// the operator's FIRST argument, a string or an array of strings, cut into codes by the font
// (`PDFont.readCode` — a byte for a simple font, the Encoding CMap's codespace for a Type 0 one). `"`'s first
// argument is a number, so it draws no glyph veraPDF judges; invisible text is not exempt. A glyph is checked
// once per (font, code, render mode, marked content, element) — for a verdict, once per (font, code) is the
// same answer, and it is what is kept here.
//
// The walk is the content walk (`content.go`) — page content, the forms it draws, annotation appearances,
// and the tiling patterns and Type 3 procedures read for their /Lang — so the population and its budgets
// are the walk's, not a second reader's. Decoding is `fontcode`'s, which `internal/pdfops` also reads with
// (ADR-009).

// glyph is one distinct (font, code) the content draws — or, with `unread` set, a string of that font nib could not
// cut into codes the way veraPDF does, which is still at least one glyph.
type glyph struct {
	font   *glyphFont
	code   int
	where  string
	unread string
}

// glyphKey dedupes the population. A typed key, not an `any`: boxing it allocated per code, and the review's
// ceiling-sized document spent 8.5 GB there.
type glyphKey struct {
	font   *glyphFont
	code   int
	unread bool
}

// glyphFont is what the glyph rules need of one font, read once.
type glyphFont struct {
	dict types.Dict
	name string // the resource name that first selected it
	// cs cuts a Type 0 font's strings; nil for a simple font, which reads a byte per code.
	cs *fontcode.Codespace
	// unread is why the font's strings cannot be cut into codes the way veraPDF cuts them, which leaves
	// every glyph it draws unread.
	unread string
	// tu is the /ToUnicode CMap as veraPDF reads it, tuIdentity a /ToUnicode naming Identity-H or -V, and
	// tuUnread why a /ToUnicode that is present could not be read.
	tu         *fontcode.ToUnicode
	tuIdentity bool
	tuUnread   string
	// enc is a simple font's encoding, built on first use (`encodingOf`).
	enc *simpleEncoding
}

// maxGlyphCodes bounds the character codes the walk reads out of string operands. The operator ceiling does
// not bound them: one `Tj` is one operator whatever its string holds, and a form drawn 65,536 times re-reads
// its strings each time. A real document is nowhere near it.
const maxGlyphCodes = 1 << 24

// maxToUnicodeBlocks is the range-index budget every /ToUnicode in one document shares (`fontcode.ParseToUnicode`):
// 65,536 blocks, 69 MB at the ceiling, and a real document's ToUnicode CMaps fill a few hundred.
const maxToUnicodeBlocks = 1 << 16

// maxDistinctGlyphs bounds the population itself. The code ceiling bounds the READING; the distinct (font, code)
// pairs are what is held, and an attacker drawing every code of 256 fonts reaches 16.7 million of them — 16 s and
// 10.7 GB allocated, measured on the P07.S02 review's document once the per-code allocations were gone. A real
// document draws thousands: a CJK book in fifty fonts is a quarter of a million.
const maxDistinctGlyphs = 1 << 20

// showGlyphs records the glyphs one text-showing operator draws.
func (w walker) showGlyphs(src []byte, operands []contentstream.Token, font types.Dict, fontObj int, fontName, where string) {
	if font == nil || len(operands) == 0 {
		return // an unresolved font draws nothing veraPDF judges; `usedFonts` refuses it for the rules
	}
	gf := w.d.glyphFontFor(font, fontObj, fontName)
	first := operands[0]
	var strs [][]byte
	switch first.Kind {
	case contentstream.LiteralString, contentstream.HexString:
		strs = append(strs, fontcode.String(first.Bytes(src)))
	case contentstream.ArrayOpen:
		// `GFOpTextShow.addArrayElements` takes the array's own strings; a nested array is not one.
		end := fontcode.MatchingClose(operands, 0, contentstream.ArrayOpen, contentstream.ArrayClose)
		depth := 0
		for _, tk := range operands[1:min(end, len(operands))] {
			switch tk.Kind {
			case contentstream.ArrayOpen:
				depth++
			case contentstream.ArrayClose:
				depth--
			case contentstream.LiteralString, contentstream.HexString:
				if depth == 0 {
					strs = append(strs, fontcode.String(tk.Bytes(src)))
				}
			}
		}
	default:
		return // `"`: aw ac string — the first argument is a number
	}
	for _, s := range strs {
		if len(s) == 0 {
			continue
		}
		if gf.unread != "" {
			// The codes are unknown, but that there IS at least one glyph is not: record one, so the rules
			// refuse rather than read an unreadable font as drawing nothing.
			w.d.addGlyph(glyph{font: gf, where: where, unread: gf.unread})
			continue
		}
		read := func(_ []byte, v int) bool {
			w.d.glyphCodes++
			if w.d.glyphCodes > maxGlyphCodes {
				w.d.overBudget()
				return false
			}
			w.d.addGlyph(glyph{font: gf, code: v, where: where})
			return !w.d.contentOver
		}
		if gf.cs != nil {
			if !gf.cs.Codes(s, read) {
				w.d.addGlyph(glyph{font: gf, where: where, unread: "the font's CMap cuts this string where veraPDF's own " +
					"reader throws (a byte no range admits with no range of the CMap's own, or the code FFFFFFFF)"})
			}
		} else {
			for _, b := range s {
				if !read(nil, int(b)) {
					break
				}
			}
		}
		if w.d.contentOver {
			return
		}
	}
}

func (d *Document) addGlyph(g glyph) {
	k := glyphKey{g.font, g.code, g.unread != ""}
	if d.glyphSeen == nil {
		d.glyphSeen = map[glyphKey]struct{}{}
	}
	if _, seen := d.glyphSeen[k]; seen {
		return
	}
	if len(d.glyphs) >= maxDistinctGlyphs {
		d.glyphsOver = true
		d.overBudget()
		return
	}
	d.glyphSeen[k] = struct{}{}
	d.glyphs = append(d.glyphs, g)
}

// glyphFontFor reads a font once, keyed by its object (or, for a direct dictionary, by the dictionary).
func (d *Document) glyphFontFor(font types.Dict, obj int, name string) *glyphFont {
	var key any = obj
	if obj == 0 {
		key = dictID(font)
	}
	if gf, ok := d.glyphFonts[key]; ok {
		return gf
	}
	if d.glyphFonts == nil {
		d.glyphFonts = map[any]*glyphFont{}
	}
	gf := &glyphFont{dict: font, name: name}
	d.glyphFonts[key] = gf
	if d.name(font["Subtype"]) == "Type0" {
		gf.cs, gf.unread = d.type0Codespace(font, name)
	}
	if tu, ok := font["ToUnicode"]; ok {
		gf.tu, gf.tuIdentity, gf.tuUnread = d.toUnicodeCMap(tu)
	}
	return gf
}

// ucs2WithEntries are the CMaps veraPDF-parser carries that hold `bfchar`/`bfrange` entries — the five Adobe UCS2
// maps — so a `usecmap` naming one changes a ToUnicode's answers, and naming any other changes nothing.
var ucs2WithEntries = map[string]bool{"Adobe-CNS1-UCS2": true, "Adobe-GB1-UCS2": true, "Adobe-Japan1-UCS2": true,
	"Adobe-Korea1-UCS2": true, "Adobe-KR-UCS2": true}

// toUnicodeCMap reads a font's /ToUnicode as veraPDF's `PDCMap` does: a NAME or a stream whose `/CMapName` begins
// `Identity-` is identity (`PDCMap.isIdentity` reads the stream dictionary's name, not its content); a stream is
// parsed, with every CMap its dictionary's `/UseCMap` chain names merged in (`PDCMap.getCMapFile`); anything else
// is no /ToUnicode at all.
func (d *Document) toUnicodeCMap(obj types.Object) (tu *fontcode.ToUnicode, identity bool, unread string) {
	if n, ok := d.nameOf(obj); ok {
		if strings.HasPrefix(n, "Identity-") {
			return nil, true, ""
		}
		return nil, false, fmt.Sprintf("its /ToUnicode names the CMap %q, which nib does not carry", n)
	}
	sd, _, err := d.Ctx.DereferenceStreamDict(obj)
	if err != nil || sd == nil {
		return nil, false, ""
	}
	if n, ok := d.stringKey(sd.Dict, "CMapName"); ok && strings.HasPrefix(n, "Identity-") {
		return nil, true, ""
	}
	seen := map[*types.StreamDict]bool{}
	var read func(sd *types.StreamDict, hop int) (*fontcode.ToUnicode, string)
	read = func(sd *types.StreamDict, hop int) (*fontcode.ToUnicode, string) {
		if seen[sd] || hop >= maxUseCMapChain {
			return nil, "its /ToUnicode's /UseCMap chain loops or runs past " + fmt.Sprint(maxUseCMapChain) + " CMaps"
		}
		seen[sd] = true
		// **Parsed once per stream, from ONE budget for the document**: fonts sharing a CMap share its parse, and a
		// document of many CMaps cannot hold 34 MB of range index apiece (the P07.S02 re-review: 32 fonts, 1.56 GB).
		tu, cached := d.toUnicodes[dictID(sd.Dict)]
		if !cached {
			if sd.Content == nil && sd.Decode() != nil {
				return nil, "its /ToUnicode stream could not be decoded"
			}
			if d.toUnicodes == nil {
				d.toUnicodes = map[uintptr]*fontcode.ToUnicode{}
				d.toUnicodeBlocks = maxToUnicodeBlocks
			}
			tu = fontcode.ParseToUnicode(sd.Content, &d.toUnicodeBlocks)
			d.toUnicodes[dictID(sd.Dict)] = tu
		}
		if ucs2WithEntries[tu.UseCMapName] {
			return nil, fmt.Sprintf("its /ToUnicode uses %s, whose entries veraPDF merges and nib does not carry", tu.UseCMapName)
		}
		switch n, isName := d.nameOf(sd.Dict["UseCMap"]); {
		case isName && ucs2WithEntries[n]:
			return nil, fmt.Sprintf("its /ToUnicode uses %s, whose entries veraPDF merges and nib does not carry", n)
		case isName:
		default:
			if used, _, err := d.Ctx.DereferenceStreamDict(sd.Dict["UseCMap"]); err == nil && used != nil {
				u, why := read(used, hop+1)
				if why != "" {
					return nil, why
				}
				// `Use` merges into the using CMap, so a cached parse is copied first rather than written through.
				tu = tu.With(u)
			}
		}
		return tu, ""
	}
	tu, why := read(sd, 0)
	if why == "" && tu.Malformed {
		why = "its /ToUnicode CMap holds an entry of the wrong kind, where veraPDF discards the whole CMap — nib " +
			"does not claim to reproduce that parser byte for byte"
	}
	return tu, false, why
}

// type0Codespace is the codespace a Type 0 font's strings are cut by: its /Encoding CMap's ranges, with every
// CMap its chain uses merged in (`CMap.useCMap`), or why nib cannot cut them as veraPDF does.
func (d *Document) type0Codespace(font types.Dict, name string) (*fontcode.Codespace, string) {
	chain, why := d.cmapChain(font, name)
	if why != "" {
		return nil, why
	}
	if len(chain) == 0 {
		return nil, "the Type 0 font has no /Encoding CMap"
	}
	if kids, err := d.Ctx.DereferenceArray(font["DescendantFonts"]); err != nil || len(kids) == 0 || d.dict(kids[0]) == nil {
		return nil, "the Type 0 font has no descendant CIDFont, which is where veraPDF reads its codes"
	}
	cs := &fontcode.Codespace{}
	for _, c := range chain {
		part, why := d.cmapCodespace(c)
		if why != "" {
			return nil, why
		}
		cs.Merge(part)
	}
	if cs.Invalid {
		return nil, "a codespace range in the font's CMap has ends of different lengths, or a begin byte above its end"
	}
	return cs, ""
}

// cmapCodespace is one CMap's own ranges, with the predefined CMap its program names through `usecmap`.
func (d *Document) cmapCodespace(c cmapRef) (*fontcode.Codespace, string) {
	if c.stream == nil {
		switch {
		case c.name == "Identity-H" || c.name == "Identity-V":
			return fontcode.Identity(), ""
		case isPredefinedCMapName(c.name):
			return nil, fmt.Sprintf("the font's CMap %s is one of ISO 32000-1's predefined CMaps, whose codespace nib does not carry", c.name)
		}
		return nil, fmt.Sprintf("the font's CMap %q is neither embedded nor a predefined name", c.name)
	}
	if c.stream.Content == nil && c.stream.Decode() != nil {
		return nil, "the font's embedded CMap could not be decoded"
	}
	cs := fontcode.ParseCodespace(c.stream.Content)
	if cs.Malformed {
		return nil, "the font's embedded CMap holds an entry of the wrong kind, where veraPDF discards it whole"
	}
	// Identity is merged inside the parse, where veraPDF merges it; a predefined CMap veraPDF would load and nib
	// does not carry refuses, and any other name loads nothing there either.
	if u := cs.UsesCMap; isPredefinedCMapName(u) {
		return nil, fmt.Sprintf("the font's embedded CMap uses the CMap %s, whose codespace nib does not carry", u)
	}
	return cs, ""
}

// glyphsDrawn is the population, built by the content walk.
func (d *Document) glyphsDrawn() ([]glyph, string) {
	if _, why := d.contentEvents(); why != "" {
		return nil, why
	}
	return d.glyphs, ""
}

// uniState is what veraPDF's `toUnicode` would be for a glyph.
type uniState int

const (
	uniMapped  uniState = iota // a known string
	uniNull                    // null — 7.21.7 t1 fails
	uniUnknown                 // nib cannot tell; `why` says why
)

// toUnicode is `GFGlyph.getToUnicode` for PDF/UA-1, i.e. `font.toUnicode(code)` (veraPDF-parser `PDFont`,
// `PDSimpleFont`, `PDType0Font`).
func (d *Document) toUnicode(g glyph) (string, uniState, string) {
	f := g.font
	if g.unread != "" {
		return "", uniUnknown, g.unread
	}
	// `PDFont.cMapToUnicode`: a /ToUnicode NAMING Identity answers the code as one UTF-16 unit.
	if f.tuIdentity {
		return string(rune(uint16(g.code))), uniMapped, ""
	}
	if f.tuUnread != "" {
		return "", uniUnknown, f.tuUnread
	}
	s, mapped, known := f.tu.Lookup(g.code)
	switch {
	case !known:
		return "", uniUnknown, "its /ToUnicode CMap does not map this code among the ranges nib indexed, and holds more"
	case mapped:
		return s, uniMapped, ""
	}
	if d.name(f.dict["Subtype"]) == "Type0" {
		return d.type0Fallback(f)
	}
	return d.simpleFallback(f, g.code)
}

// ucs2Orderings are the Adobe character collections veraPDF-parser carries a UCS2 CMap for
// (`resources/font/cmap/Adobe-*-UCS2`, and `PDType0Font.setUcsCMapFromIdentity`'s five orderings).
var ucs2Orderings = map[string]bool{"Japan1": true, "CNS1": true, "GB1": true, "Korea1": true, "KR": true}

// type0Fallback is `PDType0Font.toUnicode` past the /ToUnicode: an Identity CMap falls back to the descendant's
// `Adobe-<ordering>-UCS2`, any other CMap to its OWN collection's `<registry>-<ordering>-UCS2` — and where
// veraPDF carries no such CMap, the answer is null.
func (d *Document) type0Fallback(f *glyphFont) (string, uniState, string) {
	chain, _ := d.cmapChain(f.dict, f.name)
	var info types.Dict
	// Identity is `PDType0Font`'s NAME comparison, and an embedded Encoding stream's name is its `/CMapName`: such a
	// stream falls back through the descendant's collection like the named CMap does (measured, the P07.S02 review).
	if len(chain) > 0 && (chain[0].name == "Identity-H" || chain[0].name == "Identity-V") {
		if kids, err := d.Ctx.DereferenceArray(f.dict["DescendantFonts"]); err == nil && len(kids) > 0 {
			info = d.dict(d.dict(kids[0])["CIDSystemInfo"])
		}
	} else if len(chain) > 0 {
		info, _ = d.systemInfoOf(chain[0])
	}
	reg, _ := d.stringKey(info, "Registry")
	ord, _ := d.stringKey(info, "Ordering")
	if reg == "Adobe" && ucs2Orderings[ord] {
		return "", uniUnknown, fmt.Sprintf("it has no /ToUnicode entry for this code, and veraPDF then reads Adobe-%s-UCS2, "+
			"which nib does not carry", ord)
	}
	return "", uniNull, ""
}
