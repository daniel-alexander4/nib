package pdfops

import (
	"errors"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Which of a form's fields are on a page — the one answer (ADR-009, `/pending 888`).
//
// A form is a tree in the catalog (`/AcroForm /Fields`, `/Kids` beneath) and a set of widgets on pages; a field
// a person can fill is one with a widget that some page's `/Annots` names. The two can disagree. A file cut
// from a larger form package by other software keeps the other pages' fields in the tree — measured on real
// documents: one-page files carrying 465 fields, about 450 of them on no page and 150 of those still holding
// what was typed on the pages that are gone. pdfcpu lists only fields on a page and pdf.js lists the tree, and
// reading one as the other is how a round-trip instrument reported 5,177 lost values that no one could have
// entered.
//
// So "on a page" is decided HERE and nowhere else: the scan's finding for fields on no page, the removal of
// them, and "this document has no form fields" all ask this.

// formField is one terminal field of the form's tree: a node that carries a name and none of whose kids does.
type formField struct {
	ref    types.Object // the node as its parent's /Kids (or /Fields) names it
	dict   types.Dict
	kind   string // /FT, its own or the nearest ancestor's: Tx, Btn, Ch, Sig, or "" where none says
	flags  int    // /Ff, its own or the nearest ancestor's
	onPage bool   // some page's /Annots names one of its widgets
	value  bool   // it, or the nearest ancestor that says, holds a /V that is not empty and not /Off

	name    string        // the fully qualified name: the /T of it and of each ancestor that has one, joined with "."
	tooltip string        // its own /TU, "" where it has none
	widgets []fieldWidget // where it is drawn, in the order the tree names its widgets; empty exactly when onPage is false
}

// fieldWidget is one place a field is drawn: the 1-based page that names the widget, and its /Rect in that page's
// own user space, normalized to [llx lly urx ury] — raw, so a caller applies the page's /Rotate and boxes itself.
type fieldWidget struct {
	page int
	rect [4]float64
}

// pushButton reports a button that only does something — it holds no value (ISO 32000-1 Table 226, bit 17).
func (f formField) pushButton() bool { return f.kind == "Btn" && buttonKind(f.flags) == "button" }

// buttonKind is the ONE answer to what a button field is, from its `/Ff` flags alone (ISO 32000-1 Table 226: bit 17 a
// push button, bit 16 a radio button, else a check box) — as pdfcpu v0.16.2 and pdf.js read it, never from how many
// widgets it has (P02.S05, D5). `widgetKind` (pagemap.go) and `formField.pushButton` both route through it.
func buttonKind(ff int) string {
	switch {
	case ff&(1<<16) != 0:
		return "button"
	case ff&(1<<15) != 0:
		return "radio"
	}
	return "check"
}

// fillable reports a field `nib fill`, the mail-merge and the export are about: one that holds a value a person
// gives it. A signature is not one, and neither is a push button.
func (f formField) fillable() bool {
	switch f.kind {
	case "Tx", "Ch":
		return true
	case "Btn":
		return !f.pushButton()
	}
	return false
}

// formFields returns the terminal fields of ctx's form, each marked with whether it is on a page. Nil for a
// document with no form. Over `eachFormField`, the one walk of the tree, and `eachPageAnnots`, the page walk.
func formFields(xt *model.XRefTable) ([]formField, error) {
	root, err := xt.Catalog()
	if err != nil || root == nil {
		return nil, err
	}
	af := derefDict(xt, root["AcroForm"])
	if af == nil {
		return nil, nil
	}
	onPage := map[int]int{} // object number of a widget a page names → that page (the first, where several do)
	if err := eachPageAnnots(xt, root, func(_ types.Dict, raw types.Object, nr int) {
		for _, a := range derefArray(xt, raw) {
			if ir, ok := a.(types.IndirectRef); ok {
				if _, seen := onPage[ir.ObjectNumber.Value()]; !seen {
					onPage[ir.ObjectNumber.Value()] = nr
				}
			}
		}
	}); err != nil {
		return nil, err
	}
	named := func(d types.Dict) bool { _, ok := d.Find("T"); return ok }
	// inherited reads key from d or the nearest ancestor that has it. Bounded as the tree walk is: a /Parent
	// chain is the file's to write.
	inherited := func(d types.Dict, key string) types.Object {
		for hop := 0; hop < 50 && d != nil; hop++ {
			if o, ok := d.Find(key); ok {
				return o
			}
			d = derefDict(xt, d["Parent"])
		}
		return nil
	}
	var out []formField
	eachFormField(xt, af, func(o types.Object, d types.Dict) {
		if !named(d) {
			return // a widget kid, or a node with no name of its own: its named ancestor is the field
		}
		kids := derefArray(xt, d["Kids"])
		for _, k := range kids {
			if kd := derefDict(xt, k); kd != nil && named(kd) {
				return // a non-terminal node: its named kids are the fields
			}
		}
		f := formField{ref: o, dict: d}
		if n, ok := resolved(xt, inherited(d, "FT")).(types.Name); ok {
			f.kind = n.Value()
		}
		if i, ok := resolved(xt, inherited(d, "Ff")).(types.Integer); ok {
			f.flags = i.Value()
		}
		switch v := resolved(xt, inherited(d, "V")).(type) {
		case nil:
		case types.Name:
			f.value = v.Value() != "Off" && v.Value() != ""
		case types.StringLiteral:
			f.value = len(v.Value()) > 0
		case types.HexLiteral:
			f.value = len(v.Value()) > 0
		case types.Array:
			f.value = len(v) > 0
		default:
			f.value = true // a stream (rich text) or anything else that is there
		}
		f.name = qualifiedName(xt, d)
		f.tooltip = textEntry(xt, d, "TU")
		// Its widgets: the node itself where field and widget are one dictionary, else its kids.
		if ir, ok := o.(types.IndirectRef); ok {
			if pg, on := onPage[ir.ObjectNumber.Value()]; on {
				f.widgets = append(f.widgets, fieldWidget{pg, widgetRect(xt, d)})
			}
		}
		for _, k := range kids {
			if ir, ok := k.(types.IndirectRef); ok {
				if pg, on := onPage[ir.ObjectNumber.Value()]; on {
					f.widgets = append(f.widgets, fieldWidget{pg, widgetRect(xt, derefDict(xt, k))})
				}
			}
		}
		f.onPage = len(f.widgets) > 0
		out = append(out, f)
	})
	return out, nil
}

// qualifiedName is the name a fill matches a field by: the /T of d and of each ancestor that has a non-empty one,
// outermost first, joined with "." — pdfcpu's own (`form.fullyQualifiedFieldNameDepth`), which the export emits and
// the fill reads back. Bounded as the tree walk is: a /Parent chain is the file's to write.
func qualifiedName(xt *model.XRefTable, d types.Dict) string {
	var parts []string
	for hop := 0; hop < 50 && d != nil; hop++ {
		if t := textEntry(xt, d, "T"); t != "" {
			parts = append([]string{t}, parts...)
		}
		d = derefDict(xt, d["Parent"])
	}
	return strings.Join(parts, ".")
}

// textEntry is d[key] read as a PDF text string (PDFDocEncoding or UTF-16), "" where absent or not one.
func textEntry(xt *model.XRefTable, d types.Dict, key string) string {
	if d == nil || d[key] == nil {
		return ""
	}
	s, err := xt.DereferenceStringOrHexLiteral(d[key], model.V10, nil)
	if err != nil {
		return ""
	}
	return s
}

// widgetRect is d's /Rect as [llx lly urx ury], whichever corners the file gave first; zero where it is not four numbers.
func widgetRect(xt *model.XRefTable, d types.Dict) (r [4]float64) {
	a := derefArray(xt, d["Rect"])
	if len(a) != 4 {
		return r
	}
	var c [4]float64
	for i, o := range a {
		v, ok := pdfNumber(xt, o)
		if !ok {
			return r
		}
		c[i] = v
	}
	return [4]float64{min(c[0], c[2]), min(c[1], c[3]), max(c[0], c[2]), max(c[1], c[3])}
}

// resolved is o dereferenced, or nil where it does not resolve.
func resolved(xt *model.XRefTable, o types.Object) types.Object {
	if o == nil {
		return nil
	}
	r, err := xt.Dereference(o)
	if err != nil {
		return nil
	}
	return r
}

// NoFormFieldsRefusal is the sentence for a document with nothing to export or fill: a fact about the document,
// never a failure (ADR-072). The server recognises it and answers 422 `no-form-fields`.
const NoFormFieldsRefusal = "this document has no form fields to export or fill — no text box, check box or choice is on any of its pages"

// ErrNoFormFields is NoFormFieldsRefusal as an error. Twenty-one of 193 real documents whose only field is a
// signature were answered "could not export form", a 500, by a route that had nothing to export.
var ErrNoFormFields = errors.New(NoFormFieldsRefusal)

// noFormFields turns pdfcpu's "no form fields affected", from an EXPORT, into ErrNoFormFields; any other error
// is passed as it is. Not for a fill's own answer: there the same sentinel means the data named no field.
func noFormFields(err error) error {
	if errors.Is(err, api.ErrNoFormFieldsAffected) {
		return ErrNoFormFields
	}
	return err
}

// A push button — Print, Reset, Submit — holds no value, so it is nothing to export or fill. pdfcpu v0.13.0 had no
// such kind: it read every button as a check box, and one with more than one widget as a radio group, whose
// widgets' appearances it then asked for as dictionaries of states; a push button's is one stream, so the whole
// export or fill failed on it (three real forms of 193, `/pending 888`). v0.16.2 reads the flags and skips a push
// button itself (`form/form.go:463-471`, `fill.go:752`), so the button's widgets are kept out of the walk here as
// belt and braces, and so are widgets whose `/Parent` pdfcpu cannot walk (`unwalkableWidgets`), and put back.

// setPushButtonsAside is pushButtonsAside for a context that is read and thrown away (an export).
func setPushButtonsAside(ctx *model.Context) { pushButtonsAside(ctx.XRefTable) }

// pushButtonsAside removes every push button's widgets from the pages' annotation arrays in xt and returns the
// function that puts them back where they were. With no push button it changes nothing.
func pushButtonsAside(xt *model.XRefTable) (putBack func()) {
	putBack = func() {}
	fields, err := formFields(xt)
	if err != nil {
		fields = nil // the push buttons cannot be found; a widget pdfcpu cannot walk still can
	}
	aside := map[int]bool{}
	for _, f := range fields {
		if !f.pushButton() {
			continue
		}
		if ir, ok := f.ref.(types.IndirectRef); ok {
			aside[ir.ObjectNumber.Value()] = true
		}
		for _, k := range derefArray(xt, f.dict["Kids"]) {
			if ir, ok := k.(types.IndirectRef); ok {
				aside[ir.ObjectNumber.Value()] = true
			}
		}
	}
	root, err := xt.Catalog()
	if err != nil || root == nil {
		return putBack
	}
	unwalkableWidgets(xt, root, aside)
	if len(aside) == 0 {
		return putBack
	}
	var undo []func()
	_ = eachPageAnnots(xt, root, func(page types.Dict, raw types.Object, _ int) {
		was := derefArray(xt, raw)
		kept := make(types.Array, 0, len(was))
		for _, a := range was {
			if ir, ok := a.(types.IndirectRef); ok && aside[ir.ObjectNumber.Value()] {
				continue
			}
			kept = append(kept, a)
		}
		if len(kept) == len(was) {
			return
		}
		// The array is the page's own, or an object the page names: written where it lives.
		if ir, ok := raw.(types.IndirectRef); ok {
			if e, found := xt.FindTableEntryForIndRef(&ir); found && e != nil {
				e.Object = kept
				undo = append(undo, func() { e.Object = was })
				return
			}
		}
		page["Annots"] = kept
		undo = append(undo, func() { page["Annots"] = was })
	})
	// pdfcpu's fill and its export of choice fields walk `PageAnnots`, the cache the read built from the same arrays
	// (`form/fill.go:1368`, `form/form.go:874`), so the widgets leave it for the walk too.
	for _, pa := range xt.PageAnnots {
		for _, an := range pa {
			if an.IndRefs == nil {
				continue
			}
			refs := an.IndRefs
			was := *refs
			kept := make([]types.IndirectRef, 0, len(was))
			for _, r := range was {
				if !aside[r.ObjectNumber.Value()] {
					kept = append(kept, r)
				}
			}
			if len(kept) == len(was) {
				continue
			}
			*refs = kept
			undo = append(undo, func() { *refs = was })
		}
	}
	return func() {
		for _, u := range undo {
			u()
		}
	}
}

// unwalkableWidgets adds to aside every page annotation whose `/Parent` chain reaches an empty or unresolvable
// dictionary (P02.S05, D4). pdfcpu v0.16.2 names a widget's field by walking up its `/Parent` and answers the
// whole export or fill "corrupt field" at a link that dereferences to nothing (`form/form.go:154-156`); v0.13.0
// skipped the widget and read the rest. Two of 206 real forms (a 9 MB pair) lost their export to it. The widget is
// set aside with the push buttons and put back with them, so it is written as it was found. The walk is bounded
// as `formFields`' is: a `/Parent` chain is the file's to write.
func unwalkableWidgets(xt *model.XRefTable, root types.Dict, aside map[int]bool) {
	_ = eachPageAnnots(xt, root, func(_ types.Dict, raw types.Object, _ int) {
		for _, a := range derefArray(xt, raw) {
			ir, ok := a.(types.IndirectRef)
			if !ok {
				continue
			}
			cur := derefDict(xt, ir)
			for hop := 0; hop < 50 && cur != nil; hop++ {
				p, has := cur["Parent"]
				if !has {
					break
				}
				pd := derefDict(xt, p)
				if len(pd) == 0 {
					aside[ir.ObjectNumber.Value()] = true
					break
				}
				cur = pd
			}
		}
	})
}

// ErrOffPageFieldsRemain is RemoveOffPageFields' own check failing: the result still holds a field on no page.
var ErrOffPageFieldsRemain = errors.New("form fields on no page are still in the document after their removal")

// RemoveOffPageFields takes out of the form every field that is on no page — what the scan reports as
// `offPageFields` — and nothing else: the field leaves its parent's `/Kids` (or the form's `/Fields`), a
// container left with no field under it goes too, and the calculation order stops naming what is gone. Every
// field on a page, its value and its widgets are as they were. Only when asked: a form may keep a field with no
// box on purpose, for its own calculations, and the caller says so before it calls.
//
// Checked by a second read, as the other removals are: the result is scanned for what was to be removed.
func RemoveOffPageFields(pdf []byte) ([]byte, error) {
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, err := xt.Catalog()
		if err != nil || root == nil {
			return err
		}
		af := derefDict(xt, root["AcroForm"])
		if af == nil {
			return nil
		}
		fields, err := formFields(xt)
		if err != nil {
			return err
		}
		gone := map[int]bool{}
		for _, f := range fields {
			if f.onPage {
				continue
			}
			if ir, ok := f.ref.(types.IndirectRef); ok {
				gone[ir.ObjectNumber.Value()] = true
			}
		}
		if len(gone) == 0 {
			return nil
		}
		// A container goes when every field under it has gone. Found to a fixed point over the one walk: each
		// pass can only empty the parents of what the pass before removed.
		for changed := true; changed; {
			changed = false
			eachFormField(xt, af, func(o types.Object, d types.Dict) {
				ir, ok := o.(types.IndirectRef)
				if !ok || gone[ir.ObjectNumber.Value()] {
					return
				}
				kids := derefArray(xt, d["Kids"])
				if len(kids) == 0 {
					return
				}
				for _, k := range kids {
					kr, isRef := k.(types.IndirectRef)
					if !isRef || !gone[kr.ObjectNumber.Value()] {
						return
					}
				}
				gone[ir.ObjectNumber.Value()] = true
				changed = true
			})
		}
		without := func(a types.Array) types.Array {
			kept := make(types.Array, 0, len(a))
			for _, o := range a {
				if ir, ok := o.(types.IndirectRef); ok && gone[ir.ObjectNumber.Value()] {
					continue
				}
				kept = append(kept, o)
			}
			return kept
		}
		// Collected before anything is cut: the walk reads the arrays this then rewrites.
		var holders []types.Dict
		eachFormField(xt, af, func(_ types.Object, d types.Dict) {
			if len(derefArray(xt, d["Kids"])) > 0 {
				holders = append(holders, d)
			}
		})
		for _, d := range holders {
			setArray(xt, d, "Kids", without(derefArray(xt, d["Kids"])))
		}
		setArray(xt, af, "Fields", without(derefArray(xt, af["Fields"])))
		if co := derefArray(xt, af["CO"]); len(co) > 0 {
			if kept := without(co); len(kept) == 0 {
				delete(af, "CO")
			} else {
				setArray(xt, af, "CO", kept)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	rep, err := Scan(out)
	if err != nil {
		return nil, err
	}
	for _, f := range rep.Findings {
		if f.Kind == "offPageFields" {
			return nil, ErrOffPageFieldsRemain
		}
	}
	return out, nil
}

// setArray writes a as d[key] where the array lives: in the object d names by reference, or in d itself.
func setArray(xt *model.XRefTable, d types.Dict, key string, a types.Array) {
	if ir, ok := d[key].(types.IndirectRef); ok {
		if e, found := xt.FindTableEntryForIndRef(&ir); found && e != nil {
			e.Object = a
			return
		}
	}
	d[key] = a
}
