package uacheck

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"nib/internal/contentstream"
)

// Language — `PLAN-ua-coverage.md` P04. Each rule is veraPDF's own property, read from its source
// (veraPDF-validation `GFPDDocument`, `GFPDStructElem`, `GFPDAnnot`, `GFPDFormField`, `GFOpMarkedContent` and the
// profile's `gContainsCatalogLang` variable) and then measured on veraPDF 1.30.2 before it was written.

func init() {
	register(Rule{Clause: "7.2 t2", Summary: "natural language in the outline entries shall be determined", Check: checkOutlineLanguage})
	register(Rule{Clause: "7.2 t29", Summary: "a /Lang value shall be a language identifier (ISO 32000-1 14.9.2)", Check: checkLanguageIdentifiers})
}

// catalogLang is the catalog's `/Lang` and whether it is a string — the one place the catalog's language is read.
//
// It is veraPDF's `gContainsCatalogLang` (the profile variable, set from `GFPDDocument.getcontainsLang`): the key
// is present and a string. **Measured on 1.30.2: an empty `()` counts, an indirect string counts, a name does not.**
// Every rule that asks whether the catalog determines a language asks here (`catalogDeclaresLang`), and
// `TestTheCatalogLanguageIsReadThroughOneDoor` holds that nothing reads the key another way.
func (d *Document) catalogLang() (string, bool) {
	return d.text(d.Catalog["Lang"])
}

// checkOutlineLanguage evaluates ua1 7.2 t2. veraPDF's subject is each outline item (`PDOutline`) and its test is
// the catalog variable alone, so the verdict is "is there an item, and does the catalog determine a language".
// Measured: no catalog /Lang fails, `()` passes, `/en` fails, an indirect string passes; no item is no subject.
// The `/First` half cannot be reached through nib's reader today — pdfcpu's validator drops an `/Outlines` whose
// `/First` is missing or dangling (both measured in the slice review's fix pass) — and it stays as veraPDF's
// subject definition, so a reader that kept such a dictionary would still answer "no entries", not fail.
func checkOutlineLanguage(d *Document) Result {
	outlines := d.dict(d.Catalog["Outlines"])
	if outlines == nil || d.dict(outlines["First"]) == nil {
		return Result{Verdict: NotApplicable, Why: "the document has no outline entries"}
	}
	if catalogDeclaresLang(d) {
		return Result{Verdict: Pass}
	}
	return Result{Verdict: Fail, Where: "catalog /Outlines",
		Why: "the document has outline entries and the catalog declares no /Lang, so their language cannot be determined"}
}

// languageTag is ISO 32000-1 14.9.2's language identifier as veraPDF tests it (`CosLang`, 7.2-29), over the
// decoded value: measured, `()`, `en_US`, `en-`, a nine-letter subtag, `1en` and `en US` fail; `x-klingon` and a
// UTF-16 `en-US` pass.
var languageTag = regexp.MustCompile(`^[a-zA-Z]{1,8}(-[a-zA-Z0-9]{1,8})*$`)

