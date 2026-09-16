package pdfops

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Page selection, performed IN THE SOURCE CONTEXT — the primitive `Collect`, `RemovePages` and
// `collectWithoutStructure` all route through (ADR-009).
//
// # Why this exists at all, rather than calling pdfcpu
//
// `api.Collect` builds a BRAND-NEW context (`ExtractPages` → `CreateContextWithXRefTable` →
// `AddPages`), so every object number changes. A structure tree cannot be transplanted across that
// boundary: pdfcpu's `migrateObject`/`migrateIndRef` are unexported, and a hand-rolled deep copy
// would silently mis-point MCR `/Stm` and OBJR references in real producer documents because nib
// cannot see pdfcpu's remapping. `PLAN-ua-coverage.md` P02.S04b carries the tree, and it can only do
// that where object numbers are stable. So the selection moves here first, carrying nothing of the
// tree with it — S04a is the selection, S04b is the carry.
//
// # The inversion this creates, which is the whole risk
//
// Building a fresh context is a WHITELIST: only what pdfcpu chooses to migrate survives, and
// everything else is left behind for free. Rewriting the page tree in place is a BLACKLIST:
// everything survives unless this code explicitly drops it. Measured, the things that vanish for
// free today are the outline, the page labels, the structure tree, `/MarkInfo`, `/Metadata`,
// `/ViewerPreferences`, every `/Names` subtree except `/Dests`, `/OpenAction`, the embedded files,
// the document `/Info` (including nib's own `NibFlags`) and the signature blob.
//
// **So the catalog is REBUILT from an allowlist, never pruned with a denylist.** A denylist is a
// list somebody enumerated, and the failure mode is the key nobody thought of leaking into every
// extract, split and redaction artifact a user is about to hand to someone else. An allowlist fails
// safe: a catalog key this code has never heard of is dropped, which is exactly what `api.Collect`
// did to it yesterday.
//
// The sharpest instance is `/StructTreeRoot`, and it is not hygiene. pdfcpu writes by reachability,
// so an unlinked page's bytes never reach the output — but a surviving structure tree is written
// deeply, and its elements' `/Pg` re-anchors the DROPPED page dict and therefore its `/Contents`.
// Left in, "delete page 4" ships page 4's text. `/MarkInfo` goes with it as a pair: kept alone it is
// the one shape `orphaned()` fires on unconditionally.
//
// # The page tree comes out FLAT, and that is parity
//
// The plan first said to keep the `Pages` hierarchy so inherited attributes survived by
// construction. That is false for a reorder: nib's client sends a whole-document permutation, and a
// page that must land between two leaves of a different subtree has to MOVE — at which point what it
// inherits changes. It is also not what today does. `api.Collect` already materializes `/Resources`,
// `/MediaBox` and `/Rotate` onto every page and emits a flat tree (`pkg/pdfcpu/page.go:140-145`).
// This code emits a flat tree too, materializing all FOUR inheritable attributes — pdfcpu never
// materializes `/CropBox`, so an inherited crop box is silently lost by today's `Collect`, and
// materializing the whole set fixes that in passing.
var inheritableKeys = [...]string{"Resources", "MediaBox", "CropBox", "Rotate"}

// catalogAllowlist is the set of catalog keys a subset may emit BY DEFAULT. Measured against the old
// implementation: a subset's catalog was exactly {Type, Pages, Names}, plus /Lang where nib re-added
// it and /AcroForm where a field survived. Anything not named here is dropped.
//
// **It is no longer the complete set, and the difference is earned rather than listed.** Since
// P02.S04b a successful structure carry re-adds `/StructTreeRoot`, `/MarkInfo`, a rebuilt
// `/Metadata` and a `/DisplayDocTitle`-only `/ViewerPreferences` ON TOP of this list — never into it.
// The default-deny shape is what that preserves: a key this code has never heard of is still
// dropped, and the four above appear only when `carryStructure` has pruned the tree onto the pages
// kept and found it still anchored to them. Measured, a carried subset's catalog is
// {Lang, MarkInfo, Metadata, Pages, StructTreeRoot, Type, ViewerPreferences} against the
// non-carrying door's {Lang, Pages, Type}.
var catalogAllowlist = map[string]bool{
	"Type":     true,
	"Pages":    true,
	"Lang":     true, // not page-indexed; nib re-added it after every subset before this
	"AcroForm": true, // pruned below to the fields whose widgets survived
	"Names":    true, // reduced below to /Dests alone, itself pruned
}

