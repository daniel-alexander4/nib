package uacheck

import (
	"container/heap"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/fontcode"
)

// The per-glyph font clauses — `PLAN-ua-coverage.md` P07.S04a.
//
// Three tests over veraPDF 1.30.2's `Glyph` (PDFUA-1.xml, measured before a line was written):
//
//	7.21.5 t1    renderingMode == 3 || widthFromFontProgram == null || widthFromDictionary == null ||
//	             |widthFromFontProgram - widthFromDictionary| <= 1
//	7.21.4.1 t2  renderingMode == 3 || isGlyphPresent == null || isGlyphPresent == true
//	7.21.8 t1    name != ".notdef"                                  (no render-mode exemption)
//
// `GFGlyph` fills the presence and both widths ONLY when the font's program exists and was parsed; otherwise they stay
// null and the first two tests pass. So a font with no program — non-embedded Helvetica, a broken one — is a subject
// that passes, and the only fonts that need a program read are the embedded ones. This slice reads a simple TrueType
// program (`readTrueType`, the door P07.S03 built), a CIDFontType2 one under any CMap nib carries (P07.S04b maps codes
// to CIDs), and a Type 3 font's glyph procedures (P07.S04b, `type3.go`); an embedded Type 1 or CFF font's metrics are
// refusals naming the slice that reads them.
//
// **Two measured facts the source did not make obvious**: veraPDF's WinAnsi table names 0x81 `bullet`, not `.notdef`
// (so it passes 7.21.8 and fails the other two, being absent from the program); and a symbolic font whose only cmap is
// (3,1) holds no glyph at all — its lookups ask (3,0) and then (1,0), never (3,1).

func init() {
	register(Rule{Clause: "7.21.5 t1", Summary: "the glyph widths in the font dictionary and the embedded font program shall agree", Check: checkGlyphWidths})
	register(Rule{Clause: "7.21.4.1 t2", Summary: "every glyph drawn shall be present in the embedded font program", Check: checkGlyphsPresent})
	register(Rule{Clause: "7.21.8 t1", Summary: "the .notdef glyph shall not be drawn", Check: checkNoNotdef})
}

// glyphMetrics is what `GFGlyph` holds for one glyph: `known` false when nib cannot say, with why; `valid` false when
// veraPDF leaves presence and widths null (no program, or one it did not parse).
type glyphMetrics struct {
	known, valid bool
	why          string
	present      bool
	program      float64 // widthFromFontProgram
	dictionary   float64 // widthFromDictionary
}

// metricsOf is the one door the two metric clauses read a glyph through.
func (d *Document) metricsOf(g glyph) glyphMetrics {
	if g.unread != "" {
		return glyphMetrics{why: "nib could not cut this string into codes: " + g.unread}
	}
	font := g.font.dict
	switch st := d.name(font["Subtype"]); st {
	case "TrueType":
		f := d.ttByDict[dictID(font)]
		switch {
		case f == nil:
			return glyphMetrics{why: "the font is drawn only where nib's font population does not reach (/pending 678)"}
		case f.embedded == ttFailed && (f.secondObjectUnknown() || (f.program.state == ttParsed && f.throws == ttFailed && d.drawnUnrecorded[dictID(font)])):
			return glyphMetrics{why: "its name table throws and it is drawn in several render modes, some inside a form, an " +
				"appearance, a pattern or a Type 3 procedure, where which of veraPDF's font objects marks it parsed was not measured"}
		case f.kind == "" || (f.embedded == ttFailed && !f.parsedByASecondObject()):
			return glyphMetrics{known: true}
		case f.embedded == ttUnknown:
			return glyphMetrics{why: "whether veraPDF parses its TrueType program is not known — " + f.why}
		case f.names.state == ttUnknown:
			return glyphMetrics{why: f.names.why}
		}
		p := ttGlyphProgram{f.program, f.names}
		dw, ok, why := d.simpleDictWidth(font, g.code)
		if !ok {
			return glyphMetrics{why: why}
		}
		return glyphMetrics{known: true, valid: true, present: g.code == 0 || p.containsCode(g.code), program: p.width(g.code), dictionary: dw}
	case "Type1", "MMType1":
		sp, known, why, throws := d.simpleProgramOf(font)
		switch {
		case throws != "":
			return glyphMetrics{why: throws}
		case !known:
			return glyphMetrics{why: why}
		case sp.t1 != nil:
			return d.type1Metrics(g, sp.t1)
		case sp.cff == nil:
			return glyphMetrics{known: true}
		}
		m, _ := d.type1CMetrics(g, sp.cff)
		return m
	case "Type3":
		return d.type3Metrics(g)
	case "Type0":
		c, known, why := d.cidFontOf(font)
		switch {
		case !known:
			return glyphMetrics{why: why}
		case c == nil:
			return glyphMetrics{known: true}
		}
		cid, why := d.toCID(c, g.code)
		if why != "" {
			return glyphMetrics{why: why}
		}
		dw, ok, why := c.dictWidth(cid)
		if !ok {
			return glyphMetrics{why: why}
		}
		if c.cff != nil {
			return d.cidCFFMetrics(g, c, cid, dw)
		}
		return glyphMetrics{known: true, valid: true, present: g.code == 0 || c.containsCID(cid),
			program: c.withCheck(c.gid(cid)), dictionary: dw}
	default:
		return glyphMetrics{why: unreadSubtype(st)}
	}
}

// unreadSubtype is the one answer the font clauses give a font whose /Subtype is none of the five `FontFactory`
// builds — 7.21.8 t1 passed it, 7.21.4.1 t1 failed it and the metric clauses refused it (R2-9). veraPDF builds no font
// and no glyph for one, so it has no subject in any of them (measured on /Foo and on a missing /Subtype, all seven font
// clauses); nib meets one only where pdfcpu let it through — which it does not, measured, so a refusal is the answer
// that cannot be wrong.
func unreadSubtype(st string) string {
	return fmt.Sprintf("a font of subtype %q is not one veraPDF builds, so it has no subject here, which nib cannot agree with", st)
}

