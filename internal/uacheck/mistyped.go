package uacheck

import (
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
								found = append(found, asideEntry{fd, name, f})
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
						found = append(found, asideEntry{v, "FontFile3", ff})
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