// checkLanguageIdentifiers evaluates ua1 7.2 t29 over veraPDF's CosLang population — every STRING `/Lang` on the
// catalog, on a structure element, on the element an annotation's or a form field's `/StructParent` names (measured
// with an element reachable ONLY that way), and on a BDC property list (inline, or named in `/Properties` and
// used). A `/Lang` that is not a string is no subject, a property list defined but never used is none, and a DP's
// property list is none either — veraPDF 1.30.2 passes a bad `/Lang` there (measured, `recordLang`).
//
// **Marked content inside a tiling pattern or a Type 3 glyph procedure is read by veraPDF when it is used, and
// nib's content walk enters neither** (measured, P04.S01's grill). So a failing `/Lang` found only in such a
// stream is CannotCheck — nib cannot tell a used pattern from an unused one without walking it — and a valid one
// changes nothing. P04.S04 owns walking them.
func checkLanguageIdentifiers(d *Document) Result {
	subjects := 0
	check := func(value, where string) *Result {
		subjects++
		if languageTag.MatchString(value) {
			return nil
		}
		return &Result{Verdict: Fail, Where: where,
			Why: fmt.Sprintf("the /Lang value %q is not a language identifier (a primary tag of 1-8 letters, then subtags of 1-8 letters or digits, joined by hyphens)", value)}
	}
	if v, ok := d.catalogLang(); ok {
		if r := check(v, "catalog /Lang"); r != nil {
			return *r
		}
	}
	nodes, unread := d.structNodes()
	for _, n := range nodes {
		if v, ok := d.text(n.dict["Lang"]); ok {
			if r := check(v, nodeWhere(n, d.name(n.dict["S"]))); r != nil {
				return *r
			}
		}
	}
	parents, ptWhy := d.structParentsOfAnnotsAndFields()
	for _, sp := range parents {
		if v, ok := d.text(sp.elem["Lang"]); ok {
			if r := check(v, sp.where); r != nil {
				return *r
			}
		}
	}
	_, contentWhy := d.contentEvents()
	subjects += d.mcLangCount
	if b := d.mcLangBad; b != nil {
		return Result{Verdict: Fail, Where: b.where,
			Why: fmt.Sprintf("the /Lang value %q is not a language identifier (a primary tag of 1-8 letters, then subtags of 1-8 letters or digits, joined by hyphens)", b.value)}
	}
	switch {
	case unread != "":
		return Result{Verdict: CannotCheck, Why: unread}
	case ptWhy != "":
		return Result{Verdict: CannotCheck, Why: ptWhy}
	case contentWhy != "":
		return Result{Verdict: CannotCheck, Why: contentWhy}
	}
	where, value, why := d.unwalkedBadLang()
	switch {
	case why != "":
		return Result{Verdict: CannotCheck, Why: why}
	case where != "":
		return Result{Verdict: CannotCheck, Where: where,
			Why: fmt.Sprintf("a marked-content /Lang %q that is not a language identifier sits in a stream nib does not walk "+
				"(a tiling pattern, a Type 3 glyph, or a form one of those draws); veraPDF fails it when that stream is drawn, "+
				"and nib cannot tell whether it is", value)}
	}
	if subjects == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no /Lang entry"}
	}
	return Result{Verdict: Pass}
}

// structParentSite is a structure element reached from an annotation's or a field's `/StructParent`.
type structParentSite struct {
	elem  types.Dict
	where string
}

// structParentsOfAnnotsAndFields is the element every annotation on every page, and every form field in the
// AcroForm tree, names through its `/StructParent` and the parent tree — veraPDF's `GFPDAnnot.getLang` and
// `GFPDFormField.getLang` read the `/Lang` there. The second result is why part could not be read.
func (d *Document) structParentsOfAnnotsAndFields() ([]structParentSite, string) {
	pt, ptWhy := d.parentTree()
	var out []structParentSite
	missed := ""
	add := func(holder types.Dict, where string) {
		sp, ok := d.intValue(holder["StructParent"])
		if !ok {
			return
		}
		if elem := d.dict(pt[sp]); elem != nil {
			out = append(out, structParentSite{elem: elem, where: where + " → its /StructParent element"})
		} else if ptWhy != "" && missed == "" {
			missed = ptWhy
		}
	}
	for p := 1; p <= d.Ctx.PageCount; p++ {
		page, _, _, err := d.Ctx.PageDict(p, false)
		if err != nil || page == nil {
			return out, fmt.Sprintf("page %d does not resolve", p)
		}
		annots, aerr := d.Ctx.DereferenceArray(page["Annots"])
		if aerr != nil {
			return out, fmt.Sprintf("page %d's /Annots could not be read: %v", p, aerr)
		}
		for i, a := range annots {
			if ad := d.dict(a); ad != nil {
				add(ad, fmt.Sprintf("page %d, annotation %d", p, i))
			}
		}
	}
	seen := map[uintptr]bool{}
	var fields func(o types.Object, depth int)
	fields = func(o types.Object, depth int) {
		arr, err := d.Ctx.DereferenceArray(o)
		if err != nil || arr == nil {
			return
		}
		for _, f := range arr {
			fd := d.dict(f)
			if fd == nil || seen[dictID(fd)] {
				continue
			}
			if depth > maxWalkDepth {
				if missed == "" {
					missed = fmt.Sprintf("the form field tree nests deeper than %d levels; nib stops reading there", maxWalkDepth)
				}
				return
			}
			seen[dictID(fd)] = true
			add(fd, "form field")
			fields(fd["Kids"], depth+1)
		}
	}
	if acro := d.dict(d.Catalog["AcroForm"]); acro != nil {
		fields(acro["Fields"], 0)
	}
	return out, missed
}

// mcLang is a string `/Lang` on a BDC property list the content walk reached — kept only for the first that fails.
type mcLang struct {
	value, where string
}