// simpleDictWidth is `PDFont.getWidth` as `GFGlyph` reads it — **declared apart from `pdfops/fontwidth.go`** (ADR-009's
// named exemption): that reader answers how wide nib LAYS OUT a glyph (it takes standard-14 metrics and falls back to
// /MissingWidth past the array), this one what veraPDF compares, and the two rules differ by design. (null is 0): /Widths — only with /FirstChar AND /LastChar,
// for a code between them, and an entry past the array's end or not a number is 0, NOT /MissingWidth (measured) —
// else the descriptor's /MissingWidth, else 0. A /FirstChar or /LastChar that is not a number throws in veraPDF.
func (d *Document) simpleDictWidth(font types.Dict, code int) (float64, bool, string) {
	_, hasW := font["Widths"]
	_, hasF := font["FirstChar"]
	_, hasL := font["LastChar"]
	if hasW && hasF && hasL {
		first, okF := d.javaInt(font["FirstChar"])
		last, okL := d.javaInt(font["LastChar"])
		if d.intKeyNull(font["FirstChar"]) || d.intKeyNull(font["LastChar"]) {
			return 0, false, firstLastThrows // the door's (`widthsThrow`): veraPDF reports nothing on the document
		}
		if !okF || !okL {
			return 0, false, "its /FirstChar or /LastChar is a string, which veraPDF reads as a number nib does not model"
		}
		if arr, ok := d.resolve(font["Widths"]).(types.Array); ok && len(arr) > 0 && code >= first && code <= last {
			if i := code - first; i < len(arr) {
				if v, ok := d.real(arr[i]); ok {
					return v, true, ""
				}
			}
			return 0, true, ""
		}
	}
	if desc := d.dict(font["FontDescriptor"]); desc != nil {
		if mw, has := desc["MissingWidth"]; has {
			v, _ := d.real(mw)
			return v, true, ""
		}
	}
	return 0, true, ""
}

// javaInt is `getIntegerKey(...).intValue()`: an integer or a real cast to a long, then its low 32 bits.
func (d *Document) javaInt(o types.Object) (int, bool) {
	switch v := d.resolve(o).(type) {
	case types.Integer:
		return int(int32(v.Value())), true
	case types.Float:
		return int(int32(javaLong(v.Value()))), true
	}
	return 0, false
}

// real is `getReal`: an integer or a real, else null.
func (d *Document) real(o types.Object) (float64, bool) {
	switch v := d.resolve(o).(type) {
	case types.Integer:
		return float64(v.Value()), true
	case types.Float:
		return v.Value(), true
	}
	return 0, false
}

// ttGlyphProgram is `TrueTypeFontProgram`'s per-code answers over a parsed program and the SHARED name table (the
// first font of the cache group built it; `trueTypeFonts` refuses where that choice matters).
type ttGlyphProgram struct {
	p     trueTypeProgram
	names ttNames
}

// symbolicPath is `isSymbolic || encoding.getDirectBase() == null`: the program's codes are looked up by code.
func (t ttGlyphProgram) symbolicPath() bool { return t.names.symbolic }

// nameOf is the name table's entry; `null` is Java's null — a table that threw holds nothing, and a null name is found
// in no list and no `post` table (an EMPTY post name is a name, measured: the review's false pass).
func (t ttGlyphProgram) nameOf(code int) (name string, null bool) {
	if t.names.state == ttParsed && code >= 0 && code < len(t.names.table) {
		return t.names.table[code], false
	}
	if t.names.state == ttParsed {
		return ".notdef", false
	}
	return "", true
}

// postGID is `TrueTypePostTable.getGID`: 0 for a name it does not hold, and for null.
func (t ttGlyphProgram) postGID(name string, null bool) int {
	if null {
		return 0
	}
	return t.p.post[name]
}

func (t ttGlyphProgram) subtable(platform, encoding int) *ttSubtable {
	return t.p.byPair[[2]int{platform, encoding}] // indexed at parse: `getCmapTable` answers the FIRST matching record
}

// firstGID is `TrueTypeCmapTable.getGID`: the first subtable, in record order, that maps the code.
func (t ttGlyphProgram) firstGID(code int) int {
	for _, s := range t.p.mapping {
		if g, ok := s.m[code]; ok {
			return g
		}
	}
	return 0
}

// mask30 is the (3,0) subtable's high byte, read off the FIRST code put into it; only 00, F0, F1 and F2 count.
func (t ttGlyphProgram) mask30() (*ttSubtable, int, bool) {
	s := t.subtable(3, 0)
	if s == nil || s.first < 0 {
		return s, 0, false
	}
	switch mask := s.first & 0xFF00; mask {
	case 0x0000, 0xF000, 0xF100, 0xF200:
		return s, mask, true
	}
	return s, 0, false
}

// gidFromCMaps is `getGidFromCMaps`: (3,1) by the name's Adobe Glyph List code, then (1,0) by its Mac OS Roman code.
func (t ttGlyphProgram) gidFromCMaps(name string) int {
	uni := -1
	if v, ok := adobeGlyphList()[name]; ok {
		uni = int([]rune(v)[0])
	}
	if s := t.subtable(3, 1); s != nil {
		if g := s.m[uni]; g != 0 {
			return g
		}
	}
	if s := t.subtable(1, 0); s != nil {
		if c, ok := macOSRomanCodes[name]; ok {
			return s.m[c]
		}
	}
	return 0
}

// containsCode is `TrueTypeFontProgram.containsCode`.
func (t ttGlyphProgram) containsCode(code int) bool {
	if !t.symbolicPath() {
		name, null := t.nameOf(code)
		if name == ".notdef" && !null {
			return false
		}
		gid := 0
		if !null {
			gid = t.gidFromCMaps(name)
		} else {
			gid = t.gidFromCMaps("\x00null") // AdobeGlyphList.get(null) is EMPTY (-1): (3,1) at -1, never (1,0)
		}
		if gid == 0 {
			gid = t.postGID(name, null)
		}
		return gid > 0 && gid < t.p.numGlyphs
	}
	if s, mask, ok := t.mask30(); s != nil {
		if !ok {
			return false
		}
		g, has := s.m[mask+code]
		return has && g != 0
	}
	if s := t.subtable(1, 0); s != nil {
		g, has := s.m[code]
		return has && g != 0
	}
	return false
}

