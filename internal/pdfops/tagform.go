package pdfops

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Tagging an authored form — `PLAN-accessibility.md` P06.S07.
//
// # What S05 proved and what it did not
//
// P06.S05 cleared ua1 `7.18.1 t3` with `/TU` and `7.18.3 t1` with `/Tabs /S`, and measured
// `7.18.4 t1` as out of reach. That measurement was right about KEYS and wrong as a general claim:
// veraPDF's demand is literal — *"A Widget annotation shall be nested within a Form tag"*, failing
// with *"nested within null tag (standard type = null) instead of Form"* — and nesting is a tree,
// not a key. Measured at the phase close: three writes clear it, and `7.1 t11` with it.
//
// # The three writes, and the one that had no door
//
//   - a `/Form` grouping element per widget, holding `{/Type /OBJR, /Obj <widget>, /Pg <page>}`;
//   - `/StructParent` — SINGULAR — on the annotation;
//   - a `/ParentTree` entry that is a single REFERENCE, not an array indexed by MCID.
//
// P05.S03 modelled both ParentTree shapes and nothing had ever written the second;
// `setParentTreeSlot` fills an array slot and is the wrong door for an annotation.
// `setParentTreeSingle` and `freeParentTreeKey` are S07's, and the key search scans both shapes
// because they share one key space — a page array at key 3 and an annotation reference at key 3 are
// the same entry, and the second write destroys the first.
//
// # Why the source is exact
//
// The widget-to-field correspondence is nib's own authored input: the caller placed these fields and
// named them, so which element describes which widget is known rather than inferred. That is D4's
// exact tier, the same footing as `mdpdf`'s AST and unlike an OCR engine's reading of a picture.

// AuthorTaggedForm places form fields and DESCRIBES them, so a screen reader can announce each
// widget as a form control rather than meeting an annotation nothing in the tree points at.
//
// It is `AuthorForm` plus the tree: same fields, same `/TU` names, same `/Tabs /S`. **It never costs
// the caller the form** — a structure that cannot be built returns the authored document as
// `AuthorForm` would have produced it, with `tagged` false, because a form whose widgets are
// undescribed is what nib shipped for years and a document with no fields is worse than both.
func AuthorTaggedForm(pdf []byte, fields []FormField) (out []byte, tagged bool, err error) {
	authored, err := AuthorForm(pdf, fields)
	if err != nil {
		return nil, false, err
	}
	described, terr := writeMutated(authored, func(ctx *model.Context) error {
		live := map[int]bool{}
		for p := 1; p <= ctx.PageCount; p++ {
			if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
				live[ir.ObjectNumber.Value()] = true
			}
		}
		tree, err := ensureStructTree(ctx, live)
		if err != nil {
			return err
		}
		described := 0
		for p := 1; p <= ctx.PageCount; p++ {
			n, err := tagWidgetsOnPage(ctx, tree, p)
			if err != nil {
				return err
			}
			described += n
		}
		if described == 0 {
			return fmt.Errorf("pdfops: no widget annotation was described, so a tree would claim " +
				"structure over nothing")
		}
		return nil
	})
	if terr != nil {
		return authored, false, nil
	}
	// Both halves or neither — the law `TagAuthored`, `tagMarkdown` and `TagOCRLayer` all hold.
	described, terr = writeMutated(described, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		mi, _ := ctx.DereferenceDict(cat["MarkInfo"])
		if mi == nil {
			mi = types.Dict{}
		}
		mi["Marked"] = types.Boolean(true)
		cat["MarkInfo"] = mi
		return setTagSource(ctx, sourceExact)
	})
	if terr != nil {
		return authored, false, nil
	}
	if s := inspectTags(described); s.orphaned() {
		return authored, false, nil
	}
	return described, true, nil
}

// tagWidgetsOnPage nests every widget annotation on one page in a `/Form` element, and returns how
// many it described.
//
// A widget that already carries a `/StructParent` is left alone: something already describes it, and
// a second element pointing at the same annotation gives a reader two answers to one question.
func tagWidgetsOnPage(ctx *model.Context, tree *structTree, pageNr int) (int, error) {
	d, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil || d == nil {
		return 0, fmt.Errorf("pdfops: page %d does not resolve: %w", pageNr, err)
	}
	pageRef, err := ctx.PageDictIndRef(pageNr)
	if err != nil || pageRef == nil {
		return 0, fmt.Errorf("pdfops: page %d has no indirect reference: %w", pageNr, err)
	}
	annots, aerr := ctx.DereferenceArray(d["Annots"])
	if aerr != nil || len(annots) == 0 {
		return 0, nil
	}
	n := 0
	for _, a := range annots {
		// **An OBJR needs an indirect reference and the ADR-007-era SME note says why**:
		// `ctx.PageAnnots` holds indirect annots only, and a direct annotation dictionary has no
		// object number for `/Obj` to name. Skipped rather than failed — the rest of the form is
		// still worth describing.
		ar, isRef := a.(types.IndirectRef)
		if !isRef {
			continue
		}
		ad, derr := ctx.DereferenceDict(ar)
		if derr != nil || ad == nil {
			continue
		}
		if sub, _ := ad["Subtype"].(types.Name); sub != "Widget" {
			continue
		}
		if _, already := ad["StructParent"]; already {
			continue
		}
		elemRef, gerr := addGroupingElement(ctx, tree, "Form", nil)
		if gerr != nil {
			return n, gerr
		}
		if oerr := addOBJRTo(ctx, *elemRef, ar, *pageRef); oerr != nil {
			return n, oerr
		}
		key := freeParentTreeKey(ctx, tree)
		if perr := setParentTreeSingle(ctx, tree, key, *elemRef); perr != nil {
			return n, perr
		}
		ad["StructParent"] = types.Integer(key)
		n++
	}
	return n, nil
}
