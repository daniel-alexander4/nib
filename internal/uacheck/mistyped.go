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

// setAsideMistypedText takes out of ctx, for pdfcpu's validation only, every `/Lang`, `/Alt`, `/ActualText` and `/E`
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
func setAsideMistypedText(ctx *model.Context) (restore func()) {
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