// propertyListLang is a marked-content property list's `/Lang` and whether it is a string: inline
// (`/Span << /Lang (en-US) >> BDC`) or named and resolved through `/Resources /Properties` (`/Span /P0 BDC`).
// veraPDF reads it as `getAttribute(LANG, COS_STRING)` on the property list, so a name is no answer.
func (d *Document) propertyListLang(src []byte, operands []contentstream.Token, res types.Dict) (string, bool) {
	for i, tk := range operands {
		if tk.Kind != contentstream.Operand || string(tk.Bytes(src)) != "/Lang" || i+1 >= len(operands) {
			continue
		}
		v := operands[i+1].Bytes(src)
		switch {
		case len(v) >= 2 && v[0] == '(' && v[len(v)-1] == ')':
			return d.text(types.StringLiteral(v[1 : len(v)-1]))
		case len(v) >= 2 && v[0] == '<' && v[len(v)-1] == '>':
			return d.text(types.HexLiteral(v[1 : len(v)-1]))
		}
		return "", false
	}
	names := 0
	for _, tk := range operands {
		if tk.Kind != contentstream.Operand {
			continue
		}
		b := tk.Bytes(src)
		if len(b) == 0 || b[0] != '/' {
			continue
		}
		names++
		if names < 2 {
			continue
		}
		if props := d.dict(d.dict(res["Properties"])[string(b[1:])]); props != nil {
			return d.text(props["Lang"])
		}
	}
	return "", false
}

// unwalkedBadLang looks for marked content veraPDF reads and nib's content walk does not — the streams of tiling
// patterns and Type 3 glyph procedures, and every form XObject those draw — for a BDC property-list `/Lang` that
// fails the grammar. It answers where and what, or why it could not finish looking.
//
// **It walks the resource graph, not the object table** (the slice review, measured: a Type 3 font written directly
// in `/Resources` has no object number, and a form drawn only from inside a pattern is reached by nothing else, and
// veraPDF fails both). From every page's resources and every appearance stream's, it follows `/Pattern`, `/Font`
// (Type 3) and `/XObject` (forms) through each stream's own `/Resources`, once per dictionary, under
// `maxUnwalkedStreams`. It reads what is DEFINED, not what is drawn, which is why a hit is only ever CannotCheck.
// A form the content walk also drew is scanned again here; a bad value there would already have failed.
func (d *Document) unwalkedBadLang() (where, value, why string) {
	seen := map[uintptr]bool{}
	streams := 0
	var queue []types.Dict
	var labels []string
	push := func(res types.Dict, label string) {
		if res != nil && !seen[dictID(res)] {
			seen[dictID(res)] = true
			queue = append(queue, res)
			labels = append(labels, label)
		}
	}
	scan := func(sd *types.StreamDict, fallback types.Dict, label string) (string, string, string) {
		if streams++; streams > maxUnwalkedStreams {
			return "", "", fmt.Sprintf("the patterns, Type 3 glyphs and forms nib does not walk hold more than %d streams; "+
				"nib stops reading them there", maxUnwalkedStreams)
		}
		if err := sd.Decode(); err != nil {
			return "", "", fmt.Sprintf("%s could not be decoded, so its marked content was never read: %v", label, err)
		}
		res := d.dict(sd.Dict["Resources"])
		if res == nil {
			res = fallback
		}
		push(res, label)
		src := sd.Content
		var operands []contentstream.Token
		for _, tk := range contentstream.Tokenize(src) {
			switch tk.Kind {
			case contentstream.Whitespace:
				continue
			case contentstream.Operator:
			default:
				operands = append(operands, tk)
				continue
			}
			if string(tk.Bytes(src)) == "BDC" {
				if v, ok := d.propertyListLang(src, operands, res); ok && !languageTag.MatchString(v) {
					return label, v, ""
				}
			}
			operands = operands[:0]
		}
		return "", "", ""
	}
	stream := func(o types.Object, label string) (*types.StreamDict, string) {
		sd, _, err := d.Ctx.DereferenceStreamDict(o)
		if err != nil || sd == nil {
			return nil, fmt.Sprintf("%s does not resolve to a stream, so its marked content was never read", label)
		}
		return sd, ""
	}
	for p := 1; p <= d.Ctx.PageCount; p++ {
		page, _, _, err := d.Ctx.PageDict(p, false)
		if err != nil || page == nil {
			return "", "", fmt.Sprintf("page %d does not resolve", p)
		}
		push(d.resourcesOf(page), fmt.Sprintf("page %d", p))
		annots, _ := d.Ctx.DereferenceArray(page["Annots"])
		for i, a := range annots {
			ap := d.dict(d.dict(a)["AP"])
			for _, key := range []string{"N", "R", "D"} {
				for state, so := range d.appearanceStreams(ap[key]) {
					if sd, _, err := d.Ctx.DereferenceStreamDict(so); err == nil && sd != nil {
						push(d.dict(sd.Dict["Resources"]), fmt.Sprintf("page %d, annotation %d, appearance /%s %s", p, i, key, state))
					}
				}
			}
		}
	}
	for i := 0; i < len(queue); i++ {
		res, at := queue[i], labels[i]
		for _, name := range sortedKeys(d.dict(res["Pattern"])) {
			o := d.dict(res["Pattern"])[name]
			label := fmt.Sprintf("%s → pattern /%s", at, name)
			if sd, _, err := d.Ctx.DereferenceStreamDict(o); err == nil && sd != nil {
				if pt, ok := d.intValue(sd.Dict["PatternType"]); ok && pt == 1 {
					if w, v, why := scan(sd, res, label); w != "" || why != "" {
						return w, v, why
					}
				}
			}
		}
		for _, name := range sortedKeys(d.dict(res["Font"])) {
			font := d.dict(d.dict(res["Font"])[name])
			if font == nil {
				// **A font entry nib's reader did not keep — or a `null` in the file**, and the validated context
				// cannot say which (measured: a Type 3 font written directly in `/Resources` survives pdfcpu's
				// `ReadContext` and is nil after the validator `open` runs; two corpus files carry a genuine `null`,
				// which veraPDF reads as no font). So the raw file is asked, once, whether it writes a Type 3 font inline.
				if d.hasInlineType3Font() {
					return "", "", fmt.Sprintf("%s → font /%s is missing from what nib's reader kept, and the file writes "+
						"a Type 3 font directly inside a dictionary, which pdfcpu's validator drops — so its glyphs were never read", at, name)
				}
				continue
			}
			if d.name(font["Subtype"]) != "Type3" {
				continue
			}
			fontRes := d.dict(font["Resources"])
			if fontRes == nil {
				fontRes = res
			}
			procs := d.dict(font["CharProcs"])
			for _, glyph := range sortedKeys(procs) {
				label := fmt.Sprintf("%s → Type 3 font /%s, glyph /%s", at, name, glyph)
				sd, why := stream(procs[glyph], label)
				if why != "" {
					return "", "", why
				}
				if w, v, why := scan(sd, fontRes, label); w != "" || why != "" {
					return w, v, why
				}
			}
		}
		for _, name := range sortedKeys(d.dict(res["XObject"])) {
			o := d.dict(res["XObject"])[name]
			if sd, _, err := d.Ctx.DereferenceStreamDict(o); err == nil && sd != nil && d.name(sd.Dict["Subtype"]) == "Form" {
				if w, v, why := scan(sd, res, fmt.Sprintf("%s → form /%s", at, name)); w != "" || why != "" {
					return w, v, why
				}
			}
		}
	}
	return "", "", ""
}

