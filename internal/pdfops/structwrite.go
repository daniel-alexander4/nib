package pdfops

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure tree's write half — `PLAN-accessibility.md` P05.S03, D8.
//
// # What is built, and what is deliberately not
//
// D8 asks for `/ParentTree`, `/StructParents` and MCIDs to be *invariants of the model rather than
// maintained by each caller*. The invariants are in `structcheck.go`; this is the one mutation that
// keeps them.
//
// **Only ADD is built.** The slice's acceptance named add, remove and re-parent; removing and
// re-parenting have no caller — every tree writer builds a new tree, which adds — and this repo has a standing
// lesson about building the other two anyway. `/pending 442` deleted exported crypto that nothing
// called, which had cost a fix nobody ran plus an exemption row that outlived its reason. An
// untested mutation of a structure tree is a worse version of the same bet, because its failures are
// silent: a tree that still parses and describes the wrong content.
//
// So the clause is met for the operation that exists and recorded as unbuilt for the two that do
// not, rather than met by three mutations of which two are theatre.

// addMarkedElementUnder adds a structure element that owns one marked-content sequence on a page,
// under parent, and keeps every invariant `checkStructConsistency` tests.
//
// It returns the MCID the caller must write into the page's `BDC` property list — allocated here
// rather than passed in, because the MCID and the `/ParentTree` slot are the same number and a
// caller that chose one could disagree with the other.
//
// # What it maintains, and why each is not optional
//
//   - **The page's `/StructParents`**, created when absent. Without it the page names no ParentTree
//     row and the element it just gained is unreachable from the page's side.
//   - **The ParentTree array for that key**, extended to cover the new MCID. The array is indexed BY
//     MCID, so adding MCID 7 to a four-slot array means growing it to eight, not appending.
//   - **The element's `/K` and `/Pg`**, so the tree's side names the same content the ParentTree's
//     side does. The two directions are stored separately and nothing but this keeps them equal.
//
// # The parent — `PLAN-accessibility.md` P06.S02
//
// A nil parent means the tree root. Real structure nests: a list is `L` containing `LI` containing `Lbl` and `LBody`, and an
// element whose `/P` names the root while its parent's `/K` names it is a tree that disagrees with
// itself in the two directions a reader can walk it.
//
// **The parent is given rather than inferred.** Inferring one from page geometry is P08's job and
// would be a guess here; the caller walking an AST knows exactly.
func addMarkedElementUnder(ctx *model.Context, tree *structTree, pageNr int, structType string,
	parent *types.IndirectRef) (mcid int, elemRef *types.IndirectRef, err error) {

	if structType == "" {
		return 0, nil, fmt.Errorf("pdfops: a structure element needs an /S")
	}
	pageDict, pageRef, err := tree.page(ctx, pageNr)
	if err != nil {
		return 0, nil, err
	}

	// The page's key, created if the page has none.
	key, hasKey := 0, false
	if sp, ok := pageDict["StructParents"].(types.Integer); ok {
		key, hasKey = sp.Value(), true
	}
	slots, isSingle, _ := parentTreeKey(ctx, tree, key)
	if hasKey && isSingle {
		return 0, nil, fmt.Errorf("pdfops: page %d declares /StructParents %d and that "+
			"ParentTree entry is a single element, not an array — this document's tree is "+
			"already inconsistent and adding to it would hide that", pageNr, key)
	}
	if !hasKey {
		key, slots = allocParentTreeKey(ctx, tree), 0
		pageDict["StructParents"] = types.Integer(key)
	}

	// The MCID is the next free slot of that key's array.
	mcid = slots

	// Build the element. `/P` is the parent the caller named, or the tree root when it named none.
	//
	// **`/P` is REQUIRED, and omitting it produces a document that looks tagged and is not.**
	// Measured: without it veraPDF reports every content item as `{mcid:0}` — marked, so the
	// bracketing worked — and still fails ua1 7.1 t3, *content shall be marked as Artifact or
	// tagged as real content*, because an element with no parent is not part of the tree the MCID
	// is supposed to resolve into. The failure names the content rather than the element, which is
	// why it reads as a wrapping problem and is not one. Every element of a real LibreOffice tree
	// carries `/P` (36 of 36, measured at S02).
	parentRef := parent
	if parentRef == nil {
		rootRef, rerr := structTreeRootRef(ctx)
		if rerr != nil {
			return 0, nil, rerr
		}
		parentRef = rootRef
	}
	elem := types.Dict{
		"Type": types.Name("StructElem"),
		"S":    types.Name(structType),
		"P":    *parentRef,
		"Pg":   *pageRef,
		"K":    types.Array{types.Integer(mcid)},
	}
	ref, err := ctx.IndRefForNewObject(elem)
	if err != nil {
		return 0, nil, err
	}

	// Both directions, in one place: the tree's side and the ParentTree's side.
	if parent == nil {
		if err := appendToRootKids(ctx, tree, *ref); err != nil {
			return 0, nil, err
		}
	} else if err := appendToElementKids(ctx, *parent, *ref); err != nil {
		return 0, nil, err
	}
	if err := setParentTreeSlot(ctx, tree, key, mcid, *ref); err != nil {
		return 0, nil, err
	}
	return mcid, ref, nil
}

