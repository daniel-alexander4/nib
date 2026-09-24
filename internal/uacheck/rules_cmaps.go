package uacheck

import (
	"fmt"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The composite-font CMap clauses — `PLAN-ua-coverage.md` P07.S01.
//
// All five read a Type 0 font's DICTIONARIES and the CMaps they name, never a glyph: `7.21.3.1 t1` (the CIDFont's
// CIDSystemInfo against the CMap's), `7.21.3.2 t1` (an embedded CIDFontType2 carries a CIDToGIDMap), and `7.21.3.3
// t1`/`t2`/`t3` (a CMap is predefined or embedded, an embedded one's `/WMode` agrees with its program's, and a CMap
// it references is predefined). Each predicate is veraPDF 1.30.2's, read from its source: `GFPDType0Font`,
// `GFPDCIDFont`, `GFPDCmap`, `GFPDReferencedCMap`, `GFCMapFile`, and veraPDF-parser's `PDCMap` and `CMapFile`.

func init() {
	register(Rule{Clause: "7.21.3.1 t1", Summary: "a Type 0 font's CIDFont and CMap shall name compatible character collections", Check: checkCIDSystemInfoCompatible})
	register(Rule{Clause: "7.21.3.2 t1", Summary: "an embedded Type 2 CIDFont shall contain a CIDToGIDMap", Check: checkCIDToGIDMap})
	register(Rule{Clause: "7.21.3.3 t1", Summary: "every CMap shall be a predefined one or be embedded", Check: checkCMapsPredefinedOrEmbedded})
	register(Rule{Clause: "7.21.3.3 t2", Summary: "an embedded CMap's WMode shall agree with its dictionary's", Check: checkCMapWMode})
	register(Rule{Clause: "7.21.3.3 t3", Summary: "a CMap shall reference no CMap but a predefined one", Check: checkReferencedCMaps})
}

// cidSystemInfo is a character collection: Registry, Ordering and Supplement.
type cidSystemInfo struct {
	registry, ordering string
	supplement         int64
}

// predefinedCMaps is ISO 32000-1 Table 118's CMaps and the character collection each belongs to, transcribed
// from veraPDF-parser 1.30.2's `CharacterCollections` (`map.put(...)`, 61 entries). veraPDF keeps three
// collections per name — PDF 1.4, ISO 32000-1, ISO 32000-2 — and PDF/UA-1 reads the ISO 32000-1 column, index 1;
// each line carries the full triple so a reader can check the column. `Identity-H` and `Identity-V` ARE in it, as
// Adobe-Identity-0, so the name list below is exactly this table's keys.
var predefinedCMaps = map[string]cidSystemInfo{
	"GB-EUC-H":         {"Adobe", "GB1", 0},      // index 1 of {ADOBE_GB1_0, ADOBE_GB1_0, ADOBE_GB1_5}
	"GB-EUC-V":         {"Adobe", "GB1", 0},      // index 1 of {ADOBE_GB1_0, ADOBE_GB1_0, ADOBE_GB1_5}
	"GBpc-EUC-H":       {"Adobe", "GB1", 0},      // index 1 of {ADOBE_GB1_0, ADOBE_GB1_0, ADOBE_GB1_5}
	"GBpc-EUC-V":       {"Adobe", "GB1", 0},      // index 1 of {ADOBE_GB1_0, ADOBE_GB1_0, ADOBE_GB1_5}
	"GBK-EUC-H":        {"Adobe", "GB1", 2},      // index 1 of {ADOBE_GB1_2, ADOBE_GB1_2, ADOBE_GB1_5}
	"GBK-EUC-V":        {"Adobe", "GB1", 2},      // index 1 of {ADOBE_GB1_2, ADOBE_GB1_2, ADOBE_GB1_5}
	"GBKp-EUC-H":       {"Adobe", "GB1", 2},      // index 1 of {ADOBE_GB1_2, ADOBE_GB1_2, ADOBE_GB1_5}
	"GBKp-EUC-V":       {"Adobe", "GB1", 2},      // index 1 of {ADOBE_GB1_2, ADOBE_GB1_2, ADOBE_GB1_5}
	"GBK2K-H":          {"Adobe", "GB1", 4},      // index 1 of {ADOBE_GB1_4, ADOBE_GB1_4, ADOBE_GB1_5}
	"GBK2K-V":          {"Adobe", "GB1", 4},      // index 1 of {ADOBE_GB1_4, ADOBE_GB1_4, ADOBE_GB1_5}
	"UniGB-UCS2-H":     {"Adobe", "GB1", 4},      // index 1 of {ADOBE_GB1_4, ADOBE_GB1_4, ADOBE_GB1_5}
	"UniGB-UCS2-V":     {"Adobe", "GB1", 4},      // index 1 of {ADOBE_GB1_4, ADOBE_GB1_4, ADOBE_GB1_5}
	"UniGB-UTF16-H":    {"Adobe", "GB1", 4},      // index 1 of {null, ADOBE_GB1_4, ADOBE_GB1_5}
	"UniGB-UTF16-V":    {"Adobe", "GB1", 4},      // index 1 of {null, ADOBE_GB1_4, ADOBE_GB1_5}
	"B5pc-H":           {"Adobe", "CNS1", 0},     // index 1 of {ADOBE_CNS1_0, ADOBE_CNS1_0, ADOBE_CNS1_7}
	"B5pc-V":           {"Adobe", "CNS1", 0},     // index 1 of {ADOBE_CNS1_0, ADOBE_CNS1_0, ADOBE_CNS1_7}
	"HKscs-B5-H":       {"Adobe", "CNS1", 3},     // index 1 of {ADOBE_CNS1_3, ADOBE_CNS1_3, ADOBE_CNS1_7}
	"HKscs-B5-V":       {"Adobe", "CNS1", 3},     // index 1 of {ADOBE_CNS1_3, ADOBE_CNS1_3, ADOBE_CNS1_7}
	"ETen-B5-H":        {"Adobe", "CNS1", 0},     // index 1 of {ADOBE_CNS1_0, ADOBE_CNS1_0, ADOBE_CNS1_7}
	"ETen-B5-V":        {"Adobe", "CNS1", 0},     // index 1 of {ADOBE_CNS1_0, ADOBE_CNS1_0, ADOBE_CNS1_7}
	"ETenms-B5-H":      {"Adobe", "CNS1", 0},     // index 1 of {ADOBE_CNS1_0, ADOBE_CNS1_0, ADOBE_CNS1_7}
	"ETenms-B5-V":      {"Adobe", "CNS1", 0},     // index 1 of {ADOBE_CNS1_0, ADOBE_CNS1_0, ADOBE_CNS1_7}
	"CNS-EUC-H":        {"Adobe", "CNS1", 0},     // index 1 of {ADOBE_CNS1_0, ADOBE_CNS1_0, ADOBE_CNS1_7}
	"CNS-EUC-V":        {"Adobe", "CNS1", 0},     // index 1 of {ADOBE_CNS1_0, ADOBE_CNS1_0, ADOBE_CNS1_7}
	"UniCNS-UCS2-H":    {"Adobe", "CNS1", 3},     // index 1 of {ADOBE_CNS1_3, ADOBE_CNS1_3, ADOBE_CNS1_7}
	"UniCNS-UCS2-V":    {"Adobe", "CNS1", 3},     // index 1 of {ADOBE_CNS1_3, ADOBE_CNS1_3, ADOBE_CNS1_7}
	"UniCNS-UTF16-H":   {"Adobe", "CNS1", 4},     // index 1 of {null, ADOBE_CNS1_4, ADOBE_CNS1_7}
	"UniCNS-UTF16-V":   {"Adobe", "CNS1", 4},     // index 1 of {null, ADOBE_CNS1_4, ADOBE_CNS1_7}
	"83pv-RKSJ-H":      {"Adobe", "Japan1", 1},   // index 1 of {ADOBE_JAPAN1_1, ADOBE_JAPAN1_1, ADOBE_JAPAN1_7}
	"90ms-RKSJ-H":      {"Adobe", "Japan1", 2},   // index 1 of {ADOBE_JAPAN1_2, ADOBE_JAPAN1_2, ADOBE_JAPAN1_7}
	"90ms-RKSJ-V":      {"Adobe", "Japan1", 2},   // index 1 of {ADOBE_JAPAN1_2, ADOBE_JAPAN1_2, ADOBE_JAPAN1_7}
	"90msp-RKSJ-H":     {"Adobe", "Japan1", 2},   // index 1 of {ADOBE_JAPAN1_2, ADOBE_JAPAN1_2, ADOBE_JAPAN1_7}
	"90msp-RKSJ-V":     {"Adobe", "Japan1", 2},   // index 1 of {ADOBE_JAPAN1_2, ADOBE_JAPAN1_2, ADOBE_JAPAN1_7}
	"90pv-RKSJ-H":      {"Adobe", "Japan1", 1},   // index 1 of {ADOBE_JAPAN1_1, ADOBE_JAPAN1_1, ADOBE_JAPAN1_7}
	"Add-RKSJ-H":       {"Adobe", "Japan1", 1},   // index 1 of {ADOBE_JAPAN1_1, ADOBE_JAPAN1_1, ADOBE_JAPAN1_7}
	"Add-RKSJ-V":       {"Adobe", "Japan1", 1},   // index 1 of {ADOBE_JAPAN1_1, ADOBE_JAPAN1_1, ADOBE_JAPAN1_7}
	"EUC-H":            {"Adobe", "Japan1", 1},   // index 1 of {ADOBE_JAPAN1_1, ADOBE_JAPAN1_1, ADOBE_JAPAN1_7}
	"EUC-V":            {"Adobe", "Japan1", 1},   // index 1 of {ADOBE_JAPAN1_1, ADOBE_JAPAN1_1, ADOBE_JAPAN1_7}
	"Ext-RKSJ-H":       {"Adobe", "Japan1", 2},   // index 1 of {ADOBE_JAPAN1_2, ADOBE_JAPAN1_2, ADOBE_JAPAN1_7}
	"Ext-RKSJ-V":       {"Adobe", "Japan1", 2},   // index 1 of {ADOBE_JAPAN1_2, ADOBE_JAPAN1_2, ADOBE_JAPAN1_7}
	"H":                {"Adobe", "Japan1", 1},   // index 1 of {ADOBE_JAPAN1_1, ADOBE_JAPAN1_1, ADOBE_JAPAN1_7}
	"V":                {"Adobe", "Japan1", 1},   // index 1 of {ADOBE_JAPAN1_1, ADOBE_JAPAN1_1, ADOBE_JAPAN1_7}
	"UniJIS-UCS2-H":    {"Adobe", "Japan1", 4},   // index 1 of {ADOBE_JAPAN1_4, ADOBE_JAPAN1_4, ADOBE_JAPAN1_7}
	"UniJIS-UCS2-V":    {"Adobe", "Japan1", 4},   // index 1 of {ADOBE_JAPAN1_4, ADOBE_JAPAN1_4, ADOBE_JAPAN1_7}
	"UniJIS-UCS2-HW-H": {"Adobe", "Japan1", 4},   // index 1 of {ADOBE_JAPAN1_4, ADOBE_JAPAN1_4, ADOBE_JAPAN1_7}
	"UniJIS-UCS2-HW-V": {"Adobe", "Japan1", 4},   // index 1 of {ADOBE_JAPAN1_4, ADOBE_JAPAN1_4, ADOBE_JAPAN1_7}
	"UniJIS-UTF16-H":   {"Adobe", "Japan1", 5},   // index 1 of {null, ADOBE_JAPAN1_5, ADOBE_JAPAN1_7}
	"UniJIS-UTF16-V":   {"Adobe", "Japan1", 5},   // index 1 of {null, ADOBE_JAPAN1_5, ADOBE_JAPAN1_7}
	"KSC-EUC-H":        {"Adobe", "Korea1", 0},   // index 1 of {ADOBE_KOREA1_0, ADOBE_KOREA1_0, ADOBE_KR_9}
	"KSC-EUC-V":        {"Adobe", "Korea1", 0},   // index 1 of {ADOBE_KOREA1_0, ADOBE_KOREA1_0, ADOBE_KR_9}
	"KSCms-UHC-H":      {"Adobe", "Korea1", 1},   // index 1 of {ADOBE_KOREA1_1, ADOBE_KOREA1_1, ADOBE_KR_9}
	"KSCms-UHC-V":      {"Adobe", "Korea1", 1},   // index 1 of {ADOBE_KOREA1_1, ADOBE_KOREA1_1, ADOBE_KR_9}
	"KSCms-UHC-HW-H":   {"Adobe", "Korea1", 1},   // index 1 of {ADOBE_KOREA1_1, ADOBE_KOREA1_1, ADOBE_KR_9}
	"KSCms-UHC-HW-V":   {"Adobe", "Korea1", 1},   // index 1 of {ADOBE_KOREA1_1, ADOBE_KOREA1_1, ADOBE_KR_9}
	"KSCpc-EUC-H":      {"Adobe", "Korea1", 0},   // index 1 of {ADOBE_KOREA1_0, ADOBE_KOREA1_0, ADOBE_KR_9}
	"UniKS-UCS2-H":     {"Adobe", "Korea1", 1},   // index 1 of {ADOBE_KOREA1_1, ADOBE_KOREA1_1, ADOBE_KR_9}
	"UniKS-UCS2-V":     {"Adobe", "Korea1", 1},   // index 1 of {ADOBE_KOREA1_1, ADOBE_KOREA1_1, ADOBE_KR_9}
	"UniKS-UTF16-H":    {"Adobe", "Korea1", 2},   // index 1 of {null, ADOBE_KOREA1_2, ADOBE_KR_9}
	"UniKS-UTF16-V":    {"Adobe", "Korea1", 2},   // index 1 of {null, ADOBE_KOREA1_2, ADOBE_KR_9}
	"Identity-H":       {"Adobe", "Identity", 0}, // index 1 of {ADOBE_IDENTITY_0, ADOBE_IDENTITY_0, ADOBE_IDENTITY_0}
	"Identity-V":       {"Adobe", "Identity", 0}, // index 1 of {ADOBE_IDENTITY_0, ADOBE_IDENTITY_0, ADOBE_IDENTITY_0}
}

// isPredefinedCMapName is 7.21.3.3's name list — the profile's 61 names, which are exactly the table's keys.
func isPredefinedCMapName(n string) bool {
	_, ok := predefinedCMaps[n]
	return ok
}

// cmapRef is one CMap a used Type 0 font reaches: its `/Encoding`, or one down its `/UseCMap` chain.
type cmapRef struct {
	obj        types.Object // the name or the stream, unresolved
	stream     *types.StreamDict
	name       string // the CMap's name: the name itself, or a stream's /CMapName ("" when it has none)
	referenced bool   // reached through /UseCMap, not /Encoding
	where      string
}

// type0Font is one used Type 0 font with what the CMap clauses read of it.
type type0Font struct {
	dict, cidFont types.Dict
	where         string
	cmaps         []cmapRef
}

// maxUseCMapChain bounds the /UseCMap walk; a chain is keyed once per object, so this only matters for a file
// that writes a long one, and past it the answer is a refusal.
const maxUseCMapChain = 64

// type0Fonts is the population of every CMap clause: the Type 0 fonts the content walk USES — veraPDF builds its
// font objects from the text operators, so a font sitting only in `/Resources` is not a subject — with each
// one's `/Encoding` CMap and the CMaps its `/UseCMap` chain references, each object once.
func (d *Document) type0Fonts() ([]type0Font, string) {
	fonts, why := d.usedFonts()
	if why != "" {
		return nil, why
	}
	var out []type0Font
	for _, uf := range fonts {
		// **A font that does not resolve may be a Type 0 font nib never saw**, and on veraPDF's own corpus it
		// is: pdfcpu's validator DROPS a Type 0 font whose CIDFontType2 lacks a CIDToGIDMap, so on
		// `7.21.3.2-t01-fail-a`/`-c` the text names `/C2_0` and nib's reader has nothing under it — the very font
		// the clause fails. Read as "no Type 0 font", both were false passes; the shipped font rules already
		// refuse an unresolved font, and so does this door. Recovering it needs the raw file (/pending 656).
		if uf.unresolve {
			return nil, fmt.Sprintf("%s: text selects font %s, which does not resolve in nib's reading of its resources, so "+
				"whether it is a Type 0 font — and what its CMaps say — was never read", uf.where, uf.name)
		}
		if uf.dict == nil || d.name(uf.dict["Subtype"]) != "Type0" {
			continue
		}
		f := type0Font{dict: uf.dict, where: uf.where}
		if kids, err := d.Ctx.DereferenceArray(uf.dict["DescendantFonts"]); err == nil && len(kids) > 0 {
			f.cidFont = d.dict(kids[0])
		}
		seen := map[int]bool{}
		obj, referenced := uf.dict["Encoding"], false
		for hop := 0; obj != nil; hop++ {
			if hop >= maxUseCMapChain {
				return nil, fmt.Sprintf("%s: the font's /UseCMap chain runs past %d CMaps and nib stopped reading there", uf.where, maxUseCMapChain)
			}
			if ir, ok := obj.(types.IndirectRef); ok {
				if seen[ir.ObjectNumber.Value()] {
					// A cycle. Not reachable through a file today — pdfcpu's validator recurses on one and the
					// process dies (/pending 675) — and veraPDF throws "Loop inside CMap" and reports nothing;
					// stopping here is what keeps the walk finite the day the reader stops recursing.
					break
				}
				seen[ir.ObjectNumber.Value()] = true
			}
			c := cmapRef{obj: obj, referenced: referenced, where: uf.where}
			var next types.Object
			if n, ok := d.nameOf(obj); ok {
				c.name = n
			} else if sd, _, err := d.Ctx.DereferenceStreamDict(obj); err == nil && sd != nil {
				c.stream = sd
				if n, ok := d.nameOf(sd.Dict["CMapName"]); ok {
					c.name = n
				} else if t, ok := d.text(sd.Dict["CMapName"]); ok {
					c.name = t
				}
				next = sd.Dict["UseCMap"]
			}
			f.cmaps = append(f.cmaps, c)
			obj, referenced = next, true
		}
		out = append(out, f)
	}
	return out, ""
}

// stringKey is veraPDF's `getStringKey`: a string's text, or — since `COSName.getString` answers too — a name's.
func (d *Document) stringKey(dict types.Dict, key string) (string, bool) {
	if dict == nil {
		return "", false
	}
	if s, ok := d.text(dict[key]); ok {
		return s, true
	}
	return d.nameOf(dict[key])
}

// systemInfoOf is a CMap's character collection as veraPDF-parser's `PDCMap.getCIDSystemInfo` reads it: a named
// CMap's from the predefined table (an unknown name has an EMPTY one, not none), a stream's from its
// `/CIDSystemInfo` — a dictionary, or the first element of an array (PDF 1.4's form).
func (d *Document) systemInfoOf(c cmapRef) (types.Dict, bool) {
	if c.stream == nil {
		info, ok := predefinedCMaps[c.name]
		if !ok {
			return types.Dict{}, true
		}
		return types.Dict{"Registry": types.StringLiteral(info.registry), "Ordering": types.StringLiteral(info.ordering),
			"Supplement": types.Integer(info.supplement)}, true
	}
	v := d.resolve(c.stream.Dict["CIDSystemInfo"])
	if arr, ok := v.(types.Array); ok && len(arr) > 0 {
		v = d.resolve(arr[0])
	}
	if dict, ok := v.(types.Dict); ok {
		return dict, true
	}
	return nil, false
}

// checkCIDSystemInfoCompatible evaluates ua1 7.21.3.1 t1, veraPDF's test transcribed:
//
//	cmapName == "Identity-H" || cmapName == "Identity-V" || (CIDFontOrdering != null && CIDFontOrdering == CMapOrdering &&
//	CIDFontRegistry != null && CIDFontRegistry == CMapRegistry && CIDFontSupplement != null && CMapSupplement != null &&
//	CIDFontSupplement <= CMapSupplement)
//
// A missing `/Supplement` is 0 on either side, but a missing CIDSystemInfo is null, which fails.
func checkCIDSystemInfoCompatible(d *Document) Result {
	fonts, why := d.type0Fonts()
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	if len(fonts) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document uses no Type 0 font"}
	}
	for _, f := range fonts {
		var enc cmapRef
		if len(f.cmaps) > 0 {
			enc = f.cmaps[0]
		}
		if enc.name == "Identity-H" || enc.name == "Identity-V" {
			continue
		}
		fontInfo := d.dict(f.cidFont["CIDSystemInfo"])
		fo, foOK := d.stringKey(fontInfo, "Ordering")
		fr, frOK := d.stringKey(fontInfo, "Registry")
		ok := foOK && frOK && fontInfo != nil && len(f.cmaps) > 0
		if ok {
			cmInfo, has := d.systemInfoOf(enc)
			co, coOK := d.stringKey(cmInfo, "Ordering")
			cr, crOK := d.stringKey(cmInfo, "Registry")
			fs, _ := d.intValue(fontInfo["Supplement"])
			cs := int64(0)
			if has && cmInfo != nil {
				if v, isInt := d.intValue(cmInfo["Supplement"]); isInt {
					cs = int64(v)
				}
			}
			ok = has && coOK && crOK && fo == co && fr == cr && int64(fs) <= cs
		}
		if !ok {
			return Result{Verdict: Fail, Where: f.where,
				Why: fmt.Sprintf("the font's CMap %q and its CIDFont name different character collections (or one names none), so a code's "+
					"CID cannot be trusted to mean the glyph the CIDFont holds", enc.name)}
		}
	}
	return Result{Verdict: Pass}
}

