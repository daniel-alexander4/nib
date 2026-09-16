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
		// name is the RESOURCE NAME this placement was found under (`/Fm1`, `/Fm2`), and it is what
		// the un-fusing below repoints. The object number is not enough: when the optimizer has
		// fused two forms, two names on one sheet reference the SAME object, so repointing "every
		// entry naming this object" moves both and the first placement loses the object it had just
		// anchored. Measured — the n-up of a two-identical-page document came out `dropped`.
		name string
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
		for name, v := range xod {
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
				places = append(places, placement{page: *pageRef, xobj: ir, srcNr: nr, name: name})
				break
			}
		}
	}
	if len(places) != len(sources) {
		return nil, false // some page's content is not accounted for; do not ship a partial tree
	}

	// ── Give each PLACEMENT a form XObject of its own, carrying the /StructParents its source page had.
	//
	// The `/ParentTree` entry for that key already exists and already holds the right element
	// array — `api.NUp` copies the tree wholesale — so this one key is the entire reverse linkage.
	//
	// # Why a COPY, and why nothing can prevent the fusion instead (P02.S02, measured)
	//
	// pdfcpu fuses byte-equal form XObjects, so a document whose pages repeat comes back from any
	// optimizing read with ONE object drawn on several sheets: the census has 8 pages over 4
	// distinct contents, and its n-up arrives here as 4 objects, two of them drawn three times.
	// **No configuration prevents it.** The fusion happens inside `optimizeFontAndImages`, which
	// `OptimizeXRefTable` calls with no gate, and it is silent — the "redundant xobject" logging
	// sits on the image path, not this one, which is why it left no trace to find. Measured by
	// calling `OptimizeXRefTable` alone on a plainly-read context: `Fm4->416` became `Fm4->424`.
	// `OptimizeResourceDicts=false` changes nothing, and there is no single read to fix even if
	// there were a knob: `rewriteContext` and `dropUAIdentificationBytes` each have their own, and
	// `withoutUAClaim` has already run before `NUp` calls this function.
	//
	// **A fused form cannot be anchored, and that is arithmetic rather than a limitation here.**
	// `/StructParents` is one key on one object; a form drawn on three sheets would need three.
	// veraPDF says it as `isUniqueSemanticParent` — *"Form XObject contains MCIDs and is referenced
	// more than once"* (7.20 t2). So the carry un-fuses what the optimizer fused.
	//
	// Measured on the census through `NUp(2)`: 4 of 8 `/ParentTree` keys owned by nobody and 2
	// forms drawn 3 times each, becoming 8 keys each owned by its own XObject — and veraPDF's
	// verdict going from `7.20 t2` + `5 t1` to **`5 t1` alone**, the clause ADR-032 fails on
	// purpose. It costs 4% in bytes there (102,072 → 106,118), which is what a true tree costs.
	//
	// **The FIRST placement on an object keeps that object**, so a document whose forms were never
	// fused — the ordinary case — is written exactly as it was before this slice, and only a sheet
	// that would otherwise share a parent pays for a copy.
	byNr := map[int]placement{}
	anchored := map[int]bool{} // form object numbers an earlier placement has already claimed
	for _, pl := range places {
		e, found := ctx.FindTableEntryForIndRef(&pl.xobj)
		if !found || e == nil || e.Object == nil {
			return nil, false
		}
		sd, isStream := e.Object.(types.StreamDict)
		if !isStream {
			return nil, false
		}
		if nr := pl.xobj.ObjectNumber.Value(); !anchored[nr] {
			anchored[nr] = true
			sd.Dict["StructParents"] = types.Integer(sources[pl.srcNr].structParents)
			e.Object = sd
			byNr[pl.srcNr] = pl
			continue
		}
		// The DICTIONARY is copied and the content is not: the bytes are identical by construction
		// — being identical is what the optimizer matched on — and a `StreamDict` carries them by
		// reference, so an un-fusing costs one dictionary and one xref slot per extra sheet.
		clone := sd
		cd, isDict := sd.Dict.Clone().(types.Dict)
		if !isDict {
			return nil, false
		}
		clone.Dict = cd
		clone.Dict["StructParents"] = types.Integer(sources[pl.srcNr].structParents)
		nr, ierr := ctx.XRefTable.InsertObject(clone)
		if ierr != nil {
			return nil, false
		}
		ref := types.NewIndirectRef(nr, 0)
		if !repointSheetResource(ctx, pl.page, pl.name, *ref) {
			return nil, false
		}
		byNr[pl.srcNr] = placement{page: pl.page, xobj: *ref, srcNr: pl.srcNr, name: pl.name}
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
	// **Identity is the OBJECT NUMBER, and keying on the dictionary's CONTENT lost a twin.**
	// This walk used `d.String()`, so two byte-identical elements — which real producers emit
	// readily — were visited once and only one of them was repointed. The other kept a `/Pg`
	// naming a page that had left the page tree, and nothing could see it: this function still
	// returned true, `orphaned()` stayed false, and the fate read `carried`. Measured on
	// `twinElementFixture` through `NUp(2)`: 1 dead element `/Pg`. `readStructTree` was corrected
	// for exactly this at P05.S02 (`/pending 503`); this is the same correction, one file over.
	seen := map[int]bool{}
	repointed, stranded := 0, 0
	var walk func(o types.Object)
	walk = func(o types.Object) {
		if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
			for _, x := range arr {
				walk(x)
			}
			return
		}
		// An element written INLINE has no object number and so cannot be in the visited set at
		// all. That is safe for the same reason it is in `readStructTree`: an inline dictionary is
		// reachable from exactly one place by construction, so it cannot be revisited.
		if ir, isInd := o.(types.IndirectRef); isInd {
			nr := ir.ObjectNumber.Value()
			if seen[nr] {
				return
			}
			seen[nr] = true
		}
		d, e := ctx.DereferenceDict(o)
		if e != nil || d == nil {
			return
		}
		// **An `MCR` and an `OBJR` carry their own `/Pg`, and only `StructElem` used to be
		// repointed** — so an element moved to its sheet while its kid went on naming the page that
		// had gone, which is the second half of `/pending 503`'s tagcarry defect. Measured on a
		// two-page fixture whose second page is reached through an MCR: 1 dead kid `/Pg`, reported
		// `carried`. The three types are one rule and not three: whatever names a page this n-up
		// dismantled is repointed at the sheet that page's content now lives on, or the whole carry
		// is abandoned below.
		if t := d.NameEntry("Type"); t != nil && (*t == "StructElem" || *t == "MCR" || *t == "OBJR") {
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

// repointSheetResource points ONE named `/Resources /XObject` entry of a sheet at newRef.
//
// **By NAME, never by object number, and that distinction is the whole of it.** Two names on one
// sheet reference the same object exactly when the optimizer has fused them — which is the case
// this un-fusing exists for — so a repoint that moved "every entry naming this object" would move
// the sibling placement's entry too, stranding the object that placement had just anchored. The
// n-up of a two-identical-page document measured `dropped` for that reason before this took a name.
//
// It reports false when the sheet has no such entry, because a copy nothing draws is a document
// where the fused form is still fused and an unreferenced clone carries the key — a silent
// half-repair, which is the outcome this file refuses everywhere else.
func repointSheetResource(ctx *model.Context, page types.IndirectRef, name string, newRef types.IndirectRef) bool {
	pd, err := ctx.DereferenceDict(page)
	if err != nil || pd == nil {
		return false
	}
	res, rerr := ctx.DereferenceDict(pd["Resources"])
	if rerr != nil || res == nil {
		return false
	}
	xod, xerr := ctx.DereferenceDict(res["XObject"])
	if xerr != nil || xod == nil {
		return false
	}
	if _, has := xod[name]; !has {
		return false
	}
	xod[name] = newRef
	return true
}