// parentTreeKey reports one key's `/ParentTree` entry — how many slots its array has, or whether it
// is a single reference — and the next key no entry uses.
//
// **It exists because the writers call it once per marked run.** `parentTreeEntries` builds every
// key's slot list, so calling it per run made tagging quadratic in the document: a 20,000-clause
// Markdown file (tier 4d's interrupt fixture) took over twenty minutes to convert where the untagged
// render takes seconds. This walks the same `/Nums` and `/Kids` and dereferences only the one value
// asked about.
func parentTreeKey(ctx *model.Context, tree *structTree, key int) (slots int, single bool, next int) {
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > maxStructDepth {
			return
		}
		d, err := ctx.DereferenceDict(o)
		if err != nil || d == nil {
			return
		}
		if nums, e := ctx.DereferenceArray(d["Nums"]); e == nil && nums != nil {
			for i := 0; i+1 < len(nums); i += 2 {
				n, ok := nums[i].(types.Integer)
				if !ok {
					continue
				}
				if n.Value() >= next {
					next = n.Value() + 1
				}
				if n.Value() != key {
					continue
				}
				val := nums[i+1]
				if arr, ae := ctx.DereferenceArray(val); ae == nil && arr != nil {
					slots = len(arr)
					continue
				}
				if _, isInd := val.(types.IndirectRef); isInd {
					single = true
				}
			}
		}
		if kids, e := ctx.DereferenceArray(d["Kids"]); e == nil && kids != nil {
			for _, k := range kids {
				walk(k, depth+1)
			}
		}
	}
	walk(tree.root["ParentTree"], 0)
	return slots, single, next
}

// appendToRootKids adds an element to `/StructTreeRoot`'s `/K`, normalising the single-entry form.
func appendToRootKids(ctx *model.Context, tree *structTree, ref types.IndirectRef) error {
	k, has := tree.root["K"]
	if !has || k == nil {
		tree.root["K"] = types.Array{ref}
		return nil
	}
	if arr, err := ctx.DereferenceArray(k); err == nil && arr != nil {
		// `/K` may itself be an indirect array; update it in place where it is, or the root keeps
		// pointing at the old one.
		if ind, isInd := k.(types.IndirectRef); isInd {
			// The array is its own object, so the root still points at it and the update has to
			// land THERE — assigning to `tree.root["K"]` would leave the old object in place and
			// silently lose the new element. Same shape as `tagcarry.go`'s `e.Object = sd`.
			e, found := ctx.XRefTable.FindTableEntryForIndRef(&ind)
			if !found || e == nil {
				return fmt.Errorf("pdfops: /StructTreeRoot's /K names object %d, which the xref "+
					"table does not have", ind.ObjectNumber.Value())
			}
			e.Object = append(arr, ref)
			return nil
		}
		tree.root["K"] = append(arr, ref)
		return nil
	}
	// A single entry: promote to an array containing it and the new element.
	tree.root["K"] = types.Array{k, ref}
	return nil
}

