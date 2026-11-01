package pdfops

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// FlattenResult counts what a flatten did, by widget. Counts only: a value a form held is never reported (ADR-183).
type FlattenResult struct {
	// Flattened is the widgets whose appearance now sits in the page's content.
	Flattened int `json:"flattened"`
	// Blank is the widgets with no value and no appearance: nothing was drawn and nothing was lost.
	Blank int `json:"blank"`
	// Hidden is the widgets a reader does not show (the hidden and no-view flags): removed, not drawn.
	Hidden int `json:"hidden"`
}

// FlattenMissingAppearance is a flatten refused because some field holds a value that no appearance stream draws.
// A viewer that regenerates appearances (`/NeedAppearances`) shows such a value; a flattened page would not, so the
// value would be lost without a word. Fill the form through nib first, which writes the appearance with the value.
type FlattenMissingAppearance struct{ Fields []string }

func (e *FlattenMissingAppearance) Error() string {
	shown := e.Fields
	more := ""
	if len(shown) > 10 {
		more = fmt.Sprintf(" and %d more", len(shown)-10)
		shown = shown[:10]
	}
	return fmt.Sprintf("%d field(s) hold a value but no appearance draws it, so flattening would lose it (%s%s) — fill the form with nib first, which writes the appearance",
		len(e.Fields), strings.Join(shown, ", "), more)
}

// FlattenForm burns every form widget's appearance into its page's content and removes the widgets, the fields and the
// AcroForm (ADR-183). The appearance stream is reused as a Form XObject and drawn where the widget's rectangle was, so
// the result is vector and a field's text stays text; the page's own content, text layer included, is not touched.
//
//   - A widget with no drawable appearance and no value is blank: nothing to draw, removed. One that holds a value and
//     has no appearance refuses the whole call (*FlattenMissingAppearance).
//   - A hidden or no-view widget is removed without being drawn: it was not shown.
//   - The drawn content is an /Artifact, so a tagged document gains no untagged content (ADR-031); the widgets' own
//     structure references are taken out as a removed annotation's are (unnameInStructure), and a Form element that
//     held nothing else goes with them (ADR-124's delete): veraPDF's 7.18.4 t2 fails a Form element with no widget.
//     A screen reader therefore does not announce a flattened value: flatten what is printed, not what is read.
//   - dropSignature takes any signature with it; the caller refuses a signed document otherwise (the sign package
//     cannot be imported here).
//
// It is a rewrite like every other: a PDF/UA identification is dropped (ADR-032) and /XFA goes with the write (ADR-145).
func FlattenForm(pdf []byte, dropSignature bool) (out []byte, res FlattenResult, err error) {
	out, err = writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, err := xt.Catalog()
		if err != nil || root == nil {
			return err
		}
		acro := derefDict(xt, root["AcroForm"])
		if acro == nil {
			return ErrNoFormFields
		}
		if dropSignature {
			ctx.RemoveSignature()
		}
		gone := map[int]types.Dict{}
		var missing []string
		serial := 0
		werr := eachPageAnnots(xt, root, func(page types.Dict, raw types.Object, _ int) {
			annots := derefArray(xt, raw)
			kept := make(types.Array, 0, len(annots))
			var draw strings.Builder
			xobjects := types.Dict{}
			for _, a := range annots {
				annot := derefDict(xt, a)
				if annot == nil || nameVal(annot, "Subtype") != "Widget" {
					kept = append(kept, a)
					continue
				}
				if ir, ok := a.(types.IndirectRef); ok {
					gone[ir.ObjectNumber.Value()] = annot
				}
				if f := annot.IntEntry("F"); f != nil && *f&(2|32) != 0 { // hidden, no-view
					res.Hidden++
					continue
				}
				ref, sd, cm, ok := widgetAppearance(ctx, annot)
				if !ok {
					if holdsValue(xt, annot) {
						missing = append(missing, qualifiedName(xt, annot))
					} else {
						res.Blank++
					}
					continue
				}
				// An appearance that names no fonts of its own is drawn with the form's default resources,
				// and those leave with the AcroForm.
				if sd.Dict["Resources"] == nil && acro["DR"] != nil {
					sd.Dict["Resources"] = acro["DR"]
				}
				serial++
				name := "NibFl" + strconv.Itoa(serial)
				xobjects[name] = ref
				fmt.Fprintf(&draw, "/Artifact BMC\nq %s cm /%s Do Q\nEMC\n", cm, name)
				res.Flattened++
			}
			if len(kept) != len(annots) {
				if ir, ok := raw.(types.IndirectRef); ok {
					if e, found := xt.FindTableEntryForIndRef(&ir); found && e != nil {
						e.Object = kept
					}
				} else if len(kept) == 0 {
					page.Delete("Annots")
				} else {
					page["Annots"] = kept
				}
			}
			if len(xobjects) > 0 {
				if err := drawOnPage(ctx, page, xobjects, draw.String()); err != nil {
					missing = append(missing, "(page "+err.Error()+")")
				}
			}
		})
		if werr != nil {
			return werr
		}
		if len(missing) > 0 {
			return &FlattenMissingAppearance{Fields: missing}
		}
		if res.Flattened+res.Blank+res.Hidden == 0 {
			return ErrNoFormFields
		}
		emptyBefore := emptyFormElements(ctx)
		unnameInStructure(ctx, root, gone)
		if err := deleteEmptiedForms(ctx, emptyBefore); err != nil {
			return err
		}
		// Unlinked, not freed (dropKey): the appearances may name fonts that only the form's /DR reached, and an
		// object nothing names is not written anyway.
		root.Delete("AcroForm")
		return nil
	})
	if err != nil {
		return nil, FlattenResult{}, err
	}
	return out, res, nil
}

