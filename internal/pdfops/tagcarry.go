package pdfops

import (
	"bytes"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Carrying the tag tree through an n-up — `PLAN-accessibility.md` P01.S06.
//
// # What `api.NUp` actually does to a tagged document
//
// It does **not** destroy the structure, and every earlier conclusion in this plan said it did.
// Measured on a 4-page LibreOffice document with 45 struct elements: the composed output holds 2
// sheets, 4 Form XObjects whose decoded content is **byte-identical to the four original pages**,
// all 45 elements, and a `/ParentTree` still carrying its four entries keyed 0–3. The content is
// still inside its `BDC … EMC` sequences with its MCIDs. veraPDF says the same thing from the
// outside: its 7.1 t3 failures name `xObject[0]/contentStream[0]/content[2]{mcid:0}`, which is the
// original marked content, found and rejected only because nothing links to it.
//
// **What is missing is anchoring, and only anchoring.** No Form XObject carries `/StructParents`,
// and every element's `/Pg` names a page object that is no longer in the page tree. Both are
// repairable from what survives, so `nup` can preserve tagging rather than merely being honest
// about losing it — which is what P01's exit criterion asks for and what dropping the claim could
// never deliver.
//
// # Nothing here authors structure
//
// Every element, every `/K`, every MCID and every `/ParentTree` entry is the producer's. This code
// rewrites **references** — one key added per XObject, two keys rewritten per element — and creates
// no structure of its own. That is what keeps it inside P01's goal (*"nothing in this phase authors
// any structure"*) and on D2's side of the preservation/authoring line.
//
// # It is attempted, verified, and abandoned — never trusted
//
// `carryTagsThroughNUp` returns `ok=false` on anything it does not fully understand, and its caller
// then falls through to `honest`, which drops the claim. A half-remapped tree is a worse outcome
// than an honest loss, so there is no partial success: either every element that pointed at an
// original page now points at a sheet, or the whole attempt is discarded. The caller re-measures
// the result rather than believing this function's own report.

// pageSource is one original page, captured BEFORE the n-up runs.
type pageSource struct {
	objNr         int    // the original page's object number, which elements' /Pg still names
	structParents int    // its /StructParents, which is also its /ParentTree key
	content       []byte // decoded content stream, the key the Form XObject is matched on
}

// capturePageSources records what an n-up is about to dismantle.
//
// **Taken from the INPUT, before `api.NUp` runs**, because the output no longer contains the
// original page objects — only their content, inside Form XObjects.
func capturePageSources(pdf []byte) (map[int]pageSource, bool) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, false
	}
	out := map[int]pageSource{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			return nil, false
		}
		sp, ok := d["StructParents"]
		if !ok {
			// A page with no /StructParents contributes no ParentTree entry, so there is nothing
			// to re-anchor for it. That is not a failure — an untagged page in a tagged document
			// is ordinary — but it does mean this page's content will be unreachable afterwards,
			// which the caller's post-condition is what catches.
			continue
		}
		n, isInt := sp.(types.Integer)
		if !isInt {
			return nil, false
		}
		ir, e := ctx.PageDictIndRef(p)
		if e != nil || ir == nil {
			return nil, false
		}
		b, cerr := ctx.PageContent(d, p)
		if cerr != nil || len(b) == 0 {
			continue // an empty page has no marked content to re-anchor
		}
		out[ir.ObjectNumber.Value()] = pageSource{
			objNr:         ir.ObjectNumber.Value(),
			structParents: n.Value(),
			content:       b,
		}
	}
	return out, true
}