// parentTreeDict resolves `/ParentTree` to its dictionary, creating an empty one if the tree has
// none — the one place both entry shapes go through, so the nested-number-tree refusal below cannot
// reach one writer and miss the other (ADR-009).
func parentTreeDict(ctx *model.Context, tree *structTree) (types.Dict, error) {
	ptObj, has := tree.root["ParentTree"]
	if !has {
		ptRef, err := ctx.IndRefForNewObject(types.Dict{"Nums": types.Array{}})
		if err != nil {
			return nil, err
		}
		tree.root["ParentTree"] = *ptRef
		ptObj = *ptRef
	}
	pt, err := ctx.DereferenceDict(ptObj)
	if err != nil || pt == nil {
		return nil, fmt.Errorf("pdfops: /ParentTree does not resolve to a dictionary: %w", err)
	}
	if _, nested := pt["Kids"]; nested {
		return nil, fmt.Errorf("pdfops: this document's /ParentTree is a NESTED number tree, which " +
			"the write half does not rebalance — refusing rather than writing an entry that a " +
			"reader following /Kids would never find")
	}
	return pt, nil
}

// setParentTreeSlot puts ref at index mcid of the array for key, growing the array and creating the
// entry as needed.
//
// **Growing means filling**, not appending: the array is indexed BY MCID, so making room for MCID 7
// in a four-slot array creates slots 4, 5 and 6 as nulls. An append would put the element at index 4
// and every later lookup would find the wrong element or none.
//
// **`addMarkedElementUnder` never exercises that**, and saying so is the point: it allocates `mcid` as
// `len(arr)`, so fill and append coincide on every call it makes — swapping one for the other leaves
// its tests green. The fill is the general contract of this helper, which any writer placing content
// whose MCIDs already exist and are not contiguous needs, so it is driven directly by
// `TestAGapInTheParentTreeArrayIsFilledNotAppended` rather than left as an untested claim.
func setParentTreeSlot(ctx *model.Context, tree *structTree, key, mcid int, ref types.IndirectRef) error {
	pt, err := parentTreeDict(ctx, tree)
	if err != nil {
		return err
	}
	nums, _ := ctx.DereferenceArray(pt["Nums"])

	// Find the key's slot in the flat [key, value, key, value…] array.
	at := -1
	for i := 0; i+1 < len(nums); i += 2 {
		if n, ok := nums[i].(types.Integer); ok && n.Value() == key {
			at = i + 1
			break
		}
	}
	var arr types.Array
	if at >= 0 {
		if a, aerr := ctx.DereferenceArray(nums[at]); aerr == nil && a != nil {
			arr = a
		}
	}
	for len(arr) <= mcid {
		arr = append(arr, nil)
	}
	arr[mcid] = ref
	if at >= 0 {
		nums[at] = arr
	} else {
		nums = append(nums, types.Integer(key), arr)
		claimParentTreeKey(ctx, tree, key)
	}
	pt["Nums"] = nums
	return nil
}