// width is `TrueTypeFontProgram.getWidth` — never -1 by the time it returns, so `GFGlyph`'s /MissingWidth fallback
// does not arise for a TrueType program.
func (t ttGlyphProgram) width(code int) float64 {
	if t.symbolicPath() {
		if s, mask, ok := t.mask30(); s != nil && ok {
			if g := s.m[mask+code]; g != 0 {
				return t.withCheck(g)
			}
		}
		if s := t.subtable(1, 0); s != nil {
			return t.withCheck(s.m[code])
		}
		if !t.p.hasCmap {
			return 0
		}
		return t.withCheck(t.firstGID(code))
	}
	if w, ok := t.widthByName(t.nameOf(code)); ok {
		return w
	}
	if !t.p.hasCmap {
		return 0
	}
	return t.withCheck(t.firstGID(code))
}

// widthByName is `getWidth(String)`: the glyph's width where a cmap subtable exists or `post` names it, else -1.
func (t ttGlyphProgram) widthByName(name string, null bool) (float64, bool) {
	key := name
	if null {
		key = "\x00null"
	}
	gid := t.gidFromCMaps(key)
	if gid == 0 {
		gid = t.postGID(name, null)
	}
	_, inPost := t.p.post[name]
	if t.p.nrCmaps != 0 || (t.p.hasPost && inPost && !null) {
		return t.withCheck(gid), true
	}
	return 0, false
}

// withCheck is `getWidthWithCheck`: the advance scaled to 1000 units in FLOAT32, as veraPDF computes it; a glyph past
// the metrics takes the last advance (a monospaced tail) while it is below the glyph count, and the first beyond it.
func (t ttGlyphProgram) withCheck(gid int) float64 {
	adv := t.p.advances
	switch {
	case len(adv) == 0:
		return 0
	case gid < len(adv):
	case gid < t.p.numGlyphs:
		gid = len(adv) - 1
	default:
		gid = 0
	}
	q := float32(1000) / float32(t.p.unitsPerEm)
	return float64(float32(adv[gid]) * q)
}

// metricClause runs 7.21.5 t1 or 7.21.4.1 t2 over the glyphs: a glyph drawn only in mode 3 passes, as do a glyph whose
// font has no usable program; the first failure decides, a refusal waits for one.
func metricClause(d *Document, ok func(glyphMetrics) bool, fail func(glyph, glyphMetrics) string) Result {
	glyphs, why := d.glyphsDrawn()
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	if _, why := d.trueTypeFonts(); why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	var unsure *Result
	for _, g := range glyphs {
		if !g.visible {
			continue
		}
		m := d.metricsOf(g)
		switch {
		case !m.known:
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: g.visibleWhere, Why: fmt.Sprintf("%s: %s", glyphFontLabel(g), m.why)}
			}
		case m.valid && !ok(m):
			return Result{Verdict: Fail, Where: g.visibleWhere, Why: fail(g, m)}
		}
	}
	if unsure != nil {
		return *unsure
	}
	// **A font that did not resolve draws glyphs nib never saw** — pdfcpu drops a font it finds malformed (a CIDFont
	// whose /W range width is a name, measured), and veraPDF judges that font's glyphs. Refuse unless something failed.
	if d.ttUnresolved != "" {
		return Result{Verdict: CannotCheck, Why: d.ttUnresolved}
	}
	if len(glyphs) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document draws no glyph"}
	}
	return Result{Verdict: Pass}
}

// checkGlyphWidths evaluates ua1 7.21.5 t1 — the tolerance is 1 in a thousand units of text space, in doubles, of a
// program width veraPDF computed in float32 (measured: 501.5 against 500 fails, 501 passes).
func checkGlyphWidths(d *Document) Result {
	return metricClause(d, func(m glyphMetrics) bool {
		return math.Abs(m.program-m.dictionary) <= 1
	}, func(g glyph, m glyphMetrics) string {
		return fmt.Sprintf("%s draws code %#x %g units wide by its /Widths but %g by its embedded program, so the text is "+
			"laid out at a width the glyph does not have", glyphFontLabel(g), g.code, m.dictionary, m.program)
	})
}

// checkGlyphsPresent evaluates ua1 7.21.4.1 t2. Code 0 is present whatever the program holds (`GFGlyph`: "every font
// contains the notdef glyph").
func checkGlyphsPresent(d *Document) Result {
	return metricClause(d, func(m glyphMetrics) bool { return m.present }, func(g glyph, _ glyphMetrics) string {
		return fmt.Sprintf("%s draws code %#x, which its embedded program does not hold, so the reader draws a substitute "+
			"or nothing", glyphFontLabel(g), g.code)
	})
}

// checkNoNotdef evaluates ua1 7.21.8 t1 over every glyph, invisible ones included.
func checkNoNotdef(d *Document) Result {
	glyphs, why := d.glyphsDrawn()
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	if _, why := d.trueTypeFonts(); why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	var unsure *Result
	for _, g := range glyphs {
		name, known, why := d.glyphName(g)
		switch {
		case !known:
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Where: g.where, Why: fmt.Sprintf("%s: %s", glyphFontLabel(g), why)}
			}
		case name == ".notdef":
			return Result{Verdict: Fail, Where: g.where, Why: fmt.Sprintf("%s draws code %#x as the .notdef glyph — the "+
				"placeholder a font shows for a character it lacks", glyphFontLabel(g), g.code)}
		}
	}
	if unsure != nil {
		return *unsure
	}
	// **A font that did not resolve draws glyphs nib never saw** — pdfcpu drops a font it finds malformed (a CIDFont
	// whose /W range width is a name, measured), and veraPDF judges that font's glyphs. Refuse unless something failed.
	if d.ttUnresolved != "" {
		return Result{Verdict: CannotCheck, Why: d.ttUnresolved}
	}
	if len(glyphs) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document draws no glyph"}
	}
	return Result{Verdict: Pass}
}