// pageLeaf is one leaf of the source page tree together with the attributes it inherits, resolved
// during the single walk that finds it.
type pageLeaf struct {
	ref types.IndirectRef
	dic types.Dict
	inh map[string]types.Object
}

// collectLeaves walks the page tree ONCE, in document order, resolving inherited attributes as it
// descends.
//
// One walk rather than `ctx.PageDict(i)` per page, which is what pdfcpu's own `addPages` does:
// `PageDict` walks from the root every call, so a per-page loop is O(pages²) and is already a
// measured cost in this package (`/pending 476` profiled it at 6 s of a 27 s digest).
func collectLeaves(xt *model.XRefTable, root types.Dict) ([]pageLeaf, types.IndirectRef, error) {
	pagesRef, ok := root["Pages"].(types.IndirectRef)
	if !ok {
		return nil, types.IndirectRef{}, fmt.Errorf("pdfops: this document has no page tree")
	}
	var leaves []pageLeaf
	onPath := map[int]bool{}

	var walk func(ref types.IndirectRef, inh map[string]types.Object, depth int) error
	walk = func(ref types.IndirectRef, inh map[string]types.Object, depth int) error {
		if depth > 50 {
			return fmt.Errorf("pdfops: this document's page tree is nested too deeply to read")
		}
		nr := ref.ObjectNumber.Value()
		if onPath[nr] {
			return fmt.Errorf("pdfops: this document's page tree refers back to itself")
		}
		onPath[nr] = true
		defer delete(onPath, nr)

		node := derefDict(xt, ref)
		if node == nil {
			return fmt.Errorf("pdfops: this document's page tree has a node that cannot be read")
		}
		// A node's own value shadows what it inherits, for its whole subtree.
		sub := make(map[string]types.Object, len(inh)+len(inheritableKeys))
		for k, v := range inh {
			sub[k] = v
		}
		for _, k := range inheritableKeys {
			if v, present := node[k]; present {
				sub[k] = v
			}
		}

		kids := derefArray(xt, node["Kids"])
		// An intermediate node with an empty /Kids is NOT a page. Testing "no kids" alone counted one
		// as a leaf, which put a `/Type /Pages` dict into the output's `/Kids` and shifted every page
		// after it by one — pdfcpu's own validator walks past such a node contributing zero, so nib's
		// page numbering would have disagreed with everybody else's.
		//
		// A `/Type /Page` node that nonetheless carries `/Kids` is DESCENDED INTO, which is what
		// `eachPage` (scan.go) does. The two walks must agree: a scan finding reports a 1-based page
		// number and a page operation acts on one, and a document where they disagreed would send the
		// user to a different page from the one they were shown.
		if nameVal(node, "Type") != "Pages" && len(kids) == 0 {
			leaves = append(leaves, pageLeaf{ref: ref, dic: node, inh: sub})
			return nil
		}
		for _, k := range kids {
			kr, isRef := k.(types.IndirectRef)
			if !isRef {
				return fmt.Errorf("pdfops: this document's page tree holds a page that is not a reference")
			}
			if err := walk(kr, sub, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(pagesRef, map[string]types.Object{}, 0); err != nil {
		return nil, types.IndirectRef{}, err
	}
	return leaves, pagesRef, nil
}

// selectPages rewrites ctx's page tree to hold exactly the pages named by keep — 1-based, in the
// order given, repeats allowed — and reduces the catalog to what a subset is allowed to carry.
//
// **`carry` asks for the source structure tree to be pruned onto the pages kept** (P02.S04b), and the
// return value says whether it was: the catalog's structure keys are restored only when the carry
// succeeded, so a refusal produces exactly the honest loss this primitive produced before that slice.
// It is one flag on one door rather than two implementations, and the two named wrappers at the
// `subset` level are where each caller's choice is recorded (ADR-009).
func selectPages(ctx *model.Context, keep []int, carry bool) (bool, error) {
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		return false, err
	}
	leaves, pagesRef, err := collectLeaves(xt, root)
	if err != nil {
		return false, err
	}
	if len(keep) == 0 {
		return false, fmt.Errorf("pdfops: that selection does not name any page of this document")
	}
	for _, n := range keep {
		if n < 1 || n > len(leaves) {
			return false, fmt.Errorf("pdfops: page %d is not in this document, which has %d page(s)", n, len(leaves))
		}
	}
	if len(leaves) == 1 && leaves[0].ref.ObjectNumber.Value() == pagesRef.ObjectNumber.Value() {
		return false, fmt.Errorf("pdfops: this document's page tree has no pages under its root")
	}

	// The signature goes BEFORE anything is cloned. `clonePage` deep-copies a page's annotations
	// into fresh objects, so a signature widget stripped only from the kept originals would be
	// copied onto the duplicate and survive there with its /V blob intact.
	all := make([]types.Dict, 0, len(leaves))
	for _, l := range leaves {
		all = append(all, l.dic)
	}
	dropSignature(xt, root, all)

	kids := make(types.Array, 0, len(keep))
	placed := map[int]bool{}
	keptPages := map[int]bool{}
	// The kept page DICTIONARIES, carried rather than rebuilt from their object numbers. An earlier
	// cut passed the numbers and rebuilt a reference with `types.NewIndirectRef`, which returns a
	// POINTER — and `DereferenceDict` switches on `case IndirectRef`, so every lookup silently
	// returned nil, `live` was always empty, and every subset deleted the entire AcroForm. No
	// fixture in this package could see it: every form fixture is one page.
	keptDicts := make([]types.Dict, 0, len(keep))
	// The same pages again, with what the structure carry needs of them: which are copies, and
	// which of the copy's annotation objects stands for which of the source's.
	pages := make([]keptPage, 0, len(keep))
	for _, n := range keep {
		leaf := leaves[n-1]
		ref, dic := leaf.ref, leaf.dic
		isClone := false
		var annotOf map[int]types.IndirectRef
		if placed[ref.ObjectNumber.Value()] {
			// A page named twice must become a second OBJECT: pdfcpu refuses a page tree that
			// reaches one object twice (`ErrPageTreeDuplicate`, model/recursion.go:80, on the WRITE path as well as validation), so the
			// clone is mandatory rather than a nicety. `DuplicatePage` is the caller that needs it.
			cloneRef, mapping, cerr := clonePage(xt, dic, pagesRef)
			if cerr != nil {
				return false, cerr
			}
			ref, isClone, annotOf = *cloneRef, true, mapping
			if dic = derefDict(xt, ref); dic == nil {
				return false, fmt.Errorf("pdfops: a duplicated page could not be read back")
			}
		}
		placed[ref.ObjectNumber.Value()] = true
		keptPages[ref.ObjectNumber.Value()] = true
		keptDicts = append(keptDicts, dic)
		pages = append(pages, keptPage{ref: ref, dic: dic, clone: isClone, annotOf: annotOf})
		for _, k := range inheritableKeys {
			if v, ok := leaf.inh[k]; ok {
				dic[k] = v
			}
		}
		dic["Parent"] = pagesRef
		kids = append(kids, ref)
	}

	// The root /Pages node is reused so the catalog's reference stays valid, but its contents are
	// replaced wholesale — anything it carried was either inherited (now materialized onto the
	// pages) or is stale.
	pagesNode := derefDict(xt, pagesRef)
	if pagesNode == nil {
		return false, fmt.Errorf("pdfops: this document's page tree root could not be read")
	}
	for k := range pagesNode {
		delete(pagesNode, k)
	}
	pagesNode["Type"] = types.Name("Pages")
	pagesNode["Kids"] = kids
	pagesNode["Count"] = types.Integer(len(kids))
	ctx.PageCount = len(kids)

	// **The carry runs BEFORE the allowlist and its keys are restored after it.** That order is what
	// makes a refusal free: the prune needs `/StructTreeRoot` still on the catalog to read the tree
	// at all, and anything it leaves behind becomes unreachable the moment the allowlist drops the
	// key — so a refused carry writes exactly the document this primitive wrote before P02.S04b,
	// with no rollback and no second pass.
	carried := false
	var treeRoot, markInfo types.Object
	title := ""
	var showTitle *bool
	if carry {
		treeRoot, markInfo = root["StructTreeRoot"], root["MarkInfo"]
		title = documentTitle(ctx, root)
		showTitle = displayDocTitle(ctx)
		var cerr error
		if carried, cerr = carryStructure(ctx, root, pages); cerr != nil {
			return false, cerr
		}
	}

	for k := range root {
		if !catalogAllowlist[k] {
			delete(root, k)
		}
	}
	if err := pruneAcroForm(xt, root, keptDicts); err != nil {
		return false, err
	}
	if err := pruneNames(xt, root, keptPages); err != nil {
		return false, err
	}
	unlinkDestinations(xt, keptDicts, keptPages)
	// The trailer's /ID[0] is a PERMANENT document identifier: pdfcpu preserves it and mints only
	// /ID[1] (`write.go`'s ensureFileID), where the fresh-context path had no ID at all and minted
	// both. Carrying it would tie every extract and split to the document it came from.
	ctx.ID = nil
	// The document /Info does not survive a subset today — measured, a title and nib's own
	// `NibFlags` are both gone — and it travels into every extract and split artifact if kept.
	if ctx.Info != nil {
		if info := derefDict(xt, *ctx.Info); info != nil {
			for k := range info {
				delete(info, k)
			}
		}
	}

	if !carried {
		return false, nil
	}
	root["StructTreeRoot"] = treeRoot
	// `/MarkInfo` goes WITH the tree and is not invented: a document whose tree carried no
	// `/MarkInfo /Marked true` does not gain one here.
	if markInfo != nil {
		root["MarkInfo"] = markInfo
	}
	// **A failure here drops the carry rather than the operation.** Its errors are nib's own and it
	// runs after the structure keys are back, so returning one would add a failure mode
	// `carryStructure`'s header disclaims and leave state to roll back; taking the keys away again
	// produces exactly the honest loss the rest of the carry produces.
	if !carryTitleFloor(ctx, root, title, showTitle) {
		delete(root, "StructTreeRoot")
		delete(root, "MarkInfo")
		delete(root, "Metadata")
		delete(root, "ViewerPreferences")
		return false, nil
	}
	return true, nil
}

// clonePage produces a genuinely separate page object: its own dictionary, its own /Annots array,
// and its own annotation objects, each pointing back at the clone.
//
// The annotations cannot be shared. An annotation's /P is a single reference to one page, and its
// /StructParent is a single ParentTree key, so one annotation object on two pages is contradictory
// on both counts — and a shared widget would put one form field on two pages. Content streams,
// resources and everything they reach ARE shared: they are read-only to this operation.
// It also reports WHICH copy stands for which original. An `OBJR` in the structure tree names an
// annotation, and the copy of that OBJR has to name the copy of the annotation (P02.S04b) — a
// mapping only this function can produce, since it is the only place the correspondence exists.
func clonePage(xt *model.XRefTable, src types.Dict, parent types.IndirectRef) (*types.IndirectRef, map[int]types.IndirectRef, error) {
	dic, ok := src.Clone().(types.Dict)
	if !ok {
		return nil, nil, fmt.Errorf("pdfops: a page could not be duplicated")
	}
	annots := derefArray(xt, src["Annots"])
	delete(dic, "Annots")
	dic["Parent"] = parent

	ref, err := xt.IndRefForNewObject(dic)
	if err != nil {
		return nil, nil, err
	}
	annotOf := map[int]types.IndirectRef{}
	if len(annots) > 0 {
		cloned := make(types.Array, 0, len(annots))
		for _, a := range annots {
			ad := derefDict(xt, a)
			if ad == nil {
				continue
			}
			nd, isDict := ad.Clone().(types.Dict)
			if !isDict {
				continue
			}
			nd["P"] = *ref
			nref, aerr := xt.IndRefForNewObject(nd)
			if aerr != nil {
				return nil, nil, aerr
			}
			if ar, isRef := a.(types.IndirectRef); isRef {
				annotOf[ar.ObjectNumber.Value()] = *nref
			}
			cloned = append(cloned, *nref)
		}
		if len(cloned) > 0 {
			dic["Annots"] = cloned
		}
	}
	return ref, annotOf, nil
}

// dropSignature removes every signature from the document — the fields, their widgets, and the
// AcroForm entries that could reach them by another route — so a page operation still ERASES a
// signature rather than leaving one behind as rubble.
//
// # Why erasing is preserved deliberately rather than allowed to drift
//
// An in-place rewrite keeps the signature dictionary and its /Contents blob while invalidating the
// /ByteRange, which would move `Collect` and `RemovePages` from "erases" to "breaks". That reads
// like an improvement and is not: four gates key on a document having no signature, and all four
// would change behaviour silently. ADR-013's three `DocHash` anchors are gated on the document
// being unsigned, so they would go QUIET rather than fire; `p2p.ContributionProgress` hard-refuses
// `sign.Invalid`, so a reordered ceremony document could never be contributed to again;
// `sign.SignApproval` refuses a document whose AcroForm still carries a /DocMDP reference, which
// today's rebuild removes; and `DropUAIdentificationUnlessSigned` reads "signed" at three sites.
// The client also already tells the user, before every page operation, that the result "will not
// show that it was ever signed".
//
// Whether an edited signed document ought to read `invalid` or `unsigned` is SETTLED, not open:
// `/pending 455` closed at v1.128.98 deciding that the refusal OBSERVES the bytes the server holds
// against the result it was handed, rather than predicting from which primitive a route uses — and
// it explicitly refused the alternative of making every route produce `invalid`, because redaction
// assembles a new document and cannot preserve a signature at all. Keeping `erases` here is what
// that decision already says.
//
// # It takes EVERY page, not the kept ones, and runs before anything is cloned
//
// `clonePage` deep-copies a page's annotations into fresh objects. A signature widget stripped only
// from the pages that survive would be copied onto a duplicate and live on there, with its /V still
// reaching the blob.
//
// # Three routes to a signature, not one
//
// The obvious one is a `/FT /Sig` field in `/AcroForm` `/Fields`. The second is a field MERGED with
// its widget and present only in a page's `/Annots` — `/Fields` is where a well-formed document
// lists it, not where the annotation lives. The third is `/XFA`, which carries a whole XML copy of
// the form dataset and can hold its own signature; it is dropped outright, because an XFA dataset
// describing fields on pages this operation removed is wrong whatever else is true of it, and nib's
// own scanner already treats XFA as active content.
func dropSignature(xt *model.XRefTable, root types.Dict, pages []types.Dict) {
	form := derefDict(xt, root["AcroForm"])
	doomed := map[int]bool{}

	var mark func(ref types.IndirectRef, depth int)
	mark = func(ref types.IndirectRef, depth int) {
		if depth > 50 || doomed[ref.ObjectNumber.Value()] {
			return
		}
		d := derefDict(xt, ref)
		if d == nil {
			return
		}
		doomed[ref.ObjectNumber.Value()] = true
		for _, k := range derefArray(xt, d["Kids"]) {
			if kr, ok := k.(types.IndirectRef); ok {
				mark(kr, depth+1)
			}
		}
	}
	var scan func(ref types.IndirectRef, depth int)
	scan = func(ref types.IndirectRef, depth int) {
		if depth > 50 {
			return
		}
		d := derefDict(xt, ref)
		if d == nil {
			return
		}
		if nameVal(d, "FT") == "Sig" {
			mark(ref, 0)
			return
		}
		for _, k := range derefArray(xt, d["Kids"]) {
			if kr, ok := k.(types.IndirectRef); ok {
				scan(kr, depth+1)
			}
		}
	}
	if form != nil {
		for _, f := range derefArray(xt, form["Fields"]) {
			if fr, ok := f.(types.IndirectRef); ok {
				scan(fr, 0)
			}
		}
	}
	// A signature merged with its widget and never listed in /Fields.
	for _, page := range pages {
		for _, a := range derefArray(xt, page["Annots"]) {
			ar, ok := a.(types.IndirectRef)
			if !ok {
				continue
			}
			if ad := derefDict(xt, ar); ad != nil && nameVal(ad, "FT") == "Sig" {
				mark(ar, 0)
			}
		}
	}
	if len(doomed) == 0 {
		if form != nil {
			delete(form, "XFA")
		}
		return
	}
	for _, page := range pages {
		stripAnnots(xt, page, doomed)
	}
	if form == nil {
		return
	}
	form["Fields"] = keepFields(xt, derefArray(xt, form["Fields"]), func(nr int, _ types.Dict) bool {
		return !doomed[nr]
	}, 0)
	delete(form, "SigFlags")
	delete(form, "XFA")
	pruneFieldRefs(xt, form, "CO", doomed)
}

// stripAnnots removes the named annotations from a page, deleting /Annots when it empties.
func stripAnnots(xt *model.XRefTable, page types.Dict, doomed map[int]bool) {
	annots := derefArray(xt, page["Annots"])
	if len(annots) == 0 {
		return
	}
	live := make(types.Array, 0, len(annots))
	for _, a := range annots {
		if ar, ok := a.(types.IndirectRef); ok && doomed[ar.ObjectNumber.Value()] {
			continue
		}
		live = append(live, a)
	}
	if len(live) == 0 {
		delete(page, "Annots")
		return
	}
	page["Annots"] = live
}

// keepFields rebuilds a field subtree, REWRITING each surviving field's /Kids rather than only
// filtering the top level.
//
// Filtering only the top level is what made this a re-anchoring bug rather than untidiness: a field
// kept because one of its widgets is on a surviving page still referenced its other widgets, whose
// /P names a page this operation dropped — and pdfcpu writes by reachability, so the dropped page's
// dictionary and its /Contents went into the output. "Removed" would have meant "hidden".
func keepFields(xt *model.XRefTable, fields types.Array, keep func(nr int, d types.Dict) bool, depth int) types.Array {
	if depth > 50 {
		return nil
	}
	out := make(types.Array, 0, len(fields))
	for _, f := range fields {
		fr, ok := f.(types.IndirectRef)
		if !ok {
			continue
		}
		d := derefDict(xt, fr)
		if d == nil {
			continue
		}
		kids := derefArray(xt, d["Kids"])
		if len(kids) > 0 {
			live := keepFields(xt, kids, keep, depth+1)
			if len(live) == 0 {
				continue
			}
			d["Kids"] = live
			out = append(out, f)
			continue
		}
		if keep(fr.ObjectNumber.Value(), d) {
			out = append(out, f)
		}
	}
	return out
}

// pruneFieldRefs filters an AcroForm array of field references (/CO) to the ones still present.
func pruneFieldRefs(xt *model.XRefTable, form types.Dict, key string, doomed map[int]bool) {
	arr := derefArray(xt, form[key])
	if len(arr) == 0 {
		return
	}
	live := make(types.Array, 0, len(arr))
	for _, o := range arr {
		if r, ok := o.(types.IndirectRef); ok && doomed[r.ObjectNumber.Value()] {
			continue
		}
		live = append(live, o)
	}
	if len(live) == 0 {
		delete(form, key)
		return
	}
	form[key] = live
}

// pruneAcroForm keeps only the fields that still have a widget on a surviving page, mirroring what
// pdfcpu's `migrateFields` rebuilt for free when the destination context was built from nothing.
func pruneAcroForm(xt *model.XRefTable, root types.Dict, keptPages []types.Dict) error {
	form := derefDict(xt, root["AcroForm"])
	if form == nil {
		return nil
	}
	live := map[int]bool{}
	for _, page := range keptPages {
		for _, a := range derefArray(xt, page["Annots"]) {
			if ar, ok := a.(types.IndirectRef); ok {
				live[ar.ObjectNumber.Value()] = true
			}
		}
	}
	kept := keepFields(xt, derefArray(xt, form["Fields"]), func(nr int, _ types.Dict) bool {
		return live[nr]
	}, 0)
	if len(kept) == 0 {
		delete(root, "AcroForm")
		return nil
	}
	form["Fields"] = kept
	dead := map[int]bool{}
	for _, o := range derefArray(xt, form["CO"]) {
		if r, ok := o.(types.IndirectRef); ok && !live[r.ObjectNumber.Value()] {
			dead[r.ObjectNumber.Value()] = true
		}
	}
	pruneFieldRefs(xt, form, "CO", dead)
	return nil
}

// unlinkDestinations removes a link's destination when it names a page this subset dropped.
//
// A dangling destination is not merely untidy here. The annotation lives on a page that SURVIVES,
// so the dropped page's dictionary stays reachable through it — and pdfcpu writes by reachability,
// which would put the removed page's /Contents back into the output. pdfcpu's own migration does
// not solve this either: it patches the reference through a lookup the dropped page is absent from,
// which yields `0 0 R`.
func unlinkDestinations(xt *model.XRefTable, keptPages []types.Dict, kept map[int]bool) {
	for _, page := range keptPages {
		for _, a := range derefArray(xt, page["Annots"]) {
			ad := derefDict(xt, a)
			if ad == nil {
				continue
			}
			if _, has := ad["Dest"]; has && !destNamesAKeptPage(xt, ad["Dest"], kept) {
				delete(ad, "Dest")
			}
			action := derefDict(xt, ad["A"])
			if action == nil {
				continue
			}
			if _, has := action["D"]; has && !destNamesAKeptPage(xt, action["D"], kept) {
				delete(ad, "A")
			}
		}
	}
}

// pruneNames reduces the catalog's name trees to /Dests alone and drops the destinations that name
// a page this subset did not keep.
//
// The trees are edited through `ctx.Names` rather than the catalog dictionary, because
// `BindNameTrees` re-materializes that cache onto the catalog at write time — an edit made directly
// to the dictionary would be discarded. But the cache is not the whole story, which is finding the
// slice paid for: `BindNameTrees` only ever UPDATES the key it knows about and never REMOVES a
// sibling, so clearing the cache is sufficient only when /Names is deleted outright, which happens
// only when no destination survives. With one surviving destination, /EmbeddedFiles rode out with
// it and an extract, split or redaction shipped the source's attachments — measured, with the
// payload readable in the output bytes. So the catalog's own /Names dictionary is rebuilt too.
func pruneNames(xt *model.XRefTable, root types.Dict, keptPages map[int]bool) error {
	xt.Dests = nil
	for name := range xt.Names {
		if name != "Dests" {
			delete(xt.Names, name)
		}
	}
	if names := derefDict(xt, root["Names"]); names != nil {
		for k := range names {
			if k != "Dests" {
				delete(names, k)
			}
		}
	}
	node := xt.Names["Dests"]
	if node == nil {
		delete(root, "Names")
		return nil
	}
	var doomed []string
	if err := node.Process(xt, func(_ *model.XRefTable, k string, v *types.Object) error {
		if !destNamesAKeptPage(xt, *v, keptPages) {
			doomed = append(doomed, k)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(doomed)
	for _, k := range doomed {
		if _, _, err := node.Remove(xt, k); err != nil {
			return err
		}
	}
	// Emptiness is asked of what REMAINS, not inferred from how much was removed. An earlier cut
	// compared the number removed against the number left — equal whenever exactly half the
	// destinations pointed at dropped pages, so a two-destination document that lost one lost the
	// whole tree.
	left, err := node.KeyList()
	if err != nil {
		return err
	}
	if len(left) == 0 {
		delete(xt.Names, "Dests")
		delete(root, "Names")
		return nil
	}
	if names := derefDict(xt, root["Names"]); names != nil && len(names) == 0 {
		delete(root, "Names")
	}
	return nil
}

// destNamesAKeptPage resolves a named destination to the page it points at. Both shapes are handled:
// the bare array whose first element is the page, and the dictionary carrying it under /D.
func destNamesAKeptPage(xt *model.XRefTable, o types.Object, keptPages map[int]bool) bool {
	d, err := xt.Dereference(o)
	if err != nil || d == nil {
		return false
	}
	arr, ok := d.(types.Array)
	if !ok {
		dict, isDict := d.(types.Dict)
		if !isDict {
			return false
		}
		if arr = derefArray(xt, dict["D"]); arr == nil {
			return false
		}
	}
	if len(arr) == 0 {
		return false
	}
	ref, ok := arr[0].(types.IndirectRef)
	if !ok {
		return false // a page given by index rather than reference cannot be re-resolved
	}
	return keptPages[ref.ObjectNumber.Value()]
}