// clearParentTreeSlot empties index mcid of the array for key — the content that slot named is no
// longer owned by any element (`PLAN-accessibility.md` P09.S03's artifact edit). A tree with no
// ParentTree, no entry for key, or a slot past the array's end has nothing to clear, and one is not
// created to be emptied.
func clearParentTreeSlot(ctx *model.Context, tree *structTree, key, mcid int) error {
	if _, has := tree.root["ParentTree"]; !has {
		return nil
	}
	pt, err := parentTreeDict(ctx, tree)
	if err != nil {
		return err
	}
	nums, _ := ctx.DereferenceArray(pt["Nums"])
	for i := 0; i+1 < len(nums); i += 2 {
		n, ok := nums[i].(types.Integer)
		if !ok || n.Value() != key {
			continue
		}
		arr, aerr := ctx.DereferenceArray(nums[i+1])
		if aerr != nil || arr == nil || mcid < 0 || mcid >= len(arr) {
			return nil
		}
		arr = append(types.Array{}, arr...)
		arr[mcid] = nil
		// An indirect slot array is updated where it lives, or the entry keeps pointing at the old one.
		if ind, isInd := nums[i+1].(types.IndirectRef); isInd {
			if en, found := ctx.XRefTable.FindTableEntryForIndRef(&ind); found && en != nil {
				en.Object = arr
				return nil
			}
		}
		nums[i+1] = arr
		pt["Nums"] = nums
		return nil
	}
	return nil
}

// structTreeRootRef returns the indirect reference to `/StructTreeRoot`, which every element this
// package creates needs for its `/P`.
//
// It reads the catalog rather than taking a reference the caller passes, because the one thing a
// caller could get wrong here is silent: an element whose `/P` names some other object still parses,
// still validates, and describes nothing.
func structTreeRootRef(ctx *model.Context) (*types.IndirectRef, error) {
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return nil, err
	}
	ref, ok := cat["StructTreeRoot"].(types.IndirectRef)
	if !ok {
		return nil, fmt.Errorf("pdfops: /StructTreeRoot is not an indirect reference, so an " +
			"element cannot name it as its parent")
	}
	return &ref, nil
}

// addGroupingElement creates an element that has no marked content of its own — an `L` holding
// `LI`s, or an `LI` holding an `Lbl` and an `LBody`.
//
// **It takes no page and registers no ParentTree entry**, because it owns no MCID. A grouping
// element's `/Pg` is optional and its children carry the page; writing one here would name a page
// the element's content is not on when its children span a break.
func addGroupingElement(ctx *model.Context, tree *structTree, structType string,
	parent *types.IndirectRef) (*types.IndirectRef, error) {

	parentRef := parent
	if parentRef == nil {
		rootRef, err := structTreeRootRef(ctx)
		if err != nil {
			return nil, err
		}
		parentRef = rootRef
	}
	ref, err := ctx.IndRefForNewObject(types.Dict{
		"Type": types.Name("StructElem"),
		"S":    types.Name(structType),
		"P":    *parentRef,
		"K":    types.Array{},
	})
	if err != nil {
		return nil, err
	}
	if parent == nil {
		if err := appendToRootKids(ctx, tree, *ref); err != nil {
			return nil, err
		}
		return ref, nil
	}
	return ref, appendToElementKids(ctx, *parent, *ref)
}

// appendToElementKids adds a child to an element's `/K`, normalising the forms `/K` may take: a
// single entry, a direct array, or an indirect array.
func appendToElementKids(ctx *model.Context, parent, child types.IndirectRef) error {
	e, found := ctx.XRefTable.FindTableEntryForIndRef(&parent)
	if !found || e == nil || e.Object == nil {
		return fmt.Errorf("pdfops: the parent element (object %d) is not in the xref table",
			parent.ObjectNumber.Value())
	}
	d, ok := e.Object.(types.Dict)
	if !ok {
		return fmt.Errorf("pdfops: the parent element (object %d) is not a dictionary",
			parent.ObjectNumber.Value())
	}
	k, has := d["K"]
	if !has || k == nil {
		d["K"] = types.Array{child}
		e.Object = d
		return nil
	}
	if arr, aerr := ctx.DereferenceArray(k); aerr == nil && arr != nil {
		if ind, isInd := k.(types.IndirectRef); isInd {
			ke, kfound := ctx.XRefTable.FindTableEntryForIndRef(&ind)
			if !kfound || ke == nil {
				return fmt.Errorf("pdfops: an element's /K names object %d, which is not in the "+
					"xref table", ind.ObjectNumber.Value())
			}
			ke.Object = append(arr, child)
			return nil
		}
		d["K"] = append(arr, child)
		e.Object = d
		return nil
	}
	d["K"] = types.Array{k, child}
	e.Object = d
	return nil
}