// glyphName is `GFGlyph.name`, "" for null. A simple font reads its encoding — an EMPTY one for a symbolic TrueType
// font — and a TrueType font whose encoding names nothing asks its program: the name table, or " " for a symbolic font
// or one with no /Encoding, or else (null) whether the program holds the code, `.notdef` if not. With no program, only
// code 0 is `.notdef`. A composite font's name is `.notdef` exactly where its program lacks the glyph.
func (d *Document) glyphName(g glyph) (string, bool, string) {
	if g.unread != "" {
		return "", false, "nib could not cut this string into codes: " + g.unread
	}
	font := g.font.dict
	switch d.name(font["Subtype"]) {
	case "Type0":
		// `GFGlyph`: a program veraPDF parsed names a glyph `.notdef` exactly where it does not hold it — CID 0 included,
		// though code 0 counts as present for 7.21.4.1 t2.
		c, known, why := d.cidFontOf(font)
		switch {
		case !known:
			return "", false, why
		case c == nil:
			return "", true, ""
		}
		cid, why := d.toCID(c, g.code)
		switch {
		case why != "":
			return "", false, why
		case c.present(g.code, cid):
			return "", true, ""
		}
		return ".notdef", true, ""
	case "TrueType", "Type1", "MMType1", "Type3":
	default:
		return "", false, unreadSubtype(d.name(font["Subtype"]))
	}
	tt := d.name(font["Subtype"]) == "TrueType"
	var f *ttFont
	if tt {
		if f = d.ttByDict[dictID(font)]; f == nil {
			return "", false, "the font is drawn only where nib's font population does not reach (/pending 678)"
		}
	}
	if !tt || !f.symbolic {
		if g.font.enc == nil {
			e := d.encodingOf(font)
			g.font.enc = &e
		}
		if name, ok := g.font.enc.name(g.code); ok {
			return name, true, ""
		}
	}
	if !tt {
		return "", true, ""
	}
	if f.kind == "" {
		if g.code == 0 {
			return ".notdef", true, ""
		}
		return "", true, ""
	}
	switch {
	case f.names.state == ttUnknown:
		return "", false, f.names.why
	case f.names.symbolic:
		return " ", true, ""
	case f.names.state == ttParsed:
		name, _ := ttGlyphProgram{f.program, f.names}.nameOf(g.code)
		return name, true, ""
	}
	// Null from the program: a table that threw, or tables that never parsed — either way the program holds no name to
	// look the code up by, and `containsCode` answers false.
	return ".notdef", true, ""
}

// parsedByASecondObject is the one way a font whose name table threw still has its glyphs judged: veraPDF builds a font
// object per render mode, the second one finds the program's parse already attempted and marks the font parsed, and the
// glyphs read that mark (measured: a MacExpert-named font drawn in modes 0 and 2, or 0 and 3, fails 7.21.4.1 t2; drawn
// in one mode it has no judged glyph).
//
// Two measured limits: a /FontFile3 program is never marked so (`OpenTypeFontProgram` sets its success only after the
// inner parse returns, so the second object reads false — the review measured it passing where /FontFile2 fails), and a
// font whose uses reach a form or an appearance did not follow the rule in any measured shape (`secondObjectUnknown`).
func (f *ttFont) parsedByASecondObject() bool {
	return f.program.state == ttParsed && f.throws == ttFailed && len(f.modes) > 1 && f.kind != "OpenType" && !f.offPage
}

// secondObjectUnknown is the case the P07.S04a review measured and could not explain: a throwing /FontFile2 font drawn in
// several modes with some use inside a form XObject had no glyph judged, whichever stream held which mode.
func (f *ttFont) secondObjectUnknown() bool {
	return f.program.state == ttParsed && f.throws == ttFailed && len(f.modes) > 1 && f.kind != "OpenType" && f.offPage
}

// cidFontRead is a Type 0 font's program as its descendant reads it: a CIDFontType2's TrueType program as
// `CIDFontType2Program` reads it, or a CIDFontType0C program (`cff`, P07.S05b) — CID-keyed or Type1-keyed — as
// `CFFFontProgram` does.
type cidFontRead struct {
	ttGlyphProgram
	cff    *cffProgram    // the CFF program, when that is what the descendant carries; ttGlyphProgram is then unused
	w      *cidWidthTable // the DESCENDANT's /W and /DW, shared by every Type 0 font over it
	cid    types.Dict
	cmap   fontcode.CIDChain // the Type 0 font's CMap chain, code to CID
	cids   map[int]int       // each code's CID, asked of the CMap once per font
	held   map[int]bool      // whether the CMap held each code (`CMap.containsCode`)
	gidMap *cidGIDMap        // the /CIDToGIDMap stream; nil is the identity — anything but a stream (`CIDToGIDMapping`'s default)
}

// maxCIDAsks bounds what a document's embedded CMaps may cost, entries parsed plus mappings asked: a lookup is a walk,
// measured at ~2 ns a mapping, so the ceiling is ~10 ms of lookups; a maximal CMap parse (1,048,575 entries, ~340 ms)
// spends a quarter of it. A real CJK subset CMap walked by thousands of distinct codes can reach it and refuse — as it
// did before P07.S04b read such CMaps at all.
const maxCIDAsks = 1 << 22

// toCID is `CMap.toCID` and `CMap.containsCode` for one code, remembered per font and charged to the document's
// CMap budget (`maxCIDAsks`) by the mappings asked — a CMap may hold a million entries, and each distinct code walks them. The walk
// stops where the budget does, and once the budget is spent no later lookup walks at all.
//
// **A negative CID is a document veraPDF reports nothing on** (measured, over an identity and a stream CIDToGIDMap):
// `getGID` indexes the stream's array with it, and `getWidthWithCheck` the advances, and nothing catches the
// ArrayIndexOutOfBoundsException. veraPDF builds a glyph for invisible text too, so this refuses every use.
//
// A code no mapping holds is CID 0, and a TrueType program's CID 0 is never present, so there whether the CMap HELD the
// code decides nothing a CID does not (the P07.S04b red-proof found the separate test dead). A CID-keyed CFF charset CAN
// map CID 0 to a glyph, so its presence asks both (`CFFCIDFontProgram.containsCode`; P07.S05b's review measured the false
// pass), and the answer is kept in `held`.
func (d *Document) toCID(c *cidFontRead, code int) (int, string) {
	cid, done := c.cids[code]
	if !done {
		v, held, asked, complete := c.cmap.Lookup(code, maxCIDAsks-d.cidAsks)
		d.cidAsks += asked
		if !complete {
			return 0, fmt.Sprintf("the document's CMaps cost more than %d lookups, where nib stops", maxCIDAsks)
		}
		cid = v
		if c.cids == nil {
			c.cids, c.held = map[int]int{}, map[int]bool{}
		}
		c.cids[code], c.held[code] = cid, held
	}
	if cid < 0 {
		return 0, fmt.Sprintf("its CMap maps code %#x to the negative CID %d, where veraPDF throws an exception it "+
			"does not handle and reports nothing on the document", code, cid)
	}
	return cid, ""
}

