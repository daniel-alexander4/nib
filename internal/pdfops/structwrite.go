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
// re-parenting have no caller — P05.S04 wraps content, which adds — and this repo has a standing
// lesson about building the other two anyway. `/pending 442` deleted exported crypto that nothing
// called, which had cost a fix nobody ran plus an exemption row that outlived its reason. An
// untested mutation of a structure tree is a worse version of the same bet, because its failures are
// silent: a tree that still parses and describes the wrong content.
//
// So the clause is met for the operation that exists and recorded as unbuilt for the two that do
// not, rather than met by three mutations of which two are theatre.

// addMarkedElement adds a structure element that owns one marked-content sequence on a page, and
// keeps every invariant `checkStructConsistency` tests.
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
func addMarkedElement(ctx *model.Context, tree *structTree, pageNr int, structType string) (
	mcid int, elemRef *types.IndirectRef, err error) {

	if structType == "" {
		return 0, nil, fmt.Errorf("pdfops: a structure element needs an /S")
	}
	pageDict, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil || pageDict == nil {
		return 0, nil, fmt.Errorf("pdfops: page %d does not resolve: %w", pageNr, err)
	}
	pageRef, err := ctx.PageDictIndRef(pageNr)
	if err != nil || pageRef == nil {
		return 0, nil, fmt.Errorf("pdfops: page %d has no indirect reference: %w", pageNr, err)
	}

	arrays, singles := parentTreeEntries(ctx, tree)

	// The page's key, created if the page has none.
	key, hasKey := 0, false
	if sp, ok := pageDict["StructParents"].(types.Integer); ok {
		key, hasKey = sp.Value(), true
		if _, isSingle := singles[key]; isSingle {
			return 0, nil, fmt.Errorf("pdfops: page %d declares /StructParents %d and that "+
				"ParentTree entry is a single element, not an array — this document's tree is "+
				"already inconsistent and adding to it would hide that", pageNr, key)
		}
	}
	if !hasKey {
		key = nextParentTreeKey(arrays, singles)
		pageDict["StructParents"] = types.Integer(key)
	}

	// The MCID is the next free slot of that key's array.
	slots := arrays[key]
	mcid = len(slots)

	// Build the element. `/P` is the tree root: this is a top-level element, which is all the
	// wrapping emitter needs and is honest about what it knows — inferring a better parent from
	// page geometry is P08's job and would be a guess here.
	//
	// **`/P` is REQUIRED, and omitting it produces a document that looks tagged and is not.**
	// Measured: without it veraPDF reports every content item as `{mcid:0}` — marked, so the
	// bracketing worked — and still fails ua1 7.1 t3, *content shall be marked as Artifact or
	// tagged as real content*, because an element with no parent is not part of the tree the MCID
	// is supposed to resolve into. The failure names the content rather than the element, which is
	// why it reads as a wrapping problem and is not one. Every element of a real LibreOffice tree
	// carries `/P` (36 of 36, measured at S02).
	rootRef, err := structTreeRootRef(ctx)
	if err != nil {
		return 0, nil, err
	}
	elem := types.Dict{
		"Type": types.Name("StructElem"),
		"S":    types.Name(structType),
		"P":    *rootRef,
		"Pg":   *pageRef,
		"K":    types.Array{types.Integer(mcid)},
	}
	ref, err := ctx.IndRefForNewObject(elem)
	if err != nil {
		return 0, nil, err
	}

	// Both directions, in one place: the tree's side and the ParentTree's side.
	if err := appendToRootKids(ctx, tree, *ref); err != nil {
		return 0, nil, err
	}
	if err := setParentTreeSlot(ctx, tree, key, mcid, *ref); err != nil {
		return 0, nil, err
	}
	return mcid, ref, nil
}

// nextParentTreeKey returns a key no entry uses. Keys are a flat namespace shared by pages,
// annotations and Form XObjects, so "the page count" is not a safe answer.
func nextParentTreeKey(arrays map[int][]int, singles map[int]int) int {
	next := 0
	for k := range arrays {
		if k >= next {
			next = k + 1
		}
	}
	for k := range singles {
		if k >= next {
			next = k + 1
		}
	}
	return next
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

// setParentTreeSlot puts ref at index mcid of the array for key, growing the array and creating the
// entry as needed.
//
// **Growing means filling**, not appending: the array is indexed BY MCID, so making room for MCID 7
// in a four-slot array creates slots 4, 5 and 6 as nulls. An append would put the element at index 4
// and every later lookup would find the wrong element or none.
//
// **`addMarkedElement` never exercises that**, and saying so is the point: it allocates `mcid` as
// `len(arr)`, so fill and append coincide on every call it makes — swapping one for the other leaves
// its tests green. The fill is the general contract of this helper, which P05.S04 will need when it
// wraps content whose MCIDs already exist and are not contiguous, so it is driven directly by
// `TestAGapInTheParentTreeArrayIsFilledNotAppended` rather than left as an untested claim.
func setParentTreeSlot(ctx *model.Context, tree *structTree, key, mcid int, ref types.IndirectRef) error {
	ptObj, has := tree.root["ParentTree"]
	if !has {
		ptRef, err := ctx.IndRefForNewObject(types.Dict{"Nums": types.Array{}})
		if err != nil {
			return err
		}
		tree.root["ParentTree"] = *ptRef
		ptObj = *ptRef
	}
	pt, err := ctx.DereferenceDict(ptObj)
	if err != nil || pt == nil {
		return fmt.Errorf("pdfops: /ParentTree does not resolve to a dictionary: %w", err)
	}
	if _, nested := pt["Kids"]; nested {
		return fmt.Errorf("pdfops: this document's /ParentTree is a NESTED number tree, which the " +
			"write half does not rebalance — refusing rather than writing an entry that a reader " +
			"following /Kids would never find")
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
	}
	pt["Nums"] = nums
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