// addMCIDTo allocates another marked-content id on a page and gives it to an element that already
// exists — `PLAN-accessibility.md` P06.S02.
//
// **A paragraph that wraps to four lines is four runs and ONE element**, and the element owns all
// four MCIDs. Without this an element could own only the first, and every continuation line would
// have to become its own paragraph — which is a tree that says the document has four paragraphs
// where it has one, and a reader navigating by paragraph would be told so.
//
// It keeps both directions in step exactly as `addMarkedElementUnder` does: the MCID joins the element's
// `/K` and the element joins the page's ParentTree array at that index.
func addMCIDTo(ctx *model.Context, tree *structTree, pageNr int, elem types.IndirectRef) (int, error) {
	pageDict, _, err := tree.page(ctx, pageNr)
	if err != nil {
		return 0, err
	}
	sp, ok := pageDict["StructParents"].(types.Integer)
	if !ok {
		return 0, fmt.Errorf("pdfops: page %d has no /StructParents, so it owns no ParentTree "+
			"entry to add a marked-content id to", pageNr)
	}
	key := sp.Value()
	mcid, _, _ := parentTreeKey(ctx, tree, key)

	e, found := ctx.XRefTable.FindTableEntryForIndRef(&elem)
	if !found || e == nil || e.Object == nil {
		return 0, fmt.Errorf("pdfops: element object %d is not in the xref table",
			elem.ObjectNumber.Value())
	}
	d, isDict := e.Object.(types.Dict)
	if !isDict {
		return 0, fmt.Errorf("pdfops: element object %d is not a dictionary",
			elem.ObjectNumber.Value())
	}
	kids, _ := ctx.DereferenceArray(d["K"])
	d["K"] = append(kids, types.Integer(mcid))
	e.Object = d

	return mcid, setParentTreeSlot(ctx, tree, key, mcid, elem)
}

// setParentTreeSingle writes a `/ParentTree` entry whose value is a SINGLE reference —
// `PLAN-accessibility.md` P06.S07.
//
// # The two shapes, which P05.S03 modelled and nothing had yet written
//
// `/ParentTree` maps a key to one of two different things, and which one depends on who owns the
// key:
//
//   - a PAGE's key comes from `/StructParents` (plural) and its value is an ARRAY indexed by MCID,
//     because one page holds many marked-content sequences. That is `setParentTreeSlot`.
//   - an ANNOTATION's key comes from `/StructParent` (singular) and its value is ONE reference,
//     because one annotation belongs to exactly one element. That is this.
//
// Writing an array where a reader expects a reference produces a tree that resolves to the wrong
// kind of object — `readStructTree` distinguishes them and `checkStructConsistency` reports the
// mismatch, which is why the distinction is two functions rather than one with a flag.
func setParentTreeSingle(ctx *model.Context, tree *structTree, key int, ref types.IndirectRef) error {
	pt, err := parentTreeDict(ctx, tree)
	if err != nil {
		return err
	}
	nums, _ := ctx.DereferenceArray(pt["Nums"])
	for i := 0; i+1 < len(nums); i += 2 {
		n, ok := nums[i].(types.Integer)
		if !ok || n.Value() != key {
			continue
		}
		// An existing entry at this key is the caller having picked a key that is already taken.
		// Overwriting it would silently re-point whatever owned it at this element.
		return fmt.Errorf("pdfops: /ParentTree key %d is already taken — overwriting it would "+
			"re-point whatever owns it at a different element", key)
	}
	pt["Nums"] = append(nums, types.Integer(key), ref)
	claimParentTreeKey(ctx, tree, key)
	return nil
}

