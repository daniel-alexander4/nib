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

// What a REAL producer carries that this list drops, measured rather than imagined — `/pending 525`,
// every catalog in veraPDF's PDF_UA-1 corpus, 295 of 297 readable. `/MarkInfo`, `/ViewerPreferences`,
// `/StructTreeRoot` and `/Metadata` (295/295/294/294) are re-added by a successful carry and are not
// a loss. Of the rest: `/Outlines` 276 and `/PageMode` 33 are carried above (`/pending 524`),
// `/OpenAction` 220 is page-indexed and is `/pending 555`, `/PageLayout` 40 and `/OCProperties` 6 are
// carried below, and `/Extensions` and `/NeedsRendering` are one file each.
//
// **The caveat bounds the claim: 283 of the 295 are `veraPDF Test Builder 1.0`**, and the corpus
// holds exactly one LibreOffice file and one Word file. So the scan says a tagged document DOES
// carry these keys in practice; it does not say the distribution resembles a real producer mix. That
// half stays gated on `PLAN-ua-coverage.md` P08's corpus.
//
// # `/OutputIntents` — 26 files, and it is DROPPED, deliberately
//
// It is the ICC profile a PDF/A claim rests on, so dropping it silently de-conforms such a document
// — except that a subset has already de-conformed it, by construction and with no way back. The
// identification lives in `/Metadata`, which is not on this list; and where the structure carry
// re-adds a `/Metadata`, it BUILDS a fresh title-only packet (`structcarry.go:989-1006`), so no
// subset nib performs emits a `pdfaid` by either route. What would be carried is therefore the
// apparatus of a claim the output does not make — and in the measured population that is not a
// figure of speech: **26 of 26 entries are `/S /GTS_PDFA1`**, the subtype whose entire meaning is
// "this is the output intent PDF/A requires" (`pdfa.go:253`). Writing it onto a document whose
// `pdfaid` this operation just removed is ADR-032's rule one key over — an identification carried
// without the verification behind it.
//
// Two lesser reasons point the same way. pdfcpu validates seven keys of an output-intent dictionary
// and rejects nothing else (`validate/xReftable.go:566-612`), and a named search —
// `grep -n "OutputIntent" internal/pdfops/scan.go` — returns nothing, so neither `Scan` nor
// `StripActive` has ever looked inside one; carrying the array whole would widen a surface nib's own
// scanner does not inspect, which is `outlinecarry.go`'s argument for dropping `/A`. And it costs
// nothing a user can act on: the door that wants an output intent makes its own
// (`injectPDFAMarkers`, `pdfa.go:261`), from a vendored sRGB profile, on the way to a claim it also
// writes.
//
// **What would reverse this**, stated so the next reader does not have to re-derive it: a subset
// that emits an identification it verified. Then the intent is the apparatus of a claim the output
// DOES make and it comes back with the claim. The affirmative case that was weighed and lost is
// colour fidelity — an output intent is a base-spec key since PDF 1.4 and not a PDF/A invention
// (`validate/xReftable.go:1135,1190`), so a colour-managed reader could still use it. It lost
// because that benefit is unmeasured on this population while the mixed signal is measured at 26 of
// 26, and under default-deny an unmeasured benefit does not move a key onto the list. The same
// reasoning covers PDF/X's `/GTS_PDFX`, which this corpus contains none of.

// carriedPageLayout is the `/PageLayout` a selection may re-state, or "" for none.
//
// **It is the easy half and the reason is that it makes no per-page claim.** Every value names an
// arrangement — measured, all 40 corpus files say `/OneColumn` — and none of them is about a
// particular page, which is exactly what separates it from `/PageLabels`: "display this
// continuously" survives dropping pages and "this page is page iv" does not.
//
// It is read as a NAME and re-written as one, rather than passed through as the source's object.
// `nameVal` resolves a direct name only, so a `/PageLayout` written as an indirect reference is
// dropped — and that is the point. Passing the source object through would let one reference of the
// source's ride out on a key nobody would think to check, and pdfcpu writes by reachability: the
// `/StructTreeRoot` hazard this file's header describes, on the key that looks least capable of it.
func carriedPageLayout(root types.Dict) string {
	return nameVal(root, "PageLayout")
}