// maxUnwalkedStreams bounds unwalkedBadLang's reading, as P03's budgets bound the content walk's.
const maxUnwalkedStreams = 1 << 14

// sortedKeys is a dictionary's keys in order, so a first hit is reported the same way every run.
func sortedKeys(d types.Dict) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// hasInlineType3Font reports whether the file, parsed WITHOUT validation, holds a Type 3 font dictionary written
// inline — nested in another object rather than an object of its own — which is the shape pdfcpu's validator
// drops (see `unwalkedBadLang`). Asked only when a font entry is missing, and memoised; a file that will not parse
// raw answers true, because nib then cannot rule the dropped font out.
func (d *Document) hasInlineType3Font() bool {
	if d.directType3 != nil {
		return *d.directType3
	}
	found := d.scanInlineType3()
	d.directType3 = &found
	return found
}

func (d *Document) scanInlineType3() bool {
	ctx, err := api.ReadContext(bytes.NewReader(d.raw), model.NewDefaultConfiguration())
	if err != nil {
		return true
	}
	rd := &Document{Ctx: ctx} // the typed-value door over the raw parse
	var walk func(o types.Object, nested bool, depth int) bool
	walk = func(o types.Object, nested bool, depth int) bool {
		if depth > maxWalkDepth {
			return false
		}
		switch v := o.(type) {
		case types.Dict:
			if nested {
				if rd.name(v["Subtype"]) == "Type3" {
					return true
				}
			}
			for _, x := range v {
				if walk(x, true, depth+1) {
					return true
				}
			}
		case types.StreamDict:
			return walk(v.Dict, false, depth+1)
		case types.Array:
			for _, x := range v {
				if walk(x, true, depth+1) {
					return true
				}
			}
		}
		return false
	}
	for nr := range ctx.XRefTable.Table {
		o, err := ctx.Dereference(types.IndirectRef{ObjectNumber: types.Integer(nr)})
		if err == nil && walk(o, false, 0) {
			return true
		}
	}
	return false
}
