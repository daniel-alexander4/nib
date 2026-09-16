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

// placement is one source page's content after the n-up: the sheet it is drawn on, the Form XObject
// holding it, and the page it came from.
//
// name is the RESOURCE NAME this placement was found under (`/Fm1`, `/Fm2`), and it is what the
// un-fusing repoints. The object number is not enough: when the optimizer has fused two forms, two
// names on one sheet reference the SAME object, so repointing "every entry naming this object" moves
// both and the first placement loses the object it had just anchored. Measured — the n-up of a
// two-identical-page document came out `dropped`.
type placement struct {
	page  types.IndirectRef // the sheet the XObject is drawn on
	xobj  types.IndirectRef // the Form XObject itself
	srcNr int               // the original page it came from
	name  string
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
		// Every page this capture KEPT has been matched to a form. **It does not say every page of
		// the document has been** — `capturePageSources` skips a page with no `/StructParents` and a
		// page whose content is empty, so both sides of this comparison exclude it and the guard
		// cannot see one (/pending 535).
		//
		// Measured rather than argued, because the comment here used to claim more than it checks:
		// a two-page fixture whose second page has `/StructParents` and empty content, and one whose
		// second page has content and no `/StructParents`, both come out of `NUp` with the carry
		// abandoned and the claim dropped — `honest` deletes `/StructTreeRoot`, and the fate reads
		// as the honest loss. The skipped page is caught downstream by `dead-pg` rather than here,
		// which is a worse diagnosis and the same outcome. So this is the guard it is, not the guard
		// it read as.
		return nil, false
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

	// ── Repoint every element from its original page to the sheet it now appears on, and say WHERE
	//    its marked content went.
	//
	// # `/Stm` is a marked-content reference's key, and it is not a structure element's
	//
	// This block wrote `d["Stm"] = pl.xobj` onto the element itself until P02.S08, on the reasoning
	// that *"`/Stm` names the content stream the element's MCIDs live in … PDF 32000-1 §14.7.4.4.
	// Without it a reader resolving an MCID looks in the page's own stream, where the content no
	// longer is."* **The diagnosis was right and the remedy did nothing.** ISO 32000-1 Table 323
	// enumerates a structure element dictionary's keys — `Type, S, P, ID, Pg, K, A, C, R, T, Lang,
	// Alt, E, ActualText` — and `/Stm` is not among them, so a conforming reader ignores it and looks
	// in the page's own stream **anyway**. §14.7.4.4 is the parent-tree clause; it licenses nothing
	// here. Table 325 has no `/Stm` either, so the `OBJR` arm was the same mistake.
	//
	// `/Stm` is Table 324, on a **marked-content reference**: *"(Optional; shall be an indirect
	// reference) The content stream containing the marked-content sequence. This entry should be
	// present only if the marked-content sequence resides in a content stream other than the content
	// stream for the page."* And the bare integer form of a kid is admitted (§14.7.4.2) only *"in the
	// common case where the marked-content sequence is contained in the content stream of the page
	// that is specified in the Pg entry"* — which is exactly what an n-up stops being true.
	//
	// So an element whose content moved into a form gets its integer kids rewritten as MCRs naming
	// that form. **Collected here and applied after the walk**, never during it: an MCR created in
	// flight carries the sheet's `/Pg`, the walk would reach it through `d["K"]`, and the sheet's
	// object number is not in `byNr` — the kid would count as `stranded` and abandon its own carry.
	//
	// Measured before this: 31 of 61 elements read two source pages' text concatenated, because a
	// reader keying on `(page, mcid)` cannot tell two forms' MCID 0 apart. veraPDF and `nib ua` score
	// both shapes identically — neither looks at a kid's encoding — so this is not a clause fix.
	// Those figures are measurements, not assertions: nothing in the suite re-derives them. They are
	// recorded with their populations and limits in ADR-038 and in P02.S08's seam inventory; what the
	// suite pins is the property, in `TestACarriedNUpReadsItsOwnText`.
	type mcrTarget struct {
		elem types.Dict
		pl   *placement // nil when no placement is in force anywhere up this element's ancestry
	}
	var toMCR []mcrTarget
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
	// `inherited` is the placement in force for the subtree being walked, or nil at the root.
	//
	// **`/Pg` is INHERITED, and reading it as per-element is how an element keeps bare integer kids
	// after its content has moved.** Table 323 makes `/Pg` optional and §14.7.4.2 resolves a bare
	// integer against *"the Pg entry of the structure element dictionary"* — which, when the element
	// has none, is an ancestor's; this repo's own reader implements that inheritance
	// (`structtree.go` `readKid`'s `inheritPg`). Collecting only elements that carry `/Pg` themselves
	// therefore skipped exactly the elements a producer chose not to repeat it on, and left their
	// integers claiming content in a sheet stream that holds only the `Do`. Caught in review.
	var walk func(o types.Object, inherited *placement)
	walk = func(o types.Object, inherited *placement) {
		if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
			for _, x := range arr {
				walk(x, inherited)
			}
			return
		}
		// An element written INLINE has no object number and so cannot be in the visited set at
		// all. An inline dictionary is reachable from one place by construction; note the set is
		// keyed on the ELEMENT, and an indirect `/K` array is not entered into it, so two elements
		// sharing one array object walk it twice. That is why the rewrite below is collected per
		// element and applied once per element, and why it is idempotent.
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
			// **The stale-key removal is unconditional, because the key is illegal on these two
			// types whatever this element's `/Pg` turns out to be.** Scoping it to the repointed
			// branch left a `/Stm` in place on exactly the elements the branch could not reach.
			if *t != "MCR" {
				delete(d, "Stm")
			}
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
					// this key writes it into the document.
					d["Pg"] = pl.page
					if *t == "MCR" {
						d["Stm"] = pl.xobj // the one dictionary Table 324 gives the key to
					}
					inherited = &pl
					repointed++
				}
			}
			if *t == "StructElem" {
				toMCR = append(toMCR, mcrTarget{elem: d, pl: inherited})
			}
		}
		if k, ok := d["K"]; ok {
			walk(k, inherited)
		}
	}
	walk(root["K"], nil)
	if repointed == 0 || stranded > 0 {
		return nil, false // all or nothing: a partly-anchored tree is worse than an honest loss
	}
	for _, t := range toMCR {
		if !rewriteKidsAsMCR(ctx, t.elem, t.pl) {
			return nil, false
		}
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

// rewriteKidsAsMCR turns an element's bare-integer `/K` kids into marked-content references naming
// the form XObject its content moved into.
//
// **A bare integer is a claim about WHERE, not just which.** ISO 32000-1 §14.7.4.2 admits the integer
// form only *"in the common case where the marked-content sequence is contained in the content stream
// of the page that is specified in the Pg entry of the structure element dictionary"*. After an n-up
// that is false for every element: the sequence is in a form XObject and the page's own stream holds
// only the `Do` that draws it. Table 324's `/Stm` is the key that says so, and an MCR is the only
// dictionary allowed to carry it.
//
// The MCRs are written DIRECT rather than as indirect objects. Three reasons, in order: the spec's own
// Example 2 writes one direct (`/K << /Type /MCR /Pg 2 0 R /MCID 0 >>`); it costs no xref slot per kid;
// and pdfcpu's `/K` array validation skips an indirect entry it has already marked valid
// (`validate/structTree.go:141-152`) while a direct dict is always walked, so the shape that gets
// validated is the shape that was written.
//
// It reports false rather than rewriting partly — the same rule the rest of this file keeps. A `/K`
// this function does not fully understand leaves the element's kids alone, and the caller drops the
// whole carry rather than shipping a tree that is half one encoding and half the other.
func rewriteKidsAsMCR(ctx *model.Context, elem types.Dict, pl *placement) bool {
	k, ok := elem["K"]
	if !ok {
		return true // an element with no kids anchors through its own /Pg and owes nothing here
	}
	o, err := ctx.Dereference(k)
	if err != nil {
		return false
	}

	// **The dereferenced SHAPE decides, and calling `DereferenceArray` first was the bug.** It
	// type-asserts to `types.Array` and returns an error for anything else
	// (`model/dereference.go:354-366`), so `/K << /Type /MCR /Pg 2 0 R /MCID 0 >>` — the spec's own
	// Example 2, and what LibreOffice and Word emit for a single-kid element — came back as an error
	// and abandoned the entire carry. No fixture in this repo has that shape, so the suite was green.
	// Caught in review, before it shipped.
	switch v := o.(type) {
	case types.Integer:
		if pl == nil {
			return false // an integer kid with no placement: see the array arm
		}
		elem["K"] = mcrFor(*pl, v)
		return true

	case types.Array:
		// `DereferenceArray` hands back the STORED slice rather than a copy, so writing through the
		// index writes into the document for an indirect array; `elem["K"] = v` covers the direct
		// one, where the map already holds a header over the same backing array.
		for i, e := range v {
			n, isInt := e.(types.Integer)
			if !isInt {
				continue
			}
			if pl == nil {
				// **Refusing beats shipping a mixed encoding.** This element owns marked content by
				// integer — a claim that the content is in its page's own stream — and nothing up
				// its ancestry says which form that content moved into. Leaving the integer would
				// pass every gate this repo has: `structureCarriedCompletely` reads `/Pg` liveness,
				// `/ParentTree` ownership and form draw counts, and none of them looks at a kid's
				// encoding. So the carry is abandoned and `honest` drops the claim instead.
				return false
			}
			v[i] = mcrFor(*pl, n)
		}
		elem["K"] = v
		return true

	case types.Dict:
		// A single element, MCR or OBJR kid. Each is already correct or already repointed by the
		// walk, and there is no integer to rewrite — including on a second visit to an element
		// reached through a `/K` array two elements share, which is what makes this idempotent.
		return true

	case nil:
		// `Dereference` yields nil with no error for a reference into a free or missing xref slot
		// (`model/dereference.go:99-102`). A dangling `/K` carries no integer to rewrite; whether it
		// is a defect is `checkStructConsistency`'s question, asked of the bytes this writes.
		return true

	default:
		return false
	}
}

// mcrFor is the marked-content reference that says where one MCID's sequence went.
//
// **A bare integer is a claim about WHERE, not just which.** ISO 32000-1 §14.7.4.2 admits the integer
// form only *"in the common case where the marked-content sequence is contained in the content stream
// of the page that is specified in the Pg entry of the structure element dictionary"*. After an n-up
// that is false for every element: the sequence is in a form XObject and the page's own stream holds
// only the `Do` that draws it. Table 324's `/Stm` is the key that says so, and an MCR is the only
// dictionary allowed to carry it.
//
// The MCR is DIRECT rather than an indirect object. Three reasons, in order: the spec's own Example 2
// writes one direct (`/K << /Type /MCR /Pg 2 0 R /MCID 0 >>`); it costs no xref slot per kid; and
// pdfcpu's `/K` array validation skips an indirect entry it has already marked valid
// (`validate/structTree.go:139-152`) while a direct dict is always walked, so the shape that gets
// validated is the shape that was written.
func mcrFor(pl placement, mcid types.Integer) types.Dict {
	return types.Dict{
		"Type": types.Name("MCR"),
		"Pg":   pl.page,
		"Stm":  pl.xobj,
		"MCID": mcid,
	}
}