// gid is `CIDToGIDMapping.getGID` of a CID toCID has answered (never negative: veraPDF throws on one, and toCID
// refuses it before any caller gets here).
func (c *cidFontRead) gid(cid int) int {
	if c.gidMap == nil {
		return cid
	}
	return c.gidMap.gid(cid)
}

// present is `PDCIDFont.glyphIsPresent` for a code toCID mapped to `cid`. A CID-keyed CFF program holds a code the CMap
// held whose CID its charset maps to a glyph other than 0; a Type1-keyed one under a CIDFont takes a CID other than 0 as a GID
// (`containsGID`), and CID 0 through its OWN encoding by name (`containsCode`) — so an unmapped code is present there
// (measured: code 0 is not `.notdef`).
func (c *cidFontRead) present(code, cid int) bool {
	switch {
	case c.cff == nil:
		return c.containsCID(cid)
	case c.cff.cid:
		return c.held[code] && c.cff.containsCID(cid)
	case cid != 0:
		return cid < c.cff.amount
	}
	return c.cff.containsCode(code)
}

// cffWidth is `PDCIDFont.getWidthFromProgram` over a CFF program: a CID-keyed program's `getWidth(code)` (the charset's
// GID, -1 for a CID it does not name); a Type1-keyed one's `getWidthFromGID(cid)`, and for CID 0 its `getWidth(code)`,
// which asks GID 0 — twice where the first answer is -1.
func (c *cidFontRead) cffWidth(cid int) (float32, *cffErr) {
	p := c.cff
	switch {
	case p.cid:
		gid, ok := p.cidGID(cid)
		if !ok {
			return -1, nil
		}
		return p.widths.width(gid)
	case cid != 0:
		return p.widths.width(cid)
	}
	return p.widths.width(0)
}

// cidCFFMetrics is `GFGlyph` over a CIDFontType0C program: presence as `present`, the program's width with -1 taken as
// the descendant's default width (`getDefaultWidth`: /DW, else 1000), the dictionary's by CID.
func (d *Document) cidCFFMetrics(g glyph, c *cidFontRead, cid int, dw float64) glyphMetrics {
	w, err := c.cffWidth(cid)
	if err != nil {
		return glyphMetrics{why: err.why}
	}
	program := float64(w)
	if w == -1 {
		program = c.w.dw
	}
	return glyphMetrics{known: true, valid: true, present: g.code == 0 || c.present(g.code, cid), program: program, dictionary: dw}
}

// containsCID is `CIDFontType2Program.containsCode` for the code toCID mapped: not CID 0 (which an unheld code also
// is), within the CIDToGIDMap, and a glyph below the program's count.
func (c *cidFontRead) containsCID(cid int) bool {
	return cidType2Contains(c.gidMap, c.p, cid)
}

// cidType2Contains is `CIDFontType2Program.containsCID`, the ONE reading of it — the glyph clauses and 7.21.4.2 t2's
// over-claim both ask it: not CID 0, a CID the map holds (every CID, under the identity), and a glyph below the
// program's `maxp.numGlyphs` (the LAST `maxp`, else the advance count — `readTrueType`).
func cidType2Contains(m *cidGIDMap, p trueTypeProgram, cid int) bool {
	if cid == 0 || (m != nil && cid >= m.size()) {
		return false
	}
	gid := cid
	if m != nil {
		gid = m.gid(cid)
	}
	return gid < p.numGlyphs
}

// cidFontOf reads a Type 0 font's program — a CIDFontType2's TrueType one, or a CIDFontType0C CFF one: nil and known
// when veraPDF has no parsed program (none, or one it cannot read — presence and widths then stay null), and not known
// where nib does not read what veraPDF does.
func (d *Document) cidFontOf(font types.Dict) (*cidFontRead, bool, string) {
	if r, done := d.cidReads[dictID(font)]; done {
		return r.c, r.known, r.why
	}
	c, known, why := d.readCIDFont(font)
	if d.cidReads == nil {
		d.cidReads = map[uintptr]cidRead{}
	}
	d.cidReads[dictID(font)] = cidRead{c, known, why}
	return c, known, why
}

// cidRead is one Type 0 font's reading, kept: the glyph clauses ask it per glyph (the P07.S04a review measured a
// 128 KB CIDToGIDMap re-decoded per glyph at 0.9 ms each, and a 20,000-entry /W re-parsed at 2.4 ms each).
type cidRead struct {
	c     *cidFontRead
	known bool
	why   string
}

// fontFile3SubtypeThrows is a CIDFont whose `/FontFile3` carries a `/Subtype` that is not a name (`/pending 683`).
// veraPDF casts it to a name outside the `try` around the program's parse, so the cast throws an exception nothing
// handles and the job ends with no report. Measured on 1.30.2, a CIDFontType0 and a CIDFontType2 alike: a number, a
// string, a boolean and an array each give NO report; `null` and an absent key give a report (the program is simply
// not one veraPDF opens). Only where `/FontFile3` is the program veraPDF reaches — a CIDFontType2 with a `/FontFile2`
// stream takes that first. A simple font is not this: veraPDF reports on a TrueType font so written, and pdfcpu
// refuses the Type 1 one before any rule runs.
func (d *Document) fontFile3SubtypeThrows(cid types.Dict) string {
	desc := d.dict(cid["FontDescriptor"])
	if d.name(cid["Subtype"]) == "CIDFontType2" {
		if sd, _, err := d.Ctx.DereferenceStreamDict(desc["FontFile2"]); err == nil && sd != nil {
			return ""
		}
	}
	sd, _, err := d.Ctx.DereferenceStreamDict(desc["FontFile3"])
	if err != nil || sd == nil {
		return ""
	}
	sub := sd.Dict["Subtype"]
	if d.resolve(sub) == nil {
		return ""
	}
	if _, isName := d.nameOf(sub); isName {
		return ""
	}
	return "its /FontFile3 carries a /Subtype that is not a name, where veraPDF throws an exception it does not handle"
}

// cidFontThrows is where veraPDF throws reading a Type 0 font's CFF program when it builds the FONT, whatever is drawn
// (a subset font's widths are all read then): it reports nothing on the document.
func (d *Document) cidFontThrows(font types.Dict) string {
	cid := d.descendantOf(font)
	if cid == nil {
		return ""
	}
	if throws := d.fontFile3SubtypeThrows(cid); throws != "" {
		return throws
	}
	if d.cidProgram(cid) != "CFF" {
		return ""
	}
	_, _, _, throws := d.cidCFFOf(cid)
	return throws
}