// widgetAppearance resolves the appearance a widget shows — /AP /N, or /N[/AS] where /N is a dictionary of states —
// and the matrix that places it on the widget's rectangle (ISO 32000-1, 12.5.5, algorithm 8.1: the form's /BBox taken
// through its /Matrix, then scaled and moved onto /Rect). ok is false when there is nothing to draw: no appearance, an
// "Off" state with no stream, no /BBox, or a rectangle with no area.
func widgetAppearance(ctx *model.Context, annot types.Dict) (ref types.IndirectRef, sd *types.StreamDict, cm string, ok bool) {
	xt := ctx.XRefTable
	ap := derefDict(xt, annot["AP"])
	if ap == nil || ap["N"] == nil {
		return
	}
	n := ap["N"]
	if states := derefDict(xt, n); states != nil {
		as := nameVal(annot, "AS")
		if as == "" || states[as] == nil {
			return
		}
		n = states[as]
	}
	r, isRef := n.(types.IndirectRef)
	if !isRef {
		return
	}
	s, _, err := ctx.DereferenceStreamDict(r)
	if err != nil || s == nil {
		return
	}
	bb := derefArray(xt, s.Dict["BBox"])
	if len(bb) != 4 {
		return
	}
	var b [4]float64
	for i, o := range bb {
		v, isNum := pdfNumber(xt, o)
		if !isNum {
			return
		}
		b[i] = v
	}
	form := identityMatrix
	if fm, has := matrixFrom(ctx, s.Dict["Matrix"]); has {
		form = fm
	}
	tllx, tlly, turx, tury := form.rect(min(b[0], b[2]), min(b[1], b[3]), max(b[0], b[2]), max(b[1], b[3]))
	tw, th := turx-tllx, tury-tlly
	rc := widgetRect(xt, annot)
	rw, rh := rc[2]-rc[0], rc[3]-rc[1]
	if tw <= 0 || th <= 0 || rw <= 0 || rh <= 0 {
		return
	}
	sx, sy := rw/tw, rh/th
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	cm = strings.Join([]string{num(sx), "0", "0", num(sy), num(rc[0] - tllx*sx), num(rc[1] - tlly*sy)}, " ")
	return r, s, cm, true
}