// allocParentTreeKey returns a `/ParentTree` key nothing has claimed, so an annotation's `/StructParent`
// or a page's `/StructParents` cannot collide with anything already in the document. It is the ONE key
// allocator (ADR-009); it does not record the key — the entry's writer does, through
// `claimParentTreeKey`.
//
// # Above everything, never into a hole (`/pending 503`)
//
// The key it returns is past the highest of four things, because each is a claim on the key space a
// hole-filling search cannot see:
//
//   - **every key the ParentTree has**, in both shapes — a page array at key 3 and an annotation
//     reference at key 3 are the same entry, and the second write would destroy the first;
//   - **`/ParentTreeNextKey`**, the document's own statement of which keys are spent. A key below it with
//     no entry is not free: the tool that wrote it may have removed the entry and not the object naming
//     it, and another tool honouring the key will allocate from it next. The first cut filled the lowest
//     hole, measured on `taggedFixture`: a form's widget took key 1 while `/ParentTreeNextKey` stayed 1;
//   - **every page's `/StructParents` and every page annotation's `/StructParent`**, which name a key
//     whether or not the ParentTree still has it — reusing one re-points that object at a new element.
//
// **Form XObjects carry a key too, and since P02.S01 they ARE walked.** This paragraph used to
// declare them as residue — *"finding them means walking every resource dictionary"* — and the
// carry is what made it bite: `carryTagsThroughNUp` writes `/StructParents` onto the form XObject it
// builds from each source page, so after an n-up every key in the tree is claimed by an XObject and
// by nothing else, and an allocator blind to them would hand out a key that is already spent. The
// walk is `parentTreeOwners` (`structcomplete.go`), shared with the completeness predicate so the
// question "who owns this key" has one answer (ADR-009) rather than two that can disagree.
//
// # Once per tree
//
// The walk runs on the first call and is cached on the tree; every entry written after it raises the
// floor. Walking the ParentTree per annotation cost 2.28 s of `AuthorTaggedForm`'s 6.13 s for a
// 400-field form (profiled, v1.129.128).
func allocParentTreeKey(ctx *model.Context, tree *structTree) int {
	if tree.keyFloorKnown {
		return tree.keyFloor
	}
	_, _, floor := parentTreeKey(ctx, tree, -1)
	if nk, ok := pdfNumber(ctx.XRefTable, tree.root["ParentTreeNextKey"]); ok && int(nk) > floor {
		floor = int(nk)
	}
	for key := range parentTreeOwners(ctx) {
		if key+1 > floor {
			floor = key + 1
		}
	}
	tree.keyFloor, tree.keyFloorKnown = floor, true
	return floor
}

// claimParentTreeKey records that key now has a `/ParentTree` entry: `/ParentTreeNextKey` moves past it,
// and so does the cached floor. Called by the two writers that create an entry (`setParentTreeSlot`,
// `setParentTreeSingle`), so no entry can be created without it.
//
// **`/ParentTreeNextKey` is optional and nib writes it anyway once it adds a key.** A reader may ignore
// it — a LibreOffice tree carries none — but a tool that honours it allocates from it, and a stale value
// hands that tool a key nib has just used.
func claimParentTreeKey(ctx *model.Context, tree *structTree, key int) {
	next := key + 1
	if tree.keyFloorKnown && tree.keyFloor < next {
		tree.keyFloor = next
	}
	if cur, ok := pdfNumber(ctx.XRefTable, tree.root["ParentTreeNextKey"]); !ok || int(cur) < next {
		tree.root["ParentTreeNextKey"] = types.Integer(next)
	}
}

// addOBJRTo makes elem describe a whole object — in practice an annotation — rather than a range of
// marked content.
//
// An `OBJR` carries `/Pg` as well as `/Obj`: a reader needs to know which page the object is on to
// place it in reading order, and an annotation dictionary does not say.
func addOBJRTo(ctx *model.Context, elem types.IndirectRef, obj, page types.IndirectRef) error {
	ref, err := ctx.IndRefForNewObject(types.Dict{
		"Type": types.Name("OBJR"),
		"Obj":  obj,
		"Pg":   page,
	})
	if err != nil {
		return err
	}
	return appendToElementKids(ctx, elem, *ref)
}