func (d *Document) readCIDFont(font types.Dict) (*cidFontRead, bool, string) {
	cid := d.descendantOf(font)
	if cid == nil {
		return nil, true, ""
	}
	sd, kind := d.cidTrueTypeStream(cid)
	var c *cidFontRead
	switch {
	case kind == "":
		return nil, true, ""
	case kind == "CFF":
		// The program first, as below; a program veraPDF throws on is the door's (`cidFontThrows`), refused here.
		prog, known, why, throws := d.cidCFFOf(cid)
		switch {
		case throws != "":
			return nil, false, throws
		case !known:
			return nil, false, why
		case prog == nil:
			return nil, true, ""
		}
		c = &cidFontRead{cff: prog}
	case sd == nil:
		// An OpenType program under a CIDFontType0: veraPDF reads its "CFF " table and, with none, has no program — the
		// metrics stay null and pass (measured, R2-1). One that has the table is a program nib does not read.
		if st, why := d.cidProgramParsed(cid); st != ttFailed {
			return nil, false, why
		}
		return nil, true, ""
	default:
		// The program first: a font veraPDF did not parse leaves every metric null and passes, whatever its CMap says.
		prog := d.parseProgramStream(sd)
		switch prog.state {
		case ttUnknown:
			return nil, false, "whether veraPDF parses its TrueType program is not known — " + prog.why
		case ttFailed:
			return nil, true, ""
		}
		c = &cidFontRead{ttGlyphProgram: ttGlyphProgram{p: prog}}
	}
	cmap, why := d.cidMapOf(font)
	if why != "" {
		return nil, false, why
	}
	c.cid, c.cmap = cid, cmap
	c.w = d.cidWidthsOf(cid)
	if c.cff == nil {
		m, why := d.cidGIDMapOf(cid)
		if why != "" {
			return nil, false, why
		}
		c.gidMap = m
	}
	return c, true, ""
}

// cidGIDMap is a /CIDToGIDMap stream as `CIDToGIDMapping.readMapping` reads it — big-endian pairs, an odd last byte
// shifted high — kept as the decoded bytes, not widened into a slice of ints four times their size (R2-3).
type cidGIDMap struct{ b []byte }

// size is `getMappingSize`.
func (m *cidGIDMap) size() int { return (len(m.b) + 1) / 2 }

// gid is `getGID` for a non-negative CID: the pair's value, 0 past the map.
func (m *cidGIDMap) gid(cid int) int {
	if cid < 0 || cid >= m.size() {
		return 0
	}
	v := int(m.b[2*cid]) << 8
	if 2*cid+1 < len(m.b) {
		v += int(m.b[2*cid+1])
	}
	return v
}

// cidGIDRead is one /CIDToGIDMap stream's reading, kept.
type cidGIDRead struct {
	m   *cidGIDMap
	why string
}

// maxCIDToGIDMapDecoded bounds a /CIDToGIDMap's decoded size (the re-review, RR1-3: it was a plain `Decode()`, and
// 7.21.4.2 t2 walks every CID the map holds). A map over every 16-bit CID is 128 KiB, so past a mebibyte nib refuses.
const maxCIDToGIDMapDecoded = 1 << 20

// cidGIDMapOf is a CIDFont's /CIDToGIDMap: nil for the identity (anything but a stream), else the stream — decoded
// ONCE per stream however many CIDFonts and Type 0 fonts name it (R2-3: it was decoded per Type 0 font), and no further
// than `maxCIDToGIDMapDecoded`.
func (d *Document) cidGIDMapOf(cid types.Dict) (*cidGIDMap, string) {
	ms, _, err := d.Ctx.DereferenceStreamDict(cid["CIDToGIDMap"])
	if err != nil || ms == nil {
		return nil, ""
	}
	if r, done := d.cidGIDMaps[dictID(ms.Dict)]; done {
		return r.m, r.why
	}
	var r cidGIDRead
	if why := decodeWithin(ms, maxCIDToGIDMapDecoded, "its /CIDToGIDMap stream"); why != "" {
		r.why = why
	} else {
		r.m = &cidGIDMap{b: ms.Content}
	}
	if d.cidGIDMaps == nil {
		d.cidGIDMaps = map[uintptr]cidGIDRead{}
	}
	d.cidGIDMaps[dictID(ms.Dict)] = r
	return r.m, r.why
}

// cidMapOf is a Type 0 font's code-to-CID mapping as `PDType0Font`'s CMap holds it: the /Encoding CMap's own mappings,
// then each CMap down its /UseCMap chain after (`PDCMap.getCMapFile`). Identity-H and -V are the identity; an embedded
// CMap is read by `fontcode.ParseCIDMap`, once per stream and charged to the document's CMap budget by its
// entries (a maximal one is 13 MB and ~340 ms) against `maxCIDAsks`. The population has already refused a predefined CMap nib does not
// carry and a malformed one (`cmapCodespace`), so those refusals here keep the door honest on its own.
func (d *Document) cidMapOf(font types.Dict) (fontcode.CIDChain, string) {
	chain, why := d.cmapChain(font, "")
	switch {
	case why != "":
		return nil, why
	case len(chain) == 0:
		return nil, "the Type 0 font has no /Encoding CMap"
	}
	if why := cmapChainLoops(chain); why != "" {
		return nil, why
	}
	var out fontcode.CIDChain
	for _, c := range chain {
		var part *fontcode.CIDMap
		switch {
		case c.stream != nil:
			if part = d.cidMaps[dictID(c.stream.Dict)]; part == nil {
				if why := d.decodeFontStream(c.stream, "the font's embedded CMap"); why != "" {
					return nil, why
				}
				part = fontcode.ParseCIDMap(c.stream.Content)
				d.cidAsks += part.Entries()
				if d.cidMaps == nil {
					d.cidMaps = map[uintptr]*fontcode.CIDMap{}
				}
				d.cidMaps[dictID(c.stream.Dict)] = part
			}
			if d.cidAsks > maxCIDAsks {
				return nil, fmt.Sprintf("the document's CMaps cost more than %d lookups, where nib stops", maxCIDAsks)
			}
			if part.Malformed {
				return nil, "the font's embedded CMap holds an entry of the wrong kind, where veraPDF discards it whole"
			}
		case c.name == "Identity-H" || c.name == "Identity-V":
			part = fontcode.IdentityCIDs()
		default:
			// A predefined CMap nib carries a table for (ADR-117). Held for the process, so the document is charged
			// what asking it costs and nothing for reading it.
			var ok bool
			if _, part, ok = fontcode.Predefined(c.name); !ok {
				return nil, fmt.Sprintf("the font's CMap %q is not one nib carries", c.name)
			}
		}
		out = append(out, part)
	}
	return out, ""
}