// checkCIDToGIDMap evaluates ua1 7.21.3.2 t1: `Subtype != "CIDFontType2" || CIDToGIDMap != null || containsFontFile
// == false`, over each used Type 0 font's descendant CIDFont.
//
// **`containsFontFile` is not the key's presence** — it is `getFontProgram() != null && fontProgramParsed`
// (`GFPDFont.java:171-173`), so veraPDF PASSES a CIDFontType2 whose embedded program it could not parse. nib cannot
// run veraPDF's parser, so it fails only a program its own TrueType reader opens, and refuses the rest.
func checkCIDToGIDMap(d *Document) Result {
	fonts, why := d.type0Fonts()
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	subjects := 0
	var unsure string
	for _, f := range fonts {
		if f.cidFont == nil {
			continue
		}
		subjects++
		if d.name(f.cidFont["Subtype"]) != "CIDFontType2" {
			continue
		}
		m := f.cidFont["CIDToGIDMap"]
		if n, isName := d.nameOf(m); isName && n == "Identity" {
			continue
		}
		if sd, _, err := d.Ctx.DereferenceStreamDict(m); err == nil && sd != nil {
			continue
		}
		desc := d.dict(f.cidFont["FontDescriptor"])
		// **The program is `/FontFile2`, or ANY `/FontFile3`** — veraPDF-parser's `PDCIDFont` reads a CIDFontType2's
		// program from both, whatever the `FontFile3` subtype (`/OpenType`, `/CIDFontType0C`). The review measured
		// each as a false PASS in turn: veraPDF fails a map-less CIDFontType2 carrying its program there, and nib,
		// reading FontFile2 only and then OpenType only, passed it. A program nib's TrueType reader cannot open —
		// a CFF one — is refused below, never "no program".
		//
		// **And the program is parsed as its SUBTYPE says, not as its bytes suggest**, measured: TrueType bytes under
		// `/FontFile3 /CIDFontType0C` are PASSED by veraPDF — it reads them as CFF, fails, and has no parsed program
		// — while nib, opening them as TrueType, failed the clause. So only `/OpenType` goes to nib's sfnt reader;
		// any other `FontFile3` is a CFF program nib does not parse, and whether veraPDF parses it decides the clause.
		prog, _, err := d.Ctx.DereferenceStreamDict(desc["FontFile2"])
		if err != nil || prog == nil {
			if ff3, _, err3 := d.Ctx.DereferenceStreamDict(desc["FontFile3"]); err3 == nil && ff3 != nil {
				if sub := d.name(ff3.Dict["Subtype"]); sub != "OpenType" {
					if unsure == "" {
						unsure = fmt.Sprintf("%s: the CIDFont's program is a /FontFile3 /%s, a CFF program nib does not parse; "+
							"veraPDF fails this clause only if IT parses it, so nib cannot say which way it goes", f.where, sub)
					}
					continue
				}
				prog = ff3
			}
		}
		if prog == nil {
			continue // no program: containsFontFile is false, and the test passes
		}
		if derr := prog.Decode(); derr != nil {
			if unsure == "" {
				unsure = fmt.Sprintf("%s: the CIDFont's embedded font program could not be decoded, so whether veraPDF parses it — "+
					"which decides this clause — was never established: %v", f.where, derr)
			}
			continue
		}
		if _, perr := trueTypeGlyphCount(prog.Content); perr != nil {
			if unsure == "" {
				unsure = fmt.Sprintf("%s: nib's TrueType reader does not open the CIDFont's font program (%v); veraPDF passes this clause "+
					"for a program it cannot parse, so nib cannot say which way it goes", f.where, perr)
			}
			continue
		}
		return Result{Verdict: Fail, Where: f.where,
			Why: "an embedded Type 2 CIDFont has no CIDToGIDMap, so nothing says which glyph each CID draws"}
	}
	if unsure != "" {
		return Result{Verdict: CannotCheck, Why: unsure}
	}
	if subjects == 0 {
		return Result{Verdict: NotApplicable, Why: "the document uses no CIDFont"}
	}
	return Result{Verdict: Pass}
}

