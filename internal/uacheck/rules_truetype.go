package uacheck

import (
	"fmt"
	"math"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The TrueType program, at the font — `PLAN-ua-coverage.md` P07.S03.
//
// Four clauses over veraPDF 1.30.2's `PDTrueTypeFont` and `TrueTypeFontProgram` objects (PDFUA-1.xml, 7.21.6):
//
//	t1  TrueTypeFontProgram  isSymbolic || (cmap30Present ? nrCmaps > 1 : nrCmaps > 0)
//	t2  PDTrueTypeFont       isSymbolic || ((Encoding == MacRoman || WinAnsi) && (!containsDifferences || differencesAreUnicodeCompliant))
//	t3  PDTrueTypeFont       !isSymbolic || Encoding == null
//	t4  TrueTypeFontProgram  !isSymbolic || nrCmaps == 1 || cmap30Present
//
// Every half was measured on veraPDF over a synthetic program generator before it was written (the slice's
// grill); what reading the source alone got wrong is written where it bites.
//
// **t1 and t4 have a subject only when veraPDF PARSED the program.** veraPDF parses lazily: the program object
// exists whenever the stream does, and the parse runs when the font object is built. So a program that does not
// parse has no t1/t4 subject, is not "embedded" for 7.21.4.1 t1 (`containsFontFile` is "exists AND parsed" —
// /pending 677), and makes t2's Differences test FALSE rather than vacuous.
//
// **One parse can serve several fonts, and the first font veraPDF opens decides it.** veraPDF caches the program
// under (stream, symbolic flag, the /Encoding's OBJECT key — "direct" for every direct value), so fonts sharing
// a stream with direct encodings share one parse. The name table (`createCIDToNameTable`) is built from the FIRST
// font's encoding, and when it throws only that font is left unparsed — every later font asks a program that was
// already attempted and finds its tables read. Measured: MacExpert-then-WinAnsi fails 7.21.4.1 t1 and
// WinAnsi-then-MacExpert passes it. nib emulates the half of that which does not depend on which font comes
// first, and refuses the half that does.

func init() {
	register(Rule{Clause: "7.21.6 t1", Summary: "a non-symbolic TrueType font program shall hold a cmap veraPDF can map codes through", Check: checkTrueTypeNonSymbolicCmap})
	register(Rule{Clause: "7.21.6 t2", Summary: "a non-symbolic TrueType font shall use MacRomanEncoding or WinAnsiEncoding, with glyph names from the Adobe Glyph List", Check: checkTrueTypeNonSymbolicEncoding})
	register(Rule{Clause: "7.21.6 t3", Summary: "a symbolic TrueType font shall not have an Encoding entry", Check: checkTrueTypeSymbolicNoEncoding})
	register(Rule{Clause: "7.21.6 t4", Summary: "a symbolic TrueType font program shall hold exactly one cmap, or a (3,0) one", Check: checkTrueTypeSymbolicCmap})
}

// ttNames is a program's answer to `getGlyphName`: a table of 256 names, every name null, or not known.
type ttNames struct {
	state ttState // ttParsed: table; ttFailed: every code null; ttUnknown: why
	table []string
	why   string
	// symbolic is the program's " " answer — a symbolic font, or one with no /Encoding: null to 7.21.7 (no list holds
	// " "), but NOT `.notdef` to 7.21.8, where a null name asks whether the program holds the code (P07.S04).
	symbolic bool
}

func (n ttNames) equal(o ttNames) bool {
	if n.state != o.state || n.symbolic != o.symbolic {
		return false
	}
	for i := range n.table {
		if n.table[i] != o.table[i] {
			return false
		}
	}
	return true
}

// ttFont is one used simple TrueType font as veraPDF's font object sees it.
type ttFont struct {
	*usedFont
	symbolic bool
	kind     string // the program veraPDF opens: "" (none), "TrueType" (/FontFile2) or "OpenType" (/FontFile3)
	program  trueTypeProgram
	// throws is whether `createCIDToNameTable` throws on THIS font's own encoding (ttParsed = it does not).
	throws ttState
	// embedded is `containsFontFile` for this font's object, the share group applied — ttUnknown where it depends on
	// which font veraPDF opens first.
	embedded ttState
	// subject is whether the group's program is a 7.21.6 t1/t4 subject at all.
	subject ttState
	// names is what the program answers for a code the font's encoding leaves unnamed, the group applied.
	names ttNames
	why   string // why embedded or subject is not ttParsed
}

// trueTypeFonts is the one door every 7.21.6 clause, 7.21.4.1 t1 and the glyph fallback read a TrueType font
// through. An unresolved font refuses, as `type0Fonts` does: it may be a TrueType font nib never saw.
func (d *Document) trueTypeFonts() ([]*ttFont, string) {
	if d.ttDone {
		return d.ttList, d.ttErr
	}
	d.ttDone = true
	used, why := d.usedFonts()
	if why != "" {
		d.ttErr = why
		return nil, why
	}
	type groupKey struct {
		kind     string
		prog     int
		symbolic bool
		enc      string
		subset   bool
	}
	groups := map[groupKey][]*ttFont{}
	var order []groupKey
	d.ttByDict = map[uintptr]*ttFont{}
	for _, uf := range used {
		// **An unresolved font may be a TrueType font nib never saw** — but it must not silence the fonts nib DID read:
		// a definite failure among them outranks this refusal (the P07.S02 re-review's rule; the P07.S03 review measured
		// a symbolic WinAnsi font veraPDF fails on 7.21.4.1 t1 and 7.21.6 t3 turned CannotCheck by an unrelated /F9).
		if uf.unresolve {
			if d.ttUnresolved == "" {
				d.ttUnresolved = fmt.Sprintf("%s: text selects font %s, which does not resolve in nib's reading of its "+
					"resources, so whether it is a TrueType font — and what its program holds — was never read", uf.where, uf.name)
			}
			continue
		}
		if d.name(uf.dict["Subtype"]) != "TrueType" {
			continue
		}
		// One dictionary is one font to veraPDF, however many places nib's walk keyed it under (a direct dictionary is
		// keyed by name and place): two members of one share group would make a single font both first and later.
		if prev := d.ttByDict[dictID(uf.dict)]; prev != nil {
			if !prev.visible && uf.visible {
				prev.where = uf.where // report where it is first drawn visibly, as `usedFonts` does
			}
			prev.visible, prev.hidden = prev.visible || uf.visible, prev.hidden || uf.hidden
			prev.offPage = prev.offPage || uf.offPage
			for m := range uf.modes {
				if prev.modes == nil {
					prev.modes = map[int]bool{}
				}
				prev.modes[m] = true
			}
			continue
		}
		f := &ttFont{usedFont: uf, symbolic: d.isSymbolic(uf.dict)}
		d.ttList = append(d.ttList, f)
		d.ttByDict[dictID(uf.dict)] = f
		obj := d.trueTypeProgramOf(f)
		if f.kind == "" {
			// No program: `getGlyphName` is never asked, so a code the encoding does not name is null.
			f.names = ttNames{state: ttFailed}
			continue
		}
		enc := "direct"
		if ir, ok := uf.dict["Encoding"].(types.IndirectRef); ok {
			enc = fmt.Sprint(ir.ObjectNumber.Value())
		}
		// `FontProgramIDGenerator`: a /FontFile3 program is an OpenType one, whose key adds the subset flag.
		k := groupKey{f.kind, obj, f.symbolic, enc, f.kind == "OpenType" && isSubsetName(d.name(uf.dict["BaseFont"]))}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}
	for _, k := range order {
		d.resolveShareGroup(groups[k])
	}
	return d.ttList, ""
}

// javaLong is Java's `(long)` cast of a double: truncation, NaN to 0, and SATURATION at the ends — Go's conversion of an
// out-of-range float is implementation-defined (MinInt64 on amd64, saturating on arm64), which made /Flags 1e20 read
// non-symbolic on one machine and symbolic on another (the P07.S03 re-review; veraPDF saturates, so it is symbolic).
func javaLong(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// isSubsetName is `PDFont.isSubset`, `name.split("\\+")[0].length() == 6`: whatever precedes the first plus sign — the
// whole name when there is none — is six characters. No capitals are asked for and no plus is needed (the P07.S03
// review measured `/Probe6` and `/abcdef+Two` joining a group nib had split).
func isSubsetName(n string) bool {
	before, _, _ := strings.Cut(n, "+")
	return len(before) == 6
}

// isSymbolic is `PDFontDescriptor.isSymbolic`: /Flags bit 3 and nothing else — bit 6, "nonsymbolic", is never read.
// /Flags is read with `getIntegerKey`, which casts a REAL to a long (measured: `/Flags 4.0` is symbolic), and the bit is
// tested on its `intValue`.
func (d *Document) isSymbolic(font types.Dict) bool {
	var flags int64
	switch v := d.resolve(d.dict(font["FontDescriptor"])["Flags"]).(type) {
	case types.Integer:
		flags = int64(v.Value())
	case types.Float:
		flags = javaLong(v.Value())
	default:
		return false
	}
	return int32(flags)&4 != 0
}

// trueTypeProgramOf reads the program veraPDF opens for f — `PDTrueTypeFont.getFontProgram`: a /FontFile2 stream,
// else a /FontFile3 stream, which is parsed as TrueType whatever its /Subtype says — and returns the stream's object.
func (d *Document) trueTypeProgramOf(f *ttFont) int {
	desc := d.dict(f.dict["FontDescriptor"])
	for _, k := range []struct{ key, kind string }{{"FontFile2", "TrueType"}, {"FontFile3", "OpenType"}} {
		sd, _, err := d.Ctx.DereferenceStreamDict(desc[k.key])
		if err != nil || sd == nil {
			continue
		}
		f.kind = k.kind
		obj := 0
		if ir, ok := desc[k.key].(types.IndirectRef); ok {
			obj = ir.ObjectNumber.Value()
		}
		f.program = d.parseProgramStream(sd)
		return obj
	}
	return 0
}

// parseProgramStream is the one place a TrueType program stream is read — simple and composite fonts alike, since
// `CIDFontType2Program` parses through the same `TrueTypeFontParser`. **One parse per stream, charged to one document
// budget**: fonts sharing a program re-read it otherwise, each with a full budget (the P07.S03 review measured 200 fonts
// on one program at 3.4 s, linear in the fonts).
func (d *Document) parseProgramStream(sd *types.StreamDict) trueTypeProgram {
	if p, done := d.ttPrograms[dictID(sd.Dict)]; done {
		return p
	}
	var p trueTypeProgram
	if sd.Decode() != nil {
		p = trueTypeProgram{state: ttUnknown, why: "its program stream could not be decoded"}
	} else {
		p = readTrueType(sd.Content, maxTrueTypeReads-d.ttReads)
		d.ttReads += p.reads
	}
	if d.ttPrograms == nil {
		d.ttPrograms = map[uintptr]trueTypeProgram{}
	}
	d.ttPrograms[dictID(sd.Dict)] = p
	return p
}

// ownNames is `TrueTypeFontProgram.getGlyphName` for a program built with THIS font's flags and encoding, and
// whether `createCIDToNameTable` throws on it:
//
//   - symbolic, or no /Encoding — the name is " ", which no glyph list holds, so every code is null (measured: a
//     symbolic font with no encoding fails 7.21.7 t1 on every glyph);
//   - the tables did not parse — the table was never built, null;
//   - a NAME encoding: MacRoman or WinAnsi, and anything else throws after allocating an empty table;
//   - a DICTIONARY: its /BaseEncoding read as a NAME (WinAnsi, MacRoman, MacExpert, else Standard — a string
//     base is Standard here, though t2 reads the same string as the encoding), the Differences over it, and every
//     `.notdef` refilled from Standard; a negative Differences code throws an exception veraPDF does not handle;
//   - anything else throws.
func (d *Document) ownNames(f *ttFont) (ttNames, ttState) {
	enc := d.resolve(f.dict["Encoding"])
	if f.symbolic || enc == nil {
		return ttNames{state: ttFailed, symbolic: true}, ttParsed
	}
	if f.program.state == ttUnknown {
		return ttNames{state: ttUnknown, why: f.program.why}, ttUnknown
	}
	if f.program.state == ttFailed {
		return ttNames{state: ttFailed}, ttParsed
	}
	var base []string
	switch v := enc.(type) {
	case types.Name:
		if base = namedEncoding(v.Value()); base == nil || v.Value() == "MacExpertEncoding" {
			return ttNames{state: ttFailed}, ttFailed
		}
		return ttNames{state: ttParsed, table: base}, ttParsed
	case types.Dict:
		b, _ := d.nameOf(v["BaseEncoding"])
		if base = namedEncoding(b); base == nil {
			base = standardEncoding[:]
		}
		table := append([]string(nil), base...)
		for code, name := range d.differences(v) {
			if code < 0 {
				why := "its /Differences names a negative code, where veraPDF throws an exception it does not handle " +
					"and reports nothing for the document"
				return ttNames{state: ttUnknown, why: why}, ttUnknown
			}
			if code < 256 {
				table[code] = name
			}
		}
		// Every `.notdef` is refilled from Standard. The glyph-name fallback never sees it (it asks the table only where
		// the font's own encoding named nothing, i.e. over a Standard base already — P07.S03's red-proof found it inert
		// there), but the per-glyph PRESENCE and WIDTH read the table for every code (P07.S04: a MacExpert base's 0x41
		// is `.notdef` in the encoding and `A` here, and veraPDF finds it present).
		for i, n := range table {
			if n == ".notdef" {
				table[i] = standardEncoding[i]
			}
		}
		return ttNames{state: ttParsed, table: table}, ttParsed
	}
	return ttNames{state: ttFailed}, ttFailed
}

// resolveShareGroup applies veraPDF's program cache to fonts sharing one parse, in first-use order.
func (d *Document) resolveShareGroup(members []*ttFont) {
	own := make([]ttNames, len(members))
	allThrow, noneThrow, unknown := true, true, ""
	for i, f := range members {
		own[i], f.throws = d.ownNames(f)
		switch f.throws {
		case ttUnknown:
			unknown = own[i].why
		case ttFailed:
			noneThrow = false
		default:
			allThrow = false
		}
	}
	tables := members[0].program
	shared := len(members) > 1
	// **/FontFile3 is not /FontFile2 here.** `OpenTypeFontProgram.parseFont` marks the parse successful only after the
	// inner TrueType parse returns, so a name table that throws leaves EVERY font sharing it unparsed, not only the
	// first (read at the P07.S03 review; nib had applied /FontFile2's rule and failed t1 where veraPDF had no subject).
	openType := members[0].kind == "OpenType"
	// **How many fonts share the parse can be unknown** — pdfcpu fuses byte-identical font dictionaries (the optimizer
	// `open` runs), and a font that does not resolve may be one more TrueType font. Where a name table throws, the count
	// decides whether the parse counts.
	if !noneThrow && unknown == "" {
		if d.Ctx.Optimize != nil && len(d.Ctx.Optimize.DuplicateFonts) > 0 {
			unknown = "nib's reader merged identical font dictionaries, so how many fonts share this program in veraPDF's " +
				"reading — which decides whether its parse counts — is not known"
		} else if d.ttUnresolved != "" {
			unknown = "a font nib could not resolve may share this program, which decides whether its parse counts"
		}
	}
	sharedWhy := func(what string) string {
		return fmt.Sprintf("%d fonts share one TrueType program, and %s depends on which of them veraPDF opens first — "+
			"an order nib does not follow", len(members), what)
	}
	const encodingThrows = "its /Encoding is not one veraPDF can build glyph names from (only MacRoman, WinAnsi or an " +
		"encoding dictionary), which makes veraPDF treat the embedded program as unusable"
	namesAgree := true
	for j := range members {
		namesAgree = namesAgree && own[j].equal(own[0])
	}
	// **An unresolved font may be one more member, opened first** — then it decides an OpenType group's parse (a throw
	// leaves every member unparsed) and every group's name table (built from the first font's encoding). A symbolic
	// group's names are " " whoever is first; a non-symbolic group's are not.
	unresolvedFirst := "a font nib could not resolve may share this program and be opened first, which would decide " +
		"whether its parse counts and which encoding names its glyphs"
	for i, f := range members {
		switch {
		case tables.state == ttParsed && unknown == "" && openType && d.ttUnresolved != "":
			f.embedded, f.subject, f.why = ttUnknown, ttUnknown, unresolvedFirst
		case tables.state != ttParsed:
			f.embedded, f.subject, f.why = tables.state, tables.state, tables.why
		case unknown != "":
			f.embedded, f.subject, f.why = ttUnknown, ttUnknown, unknown
		case noneThrow:
			f.embedded, f.subject = ttParsed, ttParsed
		case !shared || (openType && allThrow):
			f.embedded, f.subject, f.why = ttFailed, ttFailed, encodingThrows
		case allThrow:
			// One font — the first opened — is unparsed, and every later one reads the attempted parse's tables.
			// Which one it is matters only when the fonts' visibility differs.
			f.subject, f.embedded = ttParsed, ttParsed
			if i == 0 {
				f.embedded, f.why = ttFailed, encodingThrows
			}
			if !uniformVisibility(members) {
				f.embedded, f.why = ttUnknown, sharedWhy("whether its parse counts")
			}
		case openType:
			// Mixed: the first font's name table decides whether any of them parsed.
			f.subject, f.embedded, f.why = ttUnknown, ttUnknown, sharedWhy("whether its parse counts")
		default:
			f.subject, f.embedded, f.why = ttParsed, ttUnknown, sharedWhy("whether its parse counts")
		}
		f.names = own[i]
		if !namesAgree {
			f.names = ttNames{state: ttUnknown, why: sharedWhy("which encoding names its glyphs")}
		}
		if d.ttUnresolved != "" && !f.symbolic && f.names.state != ttUnknown {
			f.names = ttNames{state: ttUnknown, why: unresolvedFirst}
		}
	}
}

// uniformVisibility reports whether every font in the group is drawn only visibly, or every one only invisibly —
// the case where which font veraPDF opens first cannot change 7.21.4.1 t1.
func uniformVisibility(members []*ttFont) bool {
	allVisible, allHidden := true, true
	for _, f := range members {
		if !f.visible || f.hidden {
			allVisible = false
		}
		if f.visible {
			allHidden = false
		}
	}
	return allVisible || allHidden
}

// trueTypeEncoding is `GFPDSimpleFont.getEncoding` — the NAME for a name, and for a dictionary its /BaseEncoding
// read with `getString`, which a STRING satisfies too (measured: `/BaseEncoding (WinAnsiEncoding)` passes t2).
// Anything else, and an absent entry, is null.
func (d *Document) trueTypeEncoding(font types.Dict) (string, bool) {
	switch v := d.resolve(font["Encoding"]).(type) {
	case types.Name:
		return v.Value(), true
	case types.Dict:
		switch b := d.resolve(v["BaseEncoding"]).(type) {
		case types.Name:
			return b.Value(), true
		case types.StringLiteral, types.HexLiteral:
			if s, ok := d.text(b); ok {
				return s, true
			}
		}
	}
	return "", false
}

func (f *ttFont) label(d *Document) string {
	return fmt.Sprintf("TrueType font %s (%s)", f.name, d.baseFontName(f.dict))
}

// trueTypeClause runs one clause over the fonts: the first failure decides, a refusal waits for one.
func trueTypeClause(d *Document, judge func(*ttFont) (Verdict, string)) Result {
	fonts, why := d.trueTypeFonts()
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	subjects := 0
	var unsure *Result
	for _, f := range fonts {
		v, why := judge(f)
		switch v {
		case NotApplicable:
			continue
		case Fail:
			return Result{Verdict: Fail, Why: why, Where: f.where}
		case CannotCheck:
			if unsure == nil {
				unsure = &Result{Verdict: CannotCheck, Why: why, Where: f.where}
			}
		}
		subjects++
	}
	if unsure != nil {
		return *unsure
	}
	if d.ttUnresolved != "" {
		return Result{Verdict: CannotCheck, Why: d.ttUnresolved}
	}
	if subjects == 0 {
		return Result{Verdict: NotApplicable, Why: "no TrueType font program veraPDF would parse is used by text"}
	}
	return Result{Verdict: Pass}
}

// programClause is t1 and t4's shape: a subject only where veraPDF parsed the program.
func programClause(d *Document, ok func(*ttFont) bool, fail string) Result {
	return trueTypeClause(d, func(f *ttFont) (Verdict, string) {
		switch {
		case f.kind == "" || f.subject == ttFailed:
			return NotApplicable, ""
		case f.subject == ttUnknown:
			return CannotCheck, fmt.Sprintf("%s: whether veraPDF parses its TrueType program is not known — %s", f.label(d), f.why)
		case ok(f):
			return Pass, ""
		}
		return Fail, fmt.Sprintf("%s: %s — its program holds %s%s", f.label(d), fail, subtables(f.program.nrCmaps), cmapPairs(f.program))
	})
}

func subtables(n int) string {
	if n == 1 {
		return "1 cmap subtable"
	}
	return fmt.Sprintf("%d cmap subtables", n)
}

func cmapPairs(p trueTypeProgram) string {
	var s []string
	for _, pe := range [][2]int{{3, 0}, {3, 1}, {1, 0}} {
		if p.has(pe[0], pe[1]) {
			s = append(s, fmt.Sprintf("(%d,%d)", pe[0], pe[1]))
		}
	}
	if len(s) == 0 {
		return ""
	}
	return ", among them " + strings.Join(s, " ")
}

// checkTrueTypeNonSymbolicCmap evaluates ua1 7.21.6 t1: `isSymbolic || (cmap30Present ? nrCmaps > 1 : nrCmaps > 0)`.
// A missing cmap table is zero subtables, and a (3,0) subtable alone does not count.
func checkTrueTypeNonSymbolicCmap(d *Document) Result {
	return programClause(d, func(f *ttFont) bool {
		if f.symbolic {
			return true
		}
		if f.program.has(3, 0) {
			return f.program.nrCmaps > 1
		}
		return f.program.nrCmaps > 0
	}, "a non-symbolic font's program must hold a cmap subtable other than (3,0) for its codes to map to glyphs")
}

// checkTrueTypeSymbolicCmap evaluates ua1 7.21.6 t4: `!isSymbolic || nrCmaps == 1 || cmap30Present`. Every
// subtable record counts, a duplicated pair included.
func checkTrueTypeSymbolicCmap(d *Document) Result {
	return programClause(d, func(f *ttFont) bool {
		return !f.symbolic || f.program.nrCmaps == 1 || f.program.has(3, 0)
	}, "a symbolic font's program must hold exactly one cmap subtable, or a (3,0) one, for its codes to map through a known table")
}

// checkTrueTypeNonSymbolicEncoding evaluates ua1 7.21.6 t2 over every used TrueType font, embedded or not.
//
// `differencesAreUnicodeCompliant` (`GFPDTrueTypeFont`): no program → true; a program that did not parse → false;
// no (3,1) subtable → false; a Differences NAME outside veraPDF's glyph list → false. Only the /Differences
// ARRAY's names are read — a /Differences that is not an array is "contained" and passes.
func checkTrueTypeNonSymbolicEncoding(d *Document) Result {
	return trueTypeClause(d, func(f *ttFont) (Verdict, string) {
		if f.symbolic {
			return Pass, ""
		}
		enc, _ := d.trueTypeEncoding(f.dict)
		if enc != "MacRomanEncoding" && enc != "WinAnsiEncoding" {
			if enc == "" {
				return Fail, f.label(d) + " is non-symbolic and names no MacRomanEncoding or WinAnsiEncoding, so a reader has no standard table to map its codes by"
			}
			return Fail, fmt.Sprintf("%s is non-symbolic and its encoding is %s, not MacRomanEncoding or WinAnsiEncoding", f.label(d), enc)
		}
		encDict, isDict := d.resolve(f.dict["Encoding"]).(types.Dict)
		if !isDict {
			return Pass, ""
		}
		if _, has := encDict["Differences"]; !has {
			return Pass, ""
		}
		if f.kind == "" {
			return Pass, ""
		}
		// A font's own object is unparsed only when its name table throws, which a MacRoman or WinAnsi base never
		// does — so the parse this reads is the tables'.
		switch f.program.state {
		case ttUnknown:
			return CannotCheck, fmt.Sprintf("%s has /Differences, and whether veraPDF parses its TrueType program is not known — %s", f.label(d), f.program.why)
		case ttFailed:
			return Fail, fmt.Sprintf("%s has /Differences, and veraPDF cannot parse its TrueType program (%s), so they are not shown to be Unicode-compliant", f.label(d), f.program.why)
		}
		if f.throws == ttUnknown {
			return CannotCheck, fmt.Sprintf("%s: %s", f.label(d), f.why)
		}
		if !f.program.has(3, 1) {
			return Fail, f.label(d) + " has /Differences and its program has no (3,1) Unicode cmap subtable to map the named glyphs through"
		}
		if arr, ok := d.resolve(encDict["Differences"]).(types.Array); ok {
			for _, o := range arr {
				if n, ok := d.nameOf(o); ok {
					if _, known := adobeGlyphList()[n]; !known {
						return Fail, fmt.Sprintf("%s's /Differences names /%s, which is not in the Adobe Glyph List", f.label(d), n)
					}
				}
			}
		}
		return Pass, ""
	})
}

// checkTrueTypeSymbolicNoEncoding evaluates ua1 7.21.6 t3: `!isSymbolic || Encoding == null`. The encoding is
// `getEncoding`'s — so a symbolic font with an /Encoding DICTIONARY and no /BaseEncoding passes (measured).
func checkTrueTypeSymbolicNoEncoding(d *Document) Result {
	return trueTypeClause(d, func(f *ttFont) (Verdict, string) {
		if !f.symbolic {
			return Pass, ""
		}
		if enc, ok := d.trueTypeEncoding(f.dict); ok {
			return Fail, fmt.Sprintf("%s is symbolic and names the encoding %s, which a symbolic font maps through its own cmap instead", f.label(d), enc)
		}
		return Pass, ""
	})
}

// trueTypeFallbackName is the glyph fallback's question for a TrueType font — `fontProgram.getGlyphName(code)`
// after the font's encoding named nothing. `ok` false means nib does not know what veraPDF answers.
func (d *Document) trueTypeFallbackName(font types.Dict, code int) (name string, known bool, why string) {
	if _, why := d.trueTypeFonts(); why != "" {
		return "", false, why
	}
	f := d.ttByDict[dictID(font)]
	if f == nil {
		// A font drawn only in a pattern or a Type 3 procedure is outside the font population (/pending 678).
		return "", false, "it is drawn only where nib's font population does not reach (/pending 678)"
	}
	switch f.names.state {
	case ttUnknown:
		return "", false, f.names.why
	case ttFailed:
		return "", true, ""
	}
	if code < 0 || code >= len(f.names.table) {
		return "", true, ""
	}
	return f.names.table[code], true, ""
}

// trueTypeEmbedded is 7.21.4.1 t1's question for a TrueType font — `containsFontFile` — and why, when it is not.
func (d *Document) trueTypeEmbedded(font types.Dict) (ttState, string) {
	if _, why := d.trueTypeFonts(); why != "" {
		return ttUnknown, why
	}
	f := d.ttByDict[dictID(font)]
	switch {
	case f == nil:
		return ttUnknown, "the font is outside nib's font population"
	case f.kind == "":
		return ttFailed, ""
	case f.embedded == ttFailed && f.program.state == ttFailed:
		return ttFailed, "veraPDF cannot read its embedded TrueType program (" + f.why + ")"
	}
	return f.embedded, f.why
}
