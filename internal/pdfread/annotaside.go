package pdfread

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// setAsideUnreadableAnnotationParts takes out of the validator's sight the annotation defects it refuses a whole
// document over and nib reads nothing from, and returns what puts them back as parsed (ADR-168, /pending 888).
//
// Measured over 203 real documents with a form (`build/formrt.sh`'s local corpus): five could not be opened by any
// operation — rotate, optimize, export, fill — for one of these, and all five open and draw in a viewer.
//
//   - **An entry of a page's `/Annots` with no `/Subtype`** ("dict=annotDict required entry=Subtype missing",
//     `validate/annotation.go:1516`, v0.13.0). Four documents, all written by one real-estate forms engine, each
//     holding stamp-like annotations of exactly `/AP /F /NM /P /Rect`. The entry is left out of the array the
//     validator walks and put back where it stood. A form field's widget is NOT set aside: the validator gives one
//     its `/Subtype /Widget` itself when it reaches it through the form (`validate/form.go:427`).
//   - **A `/P` that is not a reference to a page** ("dereferenceDict: wrong type types.StreamDict",
//     `validateEntryP`, `validate/annotation.go:1369`). One document: a widget whose `/P` names a 19-byte stream. `/P`
//     is optional and nothing in nib reads it; the key is taken off and put back.
//   - **A Text annotation's `/State` and `/StateModel` that are not a pair pdfcpu knows** ("dict=Text entry=StateModel
//     invalid dict entry", `validateAnnotationDictText`, `validate/annotation.go:214-249`) — /pending 899. pdfcpu
//     v0.13 accepts only the models `Marked` and `Review` with their own states; Acrobat writes `MigrationStatus` /
//     `MigrationNone` on a migrated comment. One real document of 2,326 (a reviewed school handout, its sticky
//     notes); v0.16 reads it. Nothing in nib reads either key, and the
//     validator reads them only to refuse, so every Text annotation's pair is taken off and put back — the one
//     exception to "only what the validator refuses", because a pair it accepts is one it does nothing with.
//
// Set aside, not repaired and not dropped: what a viewer shows of these documents is in those entries (the stamps'
// appearance streams), and a document that came back from an operation without them would have lost content nobody
// asked to remove. So every reader after the validator — nib's, pdfcpu's writer, the content digest — sees the
// document as it was parsed, exactly as with `escapeInfoKeys`.
//
// A document that read before this reads the same: everything set aside is back before any reader has the context,
// the first two can be in no document the validator accepted, and the third is a pair it does nothing with but
// refuse. Objects are looked up with `ctx.Dereference` alone (ADR-116: the typed lookups mark an object validated
// before the validator has seen it).
func setAsideUnreadableAnnotationParts(ctx *model.Context) (restore func()) {
	var undo []func()
	restore = func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}
	xt := ctx.XRefTable
	dictOf := func(o types.Object) types.Dict {
		o, err := xt.Dereference(o)
		if err != nil {
			return nil
		}
		d, _ := o.(types.Dict)
		return d
	}
	arrayOf := func(o types.Object) types.Array {
		o, err := xt.Dereference(o)
		if err != nil {
			return nil
		}
		a, _ := o.(types.Array)
		return a
	}

	// A `/P` the validator accepts is absent, null, or a reference to a dictionary whose /Type is /Page.
	seenP := map[int]bool{}
	asideP := func(d types.Dict, nr int) {
		if nr > 0 {
			if seenP[nr] {
				return
			}
			seenP[nr] = true
		}
		held, ok := d["P"]
		if !ok || held == nil {
			return
		}
		if _, isRef := held.(types.IndirectRef); isRef {
			o, err := xt.Dereference(held)
			if err != nil {
				return // the validator reports a reference it cannot follow itself
			}
			if o == nil {
				return
			}
			if pd, isDict := o.(types.Dict); isDict {
				if t := pd.Type(); t != nil && *t == "Page" {
					return
				}
			}
		}
		delete(d, "P")
		undo = append(undo, func() { d["P"] = held })
	}
	// A Text annotation's /State and /StateModel, together and whatever they hold: the validator reads them only to
	// refuse, so setting aside a pair it would accept changes nothing it does — restating its rule would only be a
	// second copy to keep in step with pdfcpu's.
	asideState := func(d types.Dict) {
		st, _ := xt.Dereference(d["Subtype"])
		if n, ok := st.(types.Name); !ok || n != "Text" {
			return
		}
		state, model := d["State"], d["StateModel"]
		if state == nil && model == nil {
			return
		}
		_, hadState := d["State"]
		_, hadModel := d["StateModel"]
		delete(d, "State")
		delete(d, "StateModel")
		undo = append(undo, func() {
			if hadState {
				d["State"] = state
			}
			if hadModel {
				d["StateModel"] = model
			}
		})
	}
	nrOf := func(o types.Object) int {
		if r, ok := o.(types.IndirectRef); ok {
			return r.ObjectNumber.Value()
		}
		return 0
	}

	// Pages: every /Annots entry.
	visited := map[int]bool{}
	var node func(o types.Object, depth int)
	node = func(o types.Object, depth int) {
		if depth > maxAsideDepth {
			return
		}
		if nr := nrOf(o); nr > 0 {
			if visited[nr] {
				return
			}
			visited[nr] = true
		}
		d := dictOf(o)
		if d == nil {
			return
		}
		if kids, ok := d["Kids"]; ok {
			for _, k := range arrayOf(kids) {
				node(k, depth+1)
			}
			return
		}
		held, ok := d["Annots"]
		if !ok {
			return
		}
		annots := arrayOf(held)
		var kept types.Array
		var aside []asideAnnot
		for i, e := range annots {
			ad := dictOf(e)
			if ad == nil {
				kept = append(kept, e) // not a dictionary: the validator's own to drop or refuse
				continue
			}
			asideP(ad, nrOf(e))
			asideState(ad)
			_, hasSubtype := ad["Subtype"]
			_, isField := ad["FT"]
			_, hasParent := ad["Parent"]
			if hasSubtype || isField || hasParent {
				kept = append(kept, e)
				continue
			}
			aside = append(aside, asideAnnot{at: i, entry: e})
		}
		if len(aside) == 0 {
			return
		}
		if len(kept) == 0 {
			delete(d, "Annots")
		} else {
			d["Annots"] = kept
		}
		undo = append(undo, func() { d["Annots"] = putBackAnnots(annots, arrayOf(d["Annots"]), aside) })
	}
	root, err := xt.Catalog()
	if err != nil || root == nil {
		return restore // the validator reports a document with no catalog itself
	}
	node(root["Pages"], 0)

	// The form: every field and widget, on a page or not (ADR-166 — a field can be on none).
	seenField := map[int]bool{}
	var field func(o types.Object, depth int)
	field = func(o types.Object, depth int) {
		if depth > maxAsideDepth {
			return
		}
		nr := nrOf(o)
		if nr > 0 {
			if seenField[nr] {
				return
			}
			seenField[nr] = true
		}
		d := dictOf(o)
		if d == nil {
			return
		}
		asideP(d, nr)
		for _, k := range arrayOf(d["Kids"]) {
			field(k, depth+1)
		}
	}
	if form := dictOf(root["AcroForm"]); form != nil {
		for _, f := range arrayOf(form["Fields"]) {
			field(f, 0)
		}
	}
	return restore
}

// maxAsideDepth bounds both walks. A page tree or a field tree nested deeper is left to the validator, which has
// its own answer for one.
const maxAsideDepth = 64

// asideAnnot is one `/Annots` entry kept from the validator, and where it stood.
type asideAnnot struct {
	at    int
	entry types.Object
}

// putBackAnnots is the page's `/Annots` after validation with the set-aside entries back in it.
//
// The validator does not leave the array it was given: it writes a cleaned one of its own, without the entries it
// could not resolve (`validateAnnotationsArray`). So an entry goes back in front of the first entry that followed
// it in the document's own array and is still there, and at the end when none is — the order the document wrote,
// among what survived.
func putBackAnnots(original, validated types.Array, aside []asideAnnot) types.Array {
	out := append(types.Array{}, validated...)
	for _, a := range aside {
		at := len(out)
	find:
		for _, later := range original[a.at+1:] {
			lr, ok := later.(types.IndirectRef)
			if !ok {
				continue
			}
			for i, o := range out {
				if r, ok := o.(types.IndirectRef); ok && r == lr {
					at = i
					break find
				}
			}
		}
		out = append(out, nil)
		copy(out[at+1:], out[at:])
		out[at] = a.entry
	}
	return out
}