// checkCMapsPredefinedOrEmbedded evaluates ua1 7.21.3.3 t1 over every CMap — a font's `/Encoding` and every CMap
// down its `/UseCMap` chain, since veraPDF's referenced CMap is a CMap too: predefined by name, or embedded.
func checkCMapsPredefinedOrEmbedded(d *Document) Result {
	return d.checkCMaps(false, "a CMap that is neither one of ISO 32000-1's predefined CMaps nor embedded — a reader has no way to know what it maps")
}

// checkReferencedCMaps evaluates ua1 7.21.3.3 t3: a CMap reached through `/UseCMap` is predefined, embedded or not.
func checkReferencedCMaps(d *Document) Result {
	return d.checkCMaps(true, "a CMap references another that is not one of ISO 32000-1's predefined CMaps")
}

func (d *Document) checkCMaps(referencedOnly bool, why string) Result {
	fonts, err := d.type0Fonts()
	if err != "" {
		return Result{Verdict: CannotCheck, Why: err}
	}
	subjects := 0
	for _, f := range fonts {
		for _, c := range f.cmaps {
			if referencedOnly && !c.referenced {
				continue
			}
			subjects++
			if isPredefinedCMapName(c.name) || (!referencedOnly && c.stream != nil) {
				continue
			}
			return Result{Verdict: Fail, Where: c.where, Why: fmt.Sprintf("%s (/%s)", why, c.name)}
		}
	}
	if subjects == 0 {
		why := "the document uses no Type 0 font, so it has no CMap"
		if referencedOnly {
			why = "the document uses no Type 0 font whose CMap references another"
		}
		return Result{Verdict: NotApplicable, Why: why}
	}
	return Result{Verdict: Pass}
}

