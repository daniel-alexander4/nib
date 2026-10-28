package uacheck

import (
	"errors"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// mistypedTextKeys are the four entries pdfcpu's validator refuses a whole document over when the value is not a
// string, and veraPDF reads (`/pending 612`): on the catalog, `/Lang`; on a structure element, all four.
var mistypedTextKeys = []string{"Lang", "Alt", "ActualText", "E"}

// maxAsideDepth bounds the structure walk below. It is the validator's own nesting that matters, and a tree deeper
// than this is refused by the rules' own walk (`structNodes`) before any of them reads an element.
const maxAsideDepth = 4096

// setAsideForValidator takes out of ctx, for pdfcpu's validation only, every `/Lang`, `/Alt`, `/ActualText` and `/E`
// that does not hold a string — on the catalog and on each structure element — and returns what puts them back.
//
// **Why.** pdfcpu refuses the document outright ("decodeString: dict=StructElementDict entry=Alt invalid type
// types.Name"), so nib answered "the document could not be read" about a file veraPDF reads and grades. Measured on
// veraPDF 1.30.2 over 45 documents: a name, a number, a boolean, an array or a dictionary on any of the four reads
// exactly as the key being ABSENT — every clause answers as it does for the same document without the key — with one
// exception, a NAME on `/ActualText`, which veraPDF reads as text (`Document.actualText`).
//
// **Set aside and put back, not rewritten.** The rules read the value the document wrote, through the doors that
// already answer "not a string" (`d.text`, `declaresLang`); nothing here decides what a wrong type means. The walk
// follows `/K` from the structure tree root, where the validator finds its elements, and stops at what is not a
// structure element (a marked-content or object reference names none of the four).
func setAsideForValidator(ctx *model.Context) (restore func()) {
	type held struct {
		dict types.Dict
		key  string
		val  types.Object
	}
	var taken []held
	strip := func(d types.Dict) {
		for _, key := range mistypedTextKeys {
			v, ok := d[key]
			if !ok {
				continue
			}
			r, err := ctx.Dereference(v)
			if err != nil || r == nil {
				continue // a dangling reference: the validator accepts it, and `d.text` reads it as absent
			}
			switch r.(type) {
			case types.StringLiteral, types.HexLiteral:
				continue
			}
			taken = append(taken, held{d, key, v})
			delete(d, key)
		}
	}
	root, err := ctx.XRefTable.Catalog()
	if err == nil && root != nil {
		if v, ok := root["Lang"]; ok {
			if r, derr := ctx.Dereference(v); derr == nil && r != nil {
				switch r.(type) {
				case types.StringLiteral, types.HexLiteral:
				default:
					taken = append(taken, held{root, "Lang", v})
					delete(root, "Lang")
				}
			}
		}
		seen := map[int]bool{}
		var walk func(o types.Object, depth int)
		walk = func(o types.Object, depth int) {
			if depth > maxAsideDepth {
				return
			}
			if ir, ok := o.(types.IndirectRef); ok {
				n := ir.ObjectNumber.Value()
				if seen[n] {
					return
				}
				seen[n] = true
			}
			r, derr := ctx.Dereference(o)
			if derr != nil || r == nil {
				return
			}
			switch v := r.(type) {
			case types.Array:
				for _, k := range v {
					walk(k, depth+1)
				}
			case types.Dict:
				// Resolved, as every typed read is: there is no Document yet to ask, and an indirect /Type is legal.
				typ, _ := ctx.Dereference(v["Type"])
				switch t := typ.(type) {
				case types.Name:
					if t == "MCR" || t == "OBJR" {
						return
					}
				}
				strip(v)
				walk(v["K"], depth+1)
			}
		}
		if tree, derr := ctx.Dereference(root["StructTreeRoot"]); derr == nil {
			if td, ok := tree.(types.Dict); ok {
				walk(td["K"], 0)
			}
		}
	}
	// The two fonts the validator takes away rather than refuses (`/pending 857`, ADR-116) — see `fontsTheValidatorLoses`.
	for _, h := range fontsTheValidatorLoses(ctx) {
		taken = append(taken, held{h.dict, h.key, h.val})
		delete(h.dict, h.key)
	}
	// And three entries v0.16.2's validator refuses a whole document over that veraPDF reads (`entriesTheValidatorRefuses`).
	for _, h := range entriesTheValidatorRefuses(ctx) {
		taken = append(taken, held{h.dict, h.key, h.val})
		if h.with != nil {
			h.dict[h.key] = h.with
		} else {
			delete(h.dict, h.key)
		}
	}
	return func() {
		for _, h := range taken {
			h.dict[h.key] = h.val
		}
	}
}

// actualText reads an `/ActualText` as veraPDF does: `getKey(…).getString()`, which answers for a string AND for a
// name — where `/Alt`, `/E` and `/Lang` go through `getStringKey`, which answers for a string only (`d.text`).
// Measured (`/pending 612`, `/pending 633`): a Figure whose only alternate text is `/ActualText /a` PASSES 7.3 t1 and,
// with no language, FAILS 7.2 t21; a number, a boolean, an array, a dictionary and `null` there all read as absent.
func (d *Document) actualText(obj types.Object) (string, bool) {
	if s, ok := d.text(obj); ok {
		return s, true
	}
	return d.nameOf(obj)
}

// asideEntry is one dictionary entry set aside for the validator.
type asideEntry struct {
	dict types.Dict
	key  string
	val  types.Object
	// with, when set, stands in for val during the validation instead of the entry being taken out: a required
	// entry cannot be absent, only made to satisfy the validator (`entriesTheValidatorRefuses`).
	with types.Object
}

// maxAsideNesting bounds how far the search below follows DIRECT objects inside one another; every indirect object
// is its own starting point, so the bound is on nesting within one object and not on the document.
const maxAsideNesting = 64

// fontsTheValidatorLoses finds the two font entries pdfcpu's validator takes out of a document veraPDF reads
// (`/pending 857`), so `setAsideForValidator` can carry them across it:
//
//   - **a Type 3 font written DIRECTLY in a `/Font` dictionary.** The validator removes it, and every rule that
//     reads the page's content refused (23 clauses, `hasInlineType3Font`) rather than answer over a font it could
//     not see. Carried across, the document answers exactly as the same font written as its own object does.
//   - **a `/FontFile3` whose stream's `/Subtype` is not a name — a wrong type, `null`, or absent.** The validator drops the whole font from the page's
//     resources (a CIDFontType0) or refuses the document (a Type 1 font). veraPDF throws on the first and reports on
//     the second (`fontFile3SubtypeThrows`).
//
// Every object in the cross-reference table is a starting point, and direct dictionaries and arrays inside it are
// followed; an indirect reference is not, since its target is a starting point of its own.
func fontsTheValidatorLoses(ctx *model.Context) []asideEntry {
	var found []asideEntry
	isType3 := func(o types.Object) bool {
		fd, ok := o.(types.Dict)
		if !ok {
			return false
		}
		st, _ := ctx.Dereference(fd["Subtype"])
		switch n := st.(type) {
		case types.Name:
			return n == "Type3"
		}
		return false
	}
	var visit func(o types.Object, depth int)
	visit = func(o types.Object, depth int) {
		if depth > maxAsideNesting {
			return
		}
		switch v := o.(type) {
		case types.Dict:
			if fonts, ok := v["Font"]; ok {
				if fr, err := ctx.Dereference(fonts); err == nil {
					if fd, ok := fr.(types.Dict); ok {
						for name, f := range fd {
							if isType3(f) {
								found = append(found, asideEntry{dict: fd, key: name, val: f})
							}
						}
					}
				}
			}
			if ff, ok := v["FontFile3"]; ok {
				// **`Dereference`, never `DereferenceStreamDict`, before the validation.** The second marks the object
				// VALID as it returns it (pdfcpu `xreftable.go`, v0.13.0), and the validator skips what is marked — so
				// merely looking the stream up that way made the document readable with nothing set aside, an
				// exemption from validation nobody had chosen. Found by switching the set-aside off and seeing no test
				// go red.
				var streamDict types.Dict
				if r, err := ctx.Dereference(ff); err == nil {
					switch sd := r.(type) {
					case types.StreamDict:
						streamDict = sd.Dict
					case *types.StreamDict:
						if sd != nil {
							streamDict = sd.Dict
						}
					}
				}
				if streamDict != nil {
					// Anything but a name, an absent or `null` /Subtype included: the validator drops the font over
					// those too, and veraPDF reports on them (no program, so 7.21.4.1 t1 fails).
					st, _ := ctx.Dereference(streamDict["Subtype"])
					switch st.(type) {
					case types.Name:
					default:
						found = append(found, asideEntry{dict: v, key: "FontFile3", val: ff})
					}
				}
			}
			for _, k := range v {
				if _, indirect := k.(types.IndirectRef); !indirect {
					visit(k, depth+1)
				}
			}
		case types.StreamDict:
			visit(v.Dict, depth+1)
		case *types.StreamDict:
			if v != nil {
				visit(v.Dict, depth+1)
			}
		case types.Array:
			for _, k := range v {
				if _, indirect := k.(types.IndirectRef); !indirect {
					visit(k, depth+1)
				}
			}
		}
	}
	for _, e := range ctx.XRefTable.Table {
		if e != nil && !e.Free && e.Object != nil {
			visit(e.Object, 0)
		}
	}
	return found
}

// structureTreeRefusal reports whether err is the validator's refusal of the structure tree or of its parent tree.
func structureTreeRefusal(err error) bool {
	for _, target := range []error{
		model.ErrStructureTreeCycle, model.ErrStructureTreeDuplicate, model.ErrNumberTreeCycle, model.ErrNumberTreeDuplicate,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// setAsideStructureTree takes the catalog's `/StructTreeRoot` out of the validator's sight and returns what puts it
// back. For the checker only: it never writes the context out (ADR-115), and it reads the tree through its own
// bounded walks.
func setAsideStructureTree(ctx *model.Context) (restore func()) {
	root, err := ctx.XRefTable.Catalog()
	if err != nil || root == nil {
		return func() {}
	}
	held, ok := root["StructTreeRoot"]
	if !ok {
		return func() {}
	}
	delete(root, "StructTreeRoot")
	return func() { root["StructTreeRoot"] = held }
}

// maxStandInWidths bounds the zero-filled /Widths array `entriesTheValidatorRefuses` builds: a /LastChar past it is
// left to the validator's own answer.
const maxStandInWidths = 1 << 16

// entriesTheValidatorRefuses finds the entries pdfcpu v0.16.2's validator refuses a whole document over that the
// checker reads for itself (P02.S04, D4: carry — the checker never writes the context out, ADR-115), each answered by
// a rule veraPDF measures on the document as written:
//
//   - **a simple font's `/Widths` whose length is not LastChar − FirstChar + 1** (`validate/font.go:590-620`; relaxed
//     mode forgives only some). The glyph-metric rules (7.21.5) are about exactly this mismatch. A zero-filled array
//     of the length the validator wants stands in for the validation and the document's own is put back.
//   - **an ExtGState's `/Font` whose first element is not an indirect reference** (`validate/extGState.go:977`).
//     Taken out for the validation; the glyph rules read what the document wrote.
//   - **a media clip's `/Alt` that is not a well-formed multi-language array** (`validate/array_cardinality.go:58`).
//     Taken out for the validation.
//
// The walk is fontsTheValidatorLoses': every object in the table, direct dictionaries and arrays followed, an
// indirect reference not (its target is a starting point of its own), nesting bounded by maxAsideNesting.
func entriesTheValidatorRefuses(ctx *model.Context) []asideEntry {
	var found []asideEntry
	intOf := func(o types.Object) (int, bool) { return contextInt(ctx, o) }
	nameIs := func(o types.Object, want ...string) bool { return contextNameIs(ctx, o, want...) }
	check := func(v types.Dict) {
		// Simple fonts.
		if nameIs(v["Type"], "Font") && nameIs(v["Subtype"], "TrueType", "Type1", "MMType1") {
			if fc, ok := intOf(v["FirstChar"]); ok {
				if lc, ok := intOf(v["LastChar"]); ok && lc >= fc && lc-fc+1 <= maxStandInWidths {
					if r, err := ctx.Dereference(v["Widths"]); err == nil {
						if a, isArr := r.(types.Array); isArr && len(a) != lc-fc+1 {
							stand := make(types.Array, lc-fc+1)
							for i := range stand {
								stand[i] = types.Integer(0)
							}
							found = append(found, asideEntry{dict: v, key: "Widths", val: v["Widths"], with: stand})
						}
					}
				}
			}
		}
		// ExtGState /Font.
		if raw, ok := v["Font"]; ok {
			if r, err := ctx.Dereference(raw); err == nil {
				if a, isArr := r.(types.Array); isArr && len(a) == 2 {
					if _, indirect := a[0].(types.IndirectRef); !indirect {
						found = append(found, asideEntry{dict: v, key: "Font", val: raw})
					}
				}
			}
		}
		// Media clip /Alt.
		if nameIs(v["Type"], "MediaClip") {
			if raw, ok := v["Alt"]; ok {
				if r, err := ctx.Dereference(raw); err == nil {
					if a, isArr := r.(types.Array); isArr && !multiLanguageText(ctx, a) {
						found = append(found, asideEntry{dict: v, key: "Alt", val: raw})
					}
				}
			}
		}
	}
	var visit func(o types.Object, depth int)
	visit = func(o types.Object, depth int) {
		if depth > maxAsideNesting {
			return
		}
		switch v := o.(type) {
		case types.Dict:
			check(v)
			for _, k := range v {
				if _, indirect := k.(types.IndirectRef); !indirect {
					visit(k, depth+1)
				}
			}
		case types.StreamDict:
			visit(v.Dict, depth+1)
		case *types.StreamDict:
			if v != nil {
				visit(v.Dict, depth+1)
			}
		case types.Array:
			for _, k := range v {
				if _, indirect := k.(types.IndirectRef); !indirect {
					visit(k, depth+1)
				}
			}
		}
	}
	for _, e := range ctx.XRefTable.Table {
		if e != nil && !e.Free && e.Object != nil {
			visit(e.Object, 0)
		}
	}
	return found
}

// multiLanguageText reports whether a is what `validateMultiLanguageTextEntry` accepts: pairs of a language
// identifier string and a text string, no identifier twice (compared lower-cased).
func multiLanguageText(ctx *model.Context, a types.Array) bool {
	if len(a) == 0 || len(a)%2 == 1 {
		return false
	}
	seen := map[string]bool{}
	for i := 0; i < len(a); i += 2 {
		o, err := ctx.Dereference(a[i])
		if err != nil {
			return false
		}
		s, err := types.StringOrHexLiteral(o)
		if err != nil || s == nil {
			return false
		}
		k := strings.ToLower(*s)
		if seen[k] {
			return false
		}
		seen[k] = true
		t, err := ctx.Dereference(a[i+1])
		if err != nil {
			return false
		}
		if _, err := types.StringOrHexLiteral(t); err != nil {
			return false
		}
	}
	return true
}