// holdsValue reports whether a field has a value a reader would show: a non-empty string, a name other than /Off, or a
// non-empty array, read from the widget or the nearest ancestor that has /V.
func holdsValue(xt *model.XRefTable, d types.Dict) bool {
	for hop := 0; hop < 50 && d != nil; hop++ {
		if d["V"] != nil {
			switch v := resolved(xt, d["V"]).(type) {
			case types.StringLiteral, types.HexLiteral:
				return textEntry(xt, d, "V") != ""
			case types.Name:
				return v != "Off" && v != ""
			case types.Array:
				return len(v) > 0
			}
			return false
		}
		d = derefDict(xt, d["Parent"])
	}
	return false
}

// drawOnPage appends ops to page's content, after wrapping what is there in q … Q so an unbalanced graphics state
// cannot move what is drawn, and adds xobjects to a page-own copy of its resources (the page may inherit them, and
// shared ones are not ours to change).
func drawOnPage(ctx *model.Context, page types.Dict, xobjects types.Dict, ops string) error {
	xt := ctx.XRefTable
	cur := derefDict(xt, page["Resources"])
	for p, hop := derefDict(xt, page["Parent"]), 0; cur == nil && p != nil && hop < 50; p, hop = derefDict(xt, p["Parent"]), hop+1 {
		cur = derefDict(xt, p["Resources"])
	}
	res := types.Dict{}
	for k, v := range cur {
		res[k] = v
	}
	xo := types.Dict{}
	for k, v := range derefDict(xt, res["XObject"]) {
		xo[k] = v
	}
	for k, v := range xobjects {
		xo[k] = v
	}
	res["XObject"] = xo
	page["Resources"] = res

	stream := func(b string) (types.IndirectRef, error) {
		sd, err := ctx.NewStreamDictForBuf([]byte(b))
		if err != nil {
			return types.IndirectRef{}, err
		}
		if err := sd.Encode(); err != nil {
			return types.IndirectRef{}, err
		}
		r, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return types.IndirectRef{}, err
		}
		return *r, nil
	}
	head, err := stream("q\n")
	if err != nil {
		return err
	}
	tail, err := stream("Q\n" + ops)
	if err != nil {
		return err
	}
	contents := types.Array{head}
	switch c := page["Contents"].(type) { //pagecontent:key the list of streams, kept as it is with ours around it; no content is decoded
	case nil:
	case types.Array:
		contents = append(contents, c...)
	case types.IndirectRef:
		if arr := derefArray(xt, c); arr != nil {
			contents = append(contents, arr...)
		} else {
			contents = append(contents, c)
		}
	default:
		return errors.New("has contents of a kind flatten does not read")
	}
	page["Contents"] = append(contents, tail)
	return nil
}

// emptyFormElements is the object numbers of the Form elements that already hold nothing, so that deleteEmptiedForms
// takes out only the ones this flatten emptied. Nil for a document with no structure tree.
func emptyFormElements(ctx *model.Context) map[int]bool {
	tree, err := readStructTree(ctx, livePageObjects(ctx))
	if err != nil {
		return nil
	}
	empty := map[int]bool{}
	for _, e := range tree.elems {
		if e.kind == "Form" && len(e.kids) == 0 {
			empty[e.objNr] = true
		}
	}
	return empty
}

// deleteEmptiedForms deletes each Form element that holds nothing now and held something before, one at a time with
// the tree read again after each (a delete changes what the next one resolves through; structedit.go).
func deleteEmptiedForms(ctx *model.Context, emptyBefore map[int]bool) error {
	if emptyBefore == nil {
		return nil
	}
	for {
		tree, err := readStructTree(ctx, livePageObjects(ctx))
		if err != nil {
			return nil
		}
		var next *structElem
		for _, e := range tree.elems {
			if e.kind == "Form" && e.objNr != 0 && len(e.kids) == 0 && !emptyBefore[e.objNr] {
				next = e
				break
			}
		}
		if next == nil {
			return nil
		}
		if err := deleteElement(ctx, tree, next); err != nil {
			// The last element of a tree may not be deleted (ADR-031), and a Form inside an inline tag cannot be
			// named: leave what cannot go, and stop rather than loop on it.
			return nil
		}
		emptyBefore[next.objNr] = true // handled either way: never picked again
	}
}
