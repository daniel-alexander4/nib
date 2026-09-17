package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// pruneOrphanedAcroForm removes `/AcroForm` fields whose widgets are no longer on any page of this
// document, and drops the key entirely when none survived (/pending 573).
//
// # The defect: a form claiming fields nothing draws
//
// `api.NUp` composes its sheets as NEW page objects and never looks at `/Annots`, so pdfcpu — which
// writes by reachability — drops every widget with the source page dictionaries it removes. The
// FIELD dictionaries are reachable from the catalog rather than from a page, so they survive.
// Measured on a two-field, two-widget fixture through `NUp(2)`: **2 fields out, 0 widgets** — a
// reader offers a form that cannot be filled, and a form already filled keeps its values in
// dictionaries with nothing on any page to draw or edit them.
//
// # Why this and not a widget carry
//
// `/pending 573` asked for the widgets to be carried. ADR-045's two grounds for excluding `/Widget`
// from the note carry both stand, and neither is cosmetic: ISO 32000-1 §12.5.5 maps an `/AP`
// bounding box onto `/Rect` axis-aligned, so a widget would be drawn upright inside a box the
// placement rotates; and a signature widget reaching a `/V` blob would come back onto a document
// whose byte ranges the composition has destroyed, past four gates that key on an edited document
// having no signature (`pageselect.go`'s `dropSignature` enumerates them).
//
// **The rule this restores already existed at the sibling door.** `pruneAcroForm` has kept a page
// selection's form honest since `/pending 524` — the fields whose widgets survived, and no key at
// all when none did. A composition is the same question with the answer "none", and it was not
// asking. ADR-009 in the small: one rule, two doors, one of them not calling it.
//
// # It is the same predicate, deliberately
//
// `keepFields` walks the field tree and `pruneFieldRefs` cleans `/CO`, exactly as the page
// selection's prune does. What differs is only where the live set comes from: there, the pages a
// selection kept; here, every page the composed document actually has.
//
// Reports whether anything changed, so a caller sharing one parse can skip the write.
func pruneOrphanedAcroForm(ctx *model.Context) (bool, error) {
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		return false, err
	}
	form := derefDict(xt, root["AcroForm"])
	if form == nil {
		return false, nil
	}
	fields := derefArray(xt, form["Fields"])
	if len(fields) == 0 {
		// A form with no fields is already saying nothing; removing the key would be a second
		// answer to a question nobody asked, and it is not this function's defect to fix.
		return false, nil
	}

	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			continue
		}
		for _, a := range derefArray(xt, d["Annots"]) {
			if ar, ok := a.(types.IndirectRef); ok {
				live[ar.ObjectNumber.Value()] = true
			}
		}
	}

	kept := keepFields(xt, fields, func(nr int, _ types.Dict) bool { return live[nr] }, 0)
	if len(kept) == len(fields) {
		return false, nil // every field still has its widget: an ordinary document, untouched
	}
	if len(kept) == 0 {
		delete(root, "AcroForm")
		return true, nil
	}
	form["Fields"] = kept
	doomed := map[int]bool{}
	for _, o := range derefArray(xt, form["CO"]) {
		if r, ok := o.(types.IndirectRef); ok && !live[r.ObjectNumber.Value()] {
			doomed[r.ObjectNumber.Value()] = true
		}
	}
	pruneFieldRefs(xt, form, "CO", doomed)
	return true, nil
}