// checkCMapWMode evaluates ua1 7.21.3.3 t2, `WMode == dictWMode`, over every EMBEDDED CMap: the stream dictionary's
// `/WMode` (0 when absent) against the `/WMode <n> def` the CMap program itself declares (0 when it declares none),
// as veraPDF-parser's `CMapFile.getDictWMode` and `CMapParser`'s user-dictionary read.
func checkCMapWMode(d *Document) Result {
	fonts, err := d.type0Fonts()
	if err != "" {
		return Result{Verdict: CannotCheck, Why: err}
	}
	subjects := 0
	for _, f := range fonts {
		for _, c := range f.cmaps {
			if c.stream == nil {
				continue
			}
			subjects++
			dictW := int64(0)
			if v, ok := d.intValue(c.stream.Dict["WMode"]); ok {
				dictW = int64(v)
			}
			if derr := c.stream.Decode(); derr != nil {
				return Result{Verdict: CannotCheck, Where: c.where, Why: "an embedded CMap could not be decoded: " + derr.Error()}
			}
			progW, ok := cmapProgramWMode(c.stream.Content)
			if !ok {
				return Result{Verdict: CannotCheck, Where: c.where, Why: "an embedded CMap's /WMode is not an integer nib can read"}
			}
			if progW != dictW {
				return Result{Verdict: Fail, Where: c.where, Why: fmt.Sprintf("an embedded CMap's program declares /WMode %d while its "+
					"dictionary says %d, so writing direction depends on which a reader believes", progW, dictW)}
			}
		}
	}
	if subjects == 0 {
		return Result{Verdict: NotApplicable, Why: "the document uses no embedded CMap"}
	}
	return Result{Verdict: Pass}
}