// cmapChainLoops is a /UseCMap chain `cmapChain` stopped because it loops: veraPDF throws "Loop inside CMap"
// (`PDCMap.getCMapFile`) building the font, whatever its program, and reports nothing on the document. Unreachable through
// a file today — pdfcpu's validator recurses on the same loop and the process dies (/pending 675) — so it is reasoned,
// not measured.
func cmapChainLoops(chain []cmapRef) string {
	if len(chain) > 0 {
		if last := chain[len(chain)-1]; last.stream != nil && last.stream.Dict["UseCMap"] != nil {
			return "its /UseCMap chain loops, where veraPDF throws an exception it does not handle and reports nothing on the document"
		}
	}
	return ""
}

// cidWidths is a /W array as `CIDWArray` holds it: single widths, and ranges in order.
type cidWidths struct {
	single map[int]float64
	ranges []struct {
		lo, hi int
		w      float64
	}
}

// parseCIDW reads /W and /DW once per CIDFont, `CIDWArray`'s rules: an entry opens with a CID (a non-number throws in
// veraPDF); an INTEGER after it opens a range whose width must be a number or the array ends there; an ARRAY after it
// holds single widths, a non-number skipped and a later width for the same CID winning; anything else is ignored.
// /DW when it is a number, else 1000.
func (d *Document) parseCIDW(cid types.Dict) (cidWidths, float64, bool, string) {
	dw := 1000.0
	if v, ok := d.real(cid["DW"]); ok {
		dw = v
	}
	cw := cidWidths{single: map[int]float64{}}
	w, ok := d.resolve(cid["W"]).(types.Array)
	if !ok {
		return cw, dw, true, ""
	}
parse:
	for i := 0; i < len(w); i++ {
		begin, ok := d.javaInt(w[i])
		if !ok && d.intKeyNull(w[i]) {
			return cw, dw, false, wOpenerThrows // the door's (`widthsThrow`): veraPDF reports nothing on the document
		}
		if !ok {
			return cw, dw, false, "its /W array opens an entry with a string, which veraPDF reads as a number nib does not model"
		}
		i++
		if i >= len(w) {
			break
		}
		switch v := d.resolve(w[i]).(type) {
		case types.Integer:
			i++
			if i >= len(w) {
				break parse
			}
			width, ok := d.real(w[i])
			if !ok {
				break parse
			}
			cw.ranges = append(cw.ranges, struct {
				lo, hi int
				w      float64
			}{begin, int(int32(v.Value())), width})
		case types.Array:
			for j, o := range v {
				if x, ok := d.real(o); ok {
					cw.single[int(int32(begin+j))] = x // Java's `cidBegin + i`, an int that wraps
				}
			}
		}
	}
	return cw, dw, true, ""
}

// maxCIDWEntries bounds the /W entries a document's CIDFonts may hold between them, counted once per CIDFont: past it
// nib says it stopped rather than guess. Each is read once and indexed in O(n log n), so what is bounded is the index's
// memory — a real CJK /W holds tens of thousands.
const maxCIDWEntries = 1 << 22

// cidWidthTable is one CIDFont's /W as `CIDWArray` answers it, with its /DW: the single widths, and the ranges cut into
// disjoint segments each owned by the FIRST range, in /W order, that holds its CIDs — `getWidth` scans the ranges in
// order and stops at the first, so an overlap goes to the earlier range whatever the later one says.
type cidWidthTable struct {
	single map[int]float64
	segs   []cidWidthSeg
	dw     float64
	ok     bool
	why    string
}

type cidWidthSeg struct {
	lo, hi int64
	w      float64
}

// cidWidthsOf reads a CIDFont's /W and /DW ONCE, however many Type 0 fonts name it as their descendant — R2-3 measured
// the read, and a range scan per glyph, repeated per Type 0 font: 50 fonts over one descendant of 20,000 ranges drawing
// 2,000 codes each cost 2.5 s. The entries are charged to `maxCIDWEntries` before they are parsed.
func (d *Document) cidWidthsOf(cid types.Dict) *cidWidthTable {
	if t, done := d.cidWidths[dictID(cid)]; done {
		return t
	}
	t := d.readCIDWidths(cid)
	if d.cidWidths == nil {
		d.cidWidths = map[uintptr]*cidWidthTable{}
	}
	d.cidWidths[dictID(cid)] = t
	return t
}

func (d *Document) readCIDWidths(cid types.Dict) *cidWidthTable {
	n := 0
	if w, ok := d.resolve(cid["W"]).(types.Array); ok {
		n = len(w)
		for _, o := range w {
			if a, ok := d.resolve(o).(types.Array); ok {
				n += len(a)
			}
		}
	}
	if d.cidWEntries += n; d.cidWEntries > maxCIDWEntries {
		return &cidWidthTable{why: fmt.Sprintf("the document's CIDFont /W arrays hold more than %d entries, where nib stops", maxCIDWEntries)}
	}
	cw, dw, ok, why := d.parseCIDW(cid)
	return &cidWidthTable{single: cw.single, segs: firstRangeSegments(cw.ranges), dw: dw, ok: ok, why: why}
}