// carriedPageMode is the `/PageMode` a selection may re-state, or "" for none. It takes what
// actually survived rather than the request, because the rule is one sentence: **a value survives
// when the thing it names does.**
//
// `/UseOutlines` is `/pending 524`'s and unchanged — the panel is carried only alongside a real
// outline, or the document opens with the bookmarks pane shut exactly as it did when the tree was
// being dropped. `/UseOC` is new because `/OCProperties` now survives (`optionalcarry.go`), and it
// is the same rule rather than a second one; the earlier note refused it for a reason — "the
// `/OCProperties` the allowlist drops" — that this change makes false.
//
// The other four are refused and stay refused. `/UseAttachments` names embedded files the subset
// drops, and it is not hypothetical: it is 8 of the corpus's 33 `/PageMode` files, so with
// `/UseOutlines`'s 25 the two together are all 33 and nothing measurable was left after 524.
// `/FullScreen` is a presentation mode nobody asked a page selection to turn on, and it is also
// half of a PAIR — `/ViewerPreferences /NonFullScreenPageMode` says what the reader does on exit,
// and `/ViewerPreferences` is off the allowlist (a carrying subset rebuilds it as `/DisplayDocTitle`
// alone), so carrying `/FullScreen` states one half of a setting whose other half this operation
// destroyed. `/UseNone` is the default a reader applies anyway, and `/UseThumbs` names a panel no
// key of this document controls; neither appears in the corpus at all, and naming a value to change
// nothing is the listing this allowlist exists to avoid.
func carriedPageMode(root types.Dict, outlines, optional types.Object) string {
	switch nameVal(root, "PageMode") {
	case "UseOutlines":
		if outlines != nil {
			return "UseOutlines"
		}
	case "UseOC":
		if optional != nil {
			return "UseOC"
		}
	}
	return ""
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

// selectionCeiling bounds how far a selection may AMPLIFY the document it selects from: a keep-list
// longer than this is refused before a single page is cloned.
//
// **The bound is a factor and not an absolute, because amplification is the defect and size is not.**
// A selection reaches this primitive as raw user text on two doors that validate nothing beyond
// splitting on commas — `/api/pages` and `/api/extract` through `splitPages`, and `nib pages
// --keep/--remove` through `splitSel` — so a repeated range costs a few bytes of input per page of
// output. **Measured** (10 pages, 30 annotations, full `Collect`): 10.4 KiB of peak heap and 0.09 ms
// per output page, flat from 1,000 to 200,000 pages — 1,000 pages 12 MiB/77 ms, 10,000 96 MiB/775 ms,
// 100,000 1,039 MiB/8.3 s, 200,000 2,313 MiB/21.3 s. Against ~5 bytes of input per output page that
// is ~2 KiB of peak heap per byte typed, and the `pages` field is bounded only by the 200 MiB
// multipart cap — so half a megabyte of selection text is enough to take the process, and with it
// every open document's unsaved work.
//
// An absolute ceiling would have to sit above the largest legitimate document, which is exactly where
// it stops bounding the cheap-input case. A factor closes that and costs nothing real: every
// selection nib itself generates is a permutation or smaller — `Booklet` is `bookletOrder(n)`, which
// emits exactly n; `DuplicatePage` is `["1-p", "p-"]`, n+1; a reorder is a permutation; every split
// and extract is a subset — so 10× leaves an order of magnitude of headroom over the widest of them.
// The floor keeps a small document usable: 100 copies of a one-page form is a print run, not an attack.
func selectionCeiling(pages int) int {
	const (
		factor = 10
		floor  = 100
	)
	if n := pages * factor; n > floor {
		return n
	}
	return floor
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
	if max := selectionCeiling(len(leaves)); len(keep) > max {
		return false, fmt.Errorf("pdfops: that selection asks for %d pages from a %d-page document; a selection may repeat pages up to %d of them",
			len(keep), len(leaves), max)
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
	var treeRoot, markInfo, outlines types.Object
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
		// **The outline is pruned in this same pre-allowlist window, and it has to be**: nib's own
		// bookmarks name their destination through the `/Dests` name tree, which `pruneNames` is
		// about to prune by the same rule — so this reads the tree while it is still whole and
		// shares `destNamesAKeptPage` with it rather than racing it (`outlinecarry.go`).
		//
		// **It is NOT gated on `carried`**, only on the request. The structure tree and the outline
		// are different documents' worth of truth: most PDFs are untagged, and gating bookmarks on a
		// tag carry would lose them on exactly the documents that only ever had bookmarks.
		//
		// **It IS gated on `carry`, and that is the same decision the tree's is.** The two reasons
		// `collectWithoutStructure` exists both hold here. Redaction: an outline title names the
		// content the raster destroyed, which is the tree's leak one level cruder — a heading is a
		// heading whether it comes from a structure element or a bookmark. Composition: `api.MergeRaw`
		// keeps only the FIRST document's catalog, so a carried outline of part one would be served
		// as the whole composed document's contents page, describing pages that have since moved —
		// which is the original decision's own "sends the reader to the wrong place", now true of the
		// composing doors and no longer of this one.
		outlines = carryOutline(xt, root, keptPages)
	}

	// **Optional content is read OUTSIDE the `carry` gate, and that placement is the decision**
	// (`/pending 525`, `optionalcarry.go`). Dropping `/OCProperties` does not lose a layer, it
	// REVEALS one — measured on two renderers — so the doors that must not carry a description of
	// destroyed content (`collectWithoutStructure`: redaction, and every subset feeding a
	// composition) are precisely the doors where the reveal is worst. It is read here, before the
	// allowlist, for the reason the tree and the outline are: its own subtree has to be walkable
	// while the catalog still names it.
	//
	// The walk needs the pages this selection DROPPED, which is every leaf that was not placed.
	// `keptPages` holds the object number of each original placed AND of each clone made, so a page
	// the selection named twice contributes both numbers to it and neither to this set.
	dropped := make(map[int]bool, len(leaves))
	for _, l := range leaves {
		if nr := l.ref.ObjectNumber.Value(); !keptPages[nr] {
			dropped[nr] = true
		}
	}
	optional := carryOptionalContent(xt, root, dropped)
	// Both of these are pure viewer preferences with no reference and no per-page claim, so neither
	// needs the `carry` gate either; `carriedPageMode` is given what actually survived rather than
	// asked to re-derive it.
	layout := carriedPageLayout(root)
	mode := carriedPageMode(root, outlines, optional)

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
	// Re-added ON TOP of the allowlist, never into it — the shape the structure keys already use.
	// Each of the four appears only when the thing it names survived: an outline that lost every
	// entry leaves no empty root behind, `/OCProperties` is nil where its subtree reached a dropped
	// page, and a `/PageLayout` or `/PageMode` that was not a plain name was never read. So a key
	// this code has never heard of is still dropped, and none of these is ever an empty assertion.
	if outlines != nil {
		root["Outlines"] = outlines
	}
	if optional != nil {
		root["OCProperties"] = optional
	}
	if layout != "" {
		root["PageLayout"] = types.Name(layout)
	}
	if mode != "" {
		root["PageMode"] = types.Name(mode)
	}
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
		// **`nil`, not `xt`, and it is the difference between pruning the tree and corrupting it.**
		// Every object-freeing site in pdfcpu's removal path is guarded by `if xRefTable != nil`
		// (`model/nameTree.go:381-535`) and nothing else there uses the table, so passing nil unlinks
		// the entry and frees nothing — while passing the table calls `DeleteObjectGraph` on the
		// destination's VALUE, and a destination array's graph reaches the PAGE it names and, through
		// the page, everything the page shares. Those numbers go on the free list, `BindNameTrees`
		// hands them straight back out to the kid dictionaries it mints while binding, and the
		// surviving entries end up pointing at the name-tree nodes that replaced them.
		//
		// **Measured on a document with no outline in it at all** — six named destinations, of which
		// the two on dropped pages shared a leaf: before, the output had no `/Dests` tree whatsoever,
		// so all six went, including the four whose pages survived; after, the four survive and
		// resolve. It is a silent loss of every named destination in the document, which is what a
		// link annotation written by Word or LaTeX uses.
		//
		// **Sharing a leaf is what makes it show, and is why it hid for so long.** The freeing only
		// bites once a leaf EMPTIES, because that is what calls `removeKid` — which frees the leaf's
		// dictionary and then the intermediate above it when a single kid remains, and those are the
		// numbers the binder hands back out. Two doomed names in two different leaves free nothing,
		// and the same document comes through clean.
		//
		// Freeing was never needed here: this file turns on pdfcpu writing by REACHABILITY, which is
		// why `unlinkDestinations` unlinks rather than deletes and why the allowlist drops catalog
		// keys rather than their objects. An unreferenced destination array is not written.
		if _, _, err := node.Remove(nil, k); err != nil {
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
