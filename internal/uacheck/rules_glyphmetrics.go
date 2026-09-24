package uacheck

import (
	"fmt"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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
// program (`readTrueType`, the door P07.S03 built); an embedded Type 1, CFF or composite font's metrics, and a Type 3
// font's glyph procedures, are refusals naming the slice that reads them.
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
		if kind := d.embeddedProgram(font); kind != "" {
			return glyphMetrics{why: fmt.Sprintf("its embedded %s program is one nib does not read yet (P07.S05/S06)", kind)}
		}
		return glyphMetrics{known: true}
	case "Type3":
		return glyphMetrics{why: "a Type 3 font's glyph procedures are not read yet (P07.S04b)"}
	case "Type0":
		c, known, why := d.cidTrueTypeOf(font)
		switch {
		case !known:
			return glyphMetrics{why: why}
		case c == nil:
			return glyphMetrics{known: true}
		}
		dw, ok, why := c.dictWidth(g.code)
		if !ok {
			return glyphMetrics{why: why}
		}
		return glyphMetrics{known: true, valid: true, present: g.code == 0 || c.containsCode(g.code),
			program: c.withCheck(c.gid(g.code)), dictionary: dw}
	default:
		return glyphMetrics{why: fmt.Sprintf("a font of subtype %q is not one nib reads", st)}
	}
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
		if !okF || !okL {
			return 0, false, "its /FirstChar or /LastChar is not a number, where veraPDF throws an exception it does not handle"
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
		c, known, why := d.cidTrueTypeOf(font)
		switch {
		case !known:
			return "", false, why
		case c == nil || c.containsCode(g.code):
			return "", true, ""
		}
		return ".notdef", true, ""
	case "TrueType", "Type1", "MMType1", "Type3":
	default:
		return "", true, ""
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

// cidTrueType is a CIDFontType2 font's program as `CIDFontType2Program` reads it, over an Identity CMap.
type cidTrueType struct {
	ttGlyphProgram
	w        cidWidths
	dw       float64
	wOK      bool
	wWhy     string
	cid      types.Dict
	identity bool  // the CIDToGIDMap is the identity: anything but a stream (`CIDToGIDMapping`'s default)
	cidToGID []int // otherwise the stream's big-endian pairs, an odd last byte shifted high
}

// gid is `CIDToGIDMapping.getGID(cMap.toCID(code))` — an Identity CMap's CID is its code.
func (c *cidTrueType) gid(code int) int {
	if c.identity {
		return code
	}
	if code >= 0 && code < len(c.cidToGID) {
		return c.cidToGID[code]
	}
	return 0
}

// containsCode is `CIDFontType2Program.containsCode`: the CMap holds the code (an Identity CMap holds every one), and
// `containsCID` — mapped, not CID 0, and a glyph below the program's count.
func (c *cidTrueType) containsCode(code int) bool {
	if code == 0 || (!c.identity && (code < 0 || code >= len(c.cidToGID))) {
		return false
	}
	return c.gid(code) < c.p.numGlyphs
}

// cidTrueTypeOf reads a Type 0 font's CIDFontType2 program: nil and known when veraPDF has no parsed program (none, or
// tables it cannot read — presence and widths then stay null), and not known where nib does not read what veraPDF does.
func (d *Document) cidTrueTypeOf(font types.Dict) (*cidTrueType, bool, string) {
	if r, done := d.cidReads[dictID(font)]; done {
		return r.c, r.known, r.why
	}
	c, known, why := d.readCIDTrueType(font)
	if d.cidReads == nil {
		d.cidReads = map[uintptr]cidRead{}
	}
	d.cidReads[dictID(font)] = cidRead{c, known, why}
	return c, known, why
}

// cidRead is one Type 0 font's reading, kept: the glyph clauses ask it per glyph (the P07.S04a review measured a
// 128 KB CIDToGIDMap re-decoded per glyph at 0.9 ms each, and a 20,000-entry /W re-parsed at 2.4 ms each).
type cidRead struct {
	c     *cidTrueType
	known bool
	why   string
}

func (d *Document) readCIDTrueType(font types.Dict) (*cidTrueType, bool, string) {
	cid := d.descendantOf(font)
	if cid == nil {
		return nil, true, ""
	}
	sd, kind := d.cidTrueTypeStream(cid)
	switch {
	case kind == "":
		return nil, true, ""
	case sd == nil:
		return nil, false, fmt.Sprintf("its embedded %s program is one nib does not read yet (P07.S05)", kind)
	}
	if enc, _ := d.nameOf(font["Encoding"]); enc != "Identity-H" && enc != "Identity-V" {
		return nil, false, "its CMap is not Identity-H or Identity-V, and nib does not yet map codes to CIDs (P07.S04b)"
	}
	prog := d.parseProgramStream(sd)
	switch prog.state {
	case ttUnknown:
		return nil, false, "whether veraPDF parses its TrueType program is not known — " + prog.why
	case ttFailed:
		return nil, true, ""
	}
	c := &cidTrueType{ttGlyphProgram: ttGlyphProgram{p: prog}, cid: cid, identity: true}
	c.w, c.dw, c.wOK, c.wWhy = d.parseCIDW(cid)
	if m, ok := cid["CIDToGIDMap"]; ok {
		if ms, _, err := d.Ctx.DereferenceStreamDict(m); err == nil && ms != nil {
			if ms.Decode() != nil {
				return nil, false, "its /CIDToGIDMap stream could not be decoded"
			}
			c.identity = false
			b := ms.Content
			for i := 0; i < len(b); i += 2 {
				v := int(b[i]) << 8
				if i+1 < len(b) {
					v += int(b[i+1])
				}
				c.cidToGID = append(c.cidToGID, v)
			}
		}
	}
	return c, true, ""
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
		if !ok {
			return cw, dw, false, "its /W array opens an entry with something other than a number, where veraPDF throws an exception it does not handle"
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
					cw.single[begin+j] = x
				}
			}
		}
	}
	return cw, dw, true, ""
}

// dictWidth is `PDCIDFont.getWidth` by CID — an Identity CMap's CID is its code: a single width, else the first range
// holding it, else /DW.
func (c *cidTrueType) dictWidth(code int) (float64, bool, string) {
	if !c.wOK {
		return 0, false, c.wWhy
	}
	if x, ok := c.w.single[code]; ok {
		return x, true, ""
	}
	for _, r := range c.w.ranges {
		if code >= r.lo && code <= r.hi {
			return r.w, true, ""
		}
	}
	return c.dw, true, ""
}

// cidTrueTypeParsed is `containsFontFile`'s parse half for a CIDFont: a CIDFontType2 program is read by the same parser
// as a simple TrueType one. Any other CID program (CFF) is taken as parsed, unchanged — its reader is P07.S05's.
func (d *Document) cidTrueTypeParsed(cid types.Dict) (ttState, string) {
	sd, _ := d.cidTrueTypeStream(cid)
	if sd == nil {
		return ttParsed, "" // no program, or a CFF one: its parse is P07.S05's (/pending 677's CFF half)
	}
	p := d.parseProgramStream(sd)
	return p.state, p.why
}

// cidTrueTypeStream is the ONE choice of a CIDFont's TrueType program stream — `PDCIDFont.getFontProgram` through
// `cidProgram`: a CIDFontType2's /FontFile2, or a /FontFile3 /OpenType (`OpenTypeFontProgram` over the same parser). The
// kind is `cidProgram`'s; the stream is nil for none and for any program that is not TrueType-parsed (a CFF one — which a
// CIDFontType2 can carry under /FontFile3 /CIDFontType0C).
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