// cmapProgramWMode scans a CMap program's PostScript for `/WMode <int> def`, the last one winning as a later
// `def` replaces an earlier; 0 when there is none. `ok` is false when the value after `/WMode` is not an integer.
//
// **Only at procedure depth 0**: a `{ /WMode 1 def }` is a procedure body that never runs, and veraPDF passes a CMap
// carrying one where a flat scan failed it (the slice review, measured). A non-integer value (`1.0`, `(x)`) is a
// refusal rather than veraPDF's own answer, which differs by type and is not transcribed.
func cmapProgramWMode(prog []byte) (int64, bool) {
	toks := psTokens(prog)
	w := int64(0)
	depth := 0
	for i := 0; i < len(toks); i++ {
		switch toks[i] {
		case "{":
			depth++
			continue
		case "}":
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth > 0 || i+2 >= len(toks) || toks[i] != "/WMode" || toks[i+2] != "def" {
			continue
		}
		v, err := strconv.ParseInt(toks[i+1], 10, 64)
		if err != nil {
			return 0, false
		}
		w = v
	}
	if depth > 0 {
		// **A procedure never closed makes veraPDF's CMap parser throw, and the throw discards every `def` it had
		// read**, leaving the default 0 (the review, measured: `/WMode 1 def {` passes under a dictionary with no
		// /WMode). So an unbalanced program answers 0, as veraPDF does.
		return 0, true
	}
	return w, true
}

// psTokens splits PostScript into tokens, dropping comments and keeping `/names`, numbers, operators and the
// delimiters `[ ] { } << >>` as their own tokens; strings `(...)` and `<hex>` are kept whole so their bytes never
// read as a `/WMode`.
func psTokens(b []byte) []string {
	var out []string
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == '%':
			for i < len(b) && b[i] != '\n' && b[i] != '\r' {
				i++
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == 0:
			i++
		case c == '(':
			depth, j := 0, i
			for ; j < len(b); j++ {
				if b[j] == '\\' {
					j++
					continue
				}
				if b[j] == '(' {
					depth++
				} else if b[j] == ')' {
					if depth--; depth == 0 {
						break
					}
				}
			}
			out = append(out, "(string)")
			i = j + 1
		case c == '<' && i+1 < len(b) && b[i+1] == '<', c == '>' && i+1 < len(b) && b[i+1] == '>':
			out = append(out, string(b[i:i+2]))
			i += 2
		case c == '<':
			j := i
			for j < len(b) && b[j] != '>' {
				j++
			}
			out = append(out, "<hex>")
			i = j + 1
		case c == '[' || c == ']' || c == '{' || c == '}':
			out = append(out, string(c))
			i++
		default:
			j := i + 1
			for j < len(b) && !isPSDelim(b[j]) {
				j++
			}
			out = append(out, string(b[i:j]))
			i = j
		}
	}
	return out
}

func isPSDelim(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', 0, '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}