// firstRangeSegments cuts ranges into disjoint segments, sorted, each carrying the width of the FIRST range (lowest
// index) holding it: a sweep over the ranges' boundaries with the ranges open at each held in a heap by index.
func firstRangeSegments(ranges []struct {
	lo, hi int
	w      float64
}) []cidWidthSeg {
	var pts []int64
	var order []int // the non-empty ranges, by start
	for i, r := range ranges {
		if r.lo <= r.hi { // `contains` is lo <= cid <= hi, so a backwards range holds nothing
			pts = append(pts, int64(r.lo), int64(r.hi)+1)
			order = append(order, i)
		}
	}
	slices.Sort(pts)
	pts = slices.Compact(pts)
	sort.SliceStable(order, func(a, b int) bool { return ranges[order[a]].lo < ranges[order[b]].lo })
	open := &intHeap{}
	var segs []cidWidthSeg
	owner := -1
	next := 0
	for k := 0; k+1 < len(pts); k++ {
		p := pts[k]
		for next < len(order) && int64(ranges[order[next]].lo) == p {
			heap.Push(open, order[next])
			next++
		}
		for open.Len() > 0 && int64(ranges[(*open)[0]].hi) < p {
			heap.Pop(open)
		}
		if open.Len() == 0 {
			owner = -1
			continue
		}
		top := (*open)[0]
		if top == owner && len(segs) > 0 && segs[len(segs)-1].hi == p-1 {
			segs[len(segs)-1].hi = pts[k+1] - 1
			continue
		}
		owner = top
		segs = append(segs, cidWidthSeg{p, pts[k+1] - 1, ranges[top].w})
	}
	return segs
}

// intHeap is a min-heap of range indices.
type intHeap []int

func (h intHeap) Len() int           { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *intHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *intHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// dictWidth is `PDCIDFont.getWidth` by CID: a single width, else the first range holding it, else /DW.
func (c *cidFontRead) dictWidth(cid int) (float64, bool, string) {
	return c.w.width(cid)
}

func (t *cidWidthTable) width(cid int) (float64, bool, string) {
	if !t.ok {
		return 0, false, t.why
	}
	if x, ok := t.single[cid]; ok {
		return x, true, ""
	}
	c := int64(cid)
	if i := sort.Search(len(t.segs), func(i int) bool { return t.segs[i].hi >= c }); i < len(t.segs) && t.segs[i].lo <= c {
		return t.segs[i].w, true, ""
	}
	return t.dw, true, ""
}

// cidProgramParsed is `containsFontFile` for a CIDFont — the ONE door 7.21.4.1 t1, 7.21.3.2 t1 and 7.21.4.2 t2 ask
// (ADR-009; /pending 680 was 7.21.3.2 t1 answering it with a second, stricter reader): ttFailed where veraPDF has no
// parsed program — none under a key `PDCIDFont` opens, or one its parser rejects — with why. A CIDFontType2 program is
// read by the same parser as a simple TrueType one, a CIDFontType0C one by the CFF reader (P07.S05b), and an OpenType
// one under a CIDFontType0 through its "CFF " table (`openTypeProgram`, R2-1: it was taken as parsed unread).
func (d *Document) cidProgramParsed(cid types.Dict) (ttState, string) {
	sd, kind := d.cidTrueTypeStream(cid)
	switch kind {
	case "":
		return ttFailed, "there is no program under a key veraPDF opens for this CIDFont"
	case "OpenType":
		if sd == nil { // under a CIDFontType0: `OpenTypeFontProgram` with isCFF
			return d.openTypeProgram(d.dict(cid["FontDescriptor"]))
		}
	}
	if kind == "CFF" {
		c, known, why, throws := d.cidCFFOf(cid)
		switch {
		case throws != "":
			return ttUnknown, throws
		case !known:
			return ttUnknown, why
		case c == nil:
			return ttFailed, "its CFF program does not parse, or there is no program veraPDF opens"
		}
		return ttParsed, ""
	}
	if sd == nil {
		return ttUnknown, "its " + kind + " program stream could not be read"
	}
	p := d.parseProgramStream(sd)
	return p.state, p.why
}

// openTypeRead is one /FontFile3 /OpenType stream's answer, kept.
type openTypeRead struct {
	st  ttState
	why string
}

// openTypeProgram is whether veraPDF has a parsed program for a /FontFile3 /OpenType stream it opens as CFF-flavoured —
// under a simple Type 1 font or a CIDFontType0 (`isCFF`): with no "CFF " table it has none (measured, R2-1: junk, a
// two-byte program and a TrueType one all FAIL 7.21.4.1 t1, where nib passed them unread); with one, it parses that table
// as CFF, which nib does not read through the wrapper — a refusal naming OpenType, never a pass. Decoded once per stream.
func (d *Document) openTypeProgram(desc types.Dict) (ttState, string) {
	sd, _, err := d.Ctx.DereferenceStreamDict(desc["FontFile3"])
	if err != nil || sd == nil {
		return ttFailed, "there is no /FontFile3 stream"
	}
	if r, done := d.openTypeCFF[dictID(sd.Dict)]; done {
		return r.st, r.why
	}
	r := openTypeRead{ttUnknown, "its OpenType program holds a \"CFF \" table, which nib does not read inside an OpenType wrapper"}
	why := d.decodeFontStream(sd, "its OpenType program stream")
	switch {
	case why != "":
		r.why = why
	case !openTypeHasCFFTable(sd.Content):
		r = openTypeRead{ttFailed, "its OpenType program has no \"CFF \" table, which is where veraPDF reads a CFF-flavoured one"}
	}
	if d.openTypeCFF == nil {
		d.openTypeCFF = map[uintptr]openTypeRead{}
	}
	d.openTypeCFF[dictID(sd.Dict)] = r
	return r.st, r.why
}

// cidTrueTypeStream is the ONE choice of a CIDFont's TrueType program stream — `PDCIDFont.getFontProgram` through
// `cidProgram`: a CIDFontType2's /FontFile2, or a /FontFile3 /OpenType (`OpenTypeFontProgram` over the same parser). The
// kind is `cidProgram`'s, and callers route on it: the stream is nil for none and for any program that is not
// TrueType-parsed — a "CFF" one (under either CIDFont subtype) goes to `cidCFFOf`.
func (d *Document) cidTrueTypeStream(cid types.Dict) (*types.StreamDict, string) {
	kind := d.cidProgram(cid)
	if kind == "" || d.name(cid["Subtype"]) != "CIDFontType2" || (kind != "TrueType" && kind != "OpenType") {
		return nil, kind
	}
	key := map[string]string{"TrueType": "FontFile2", "OpenType": "FontFile3"}[kind]
	sd, _, err := d.Ctx.DereferenceStreamDict(d.dict(cid["FontDescriptor"])[key])
	if err != nil {
		return nil, kind
	}
	return sd, kind
}