// carryTagsThroughNUp re-anchors a structure tree that `api.NUp` left pointing at pages it removed.
//
// It returns the repaired document and true only when **every** element that pointed at an original
// page has been repointed at a sheet. Anything else returns false and the caller drops the claim.
func carryTagsThroughNUp(src, composed []byte) ([]byte, bool) {
	sources, ok := capturePageSources(src)
	if !ok || len(sources) == 0 {
		return nil, false
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(composed), model.NewDefaultConfiguration())
	if err != nil {
		return nil, false
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return nil, false
	}
	if _, hasTree := cat["StructTreeRoot"]; !hasTree {
		return nil, false // nothing survived to carry
	}

	// ── Match each Form XObject to the original page whose content it holds.
	//
	// **Byte equality on the decoded content stream**, which `api.NUp` copies verbatim — measured,
	// all four XObjects matched their source page exactly. The blind spot is declared rather than
	// papered over: two original pages with byte-identical content are indistinguishable here, and
	// the first match wins. The harm is bounded — identical content means identical text, so a
	// reader gets the right words under a possibly-wrong ancestor — but it is a real ambiguity and
	// `matched` below refuses to bind one source page to two XObjects, which keeps it from
	// silently dropping an element set.
	type placement struct {
		page  types.IndirectRef // the sheet the XObject is drawn on
		xobj  types.IndirectRef // the Form XObject itself
		srcNr int               // the original page it came from
	}
	var places []placement
	claimed := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			return nil, false
		}
		pageRef, e := ctx.PageDictIndRef(p)
		if e != nil || pageRef == nil {
			return nil, false
		}
		res, rerr := ctx.DereferenceDict(d["Resources"])
		if rerr != nil || res == nil {
			continue
		}
		xod, xerr := ctx.DereferenceDict(res["XObject"])
		if xerr != nil || xod == nil {
			continue
		}
		for _, v := range xod {
			ir, isInd := v.(types.IndirectRef)
			if !isInd {
				return nil, false
			}
			sd, _, serr := ctx.DereferenceStreamDict(ir)
			if serr != nil || sd == nil {
				return nil, false
			}
			if sub := sd.Dict.NameEntry("Subtype"); sub == nil || *sub != "Form" {
				continue
			}
			if derr := sd.Decode(); derr != nil {
				return nil, false
			}
			for nr, ps := range sources {
				if claimed[nr] || !bytes.Equal(sd.Content, ps.content) {
					continue
				}
				claimed[nr] = true
				places = append(places, placement{page: *pageRef, xobj: ir, srcNr: nr})
				break
			}
		}
	}
	if len(places) != len(sources) {
		return nil, false // some page's content is not accounted for; do not ship a partial tree
	}

	// ── Give each XObject the /StructParents its source page had.
	//
	// The `/ParentTree` entry for that key already exists and already holds the right element
	// array — `api.NUp` copies the tree wholesale — so this one key is the entire reverse linkage.
	byNr := map[int]placement{}
	for _, pl := range places {
		byNr[pl.srcNr] = pl
		e, found := ctx.FindTableEntryForIndRef(&pl.xobj)
		if !found || e == nil || e.Object == nil {
			return nil, false
		}
		sd, isStream := e.Object.(types.StreamDict)
		if !isStream {
			return nil, false
		}
		sd.Dict["StructParents"] = types.Integer(sources[pl.srcNr].structParents)
		e.Object = sd
	}

	// ── Repoint every element from its original page to the sheet it now appears on.
	//
	// `/Stm` names the content stream the element's MCIDs live in, which is the form XObject rather
	// than the page — PDF 32000-1 §14.7.4.4. Without it a reader resolving an MCID looks in the
	// page's own stream, where the content no longer is.
	root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
	if rerr != nil || root == nil {
		return nil, false
	}
	seen := map[string]bool{}
	repointed, stranded := 0, 0
	var walk func(o types.Object)
	walk = func(o types.Object) {
		if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
			for _, x := range arr {
				walk(x)
			}
			return
		}
		d, e := ctx.DereferenceDict(o)
		if e != nil || d == nil {
			return
		}
		key := d.String()
		if seen[key] {
			return
		}
		seen[key] = true
		if t := d.NameEntry("Type"); t != nil && *t == "StructElem" {
			if pg, ok := d["Pg"]; ok {
				ind, isInd := pg.(types.IndirectRef)
				switch {
				case !isInd:
					stranded++
				default:
					pl, found := byNr[ind.ObjectNumber.Value()]
					if !found {
						stranded++
						break
					}
					// A dict is a map and `DereferenceDict` hands back the stored one, so writing
					// these two keys writes them into the document.
					d["Pg"] = pl.page
					d["Stm"] = pl.xobj
					repointed++
				}
			}
		}
		if k, ok := d["K"]; ok {
			walk(k)
		}
	}
	walk(root["K"])
	if repointed == 0 || stranded > 0 {
		return nil, false // all or nothing: a partly-anchored tree is worse than an honest loss
	}

	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}
