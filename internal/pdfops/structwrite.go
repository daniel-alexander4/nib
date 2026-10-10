package pdfops

import (
	"fmt"
	"math"
	"reflect"

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

	// The page's key, created if the page has none; the MCID is the next free slot of that key's array.
	key, slots, err := pageRow(ctx, tree, pageDict, pageNr)
	if err != nil {
		return 0, nil, err
	}
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

// pageRow is page pageNr's `/ParentTree` key and how many slots its row has — the key allocated and set as the page's
// `/StructParents` when the page has none (a row of no slots, which the first `setParentTreeSlot` creates). It is the one
// place a page gains a key (ADR-009): `addMarkedElementUnder` wrote it inline, and a carry onto an untagged page of a
// tagged document needs the same (`PLAN-text-reflow.md` P07.S07).
func pageRow(ctx *model.Context, tree *structTree, pageDict types.Dict, pageNr int) (key, slots int, err error) {
	key, hasKey, written := structParentsOf(ctx.XRefTable, pageDict)
	if !written {
		key = allocParentTreeKey(ctx, tree)
		pageDict["StructParents"] = types.Integer(key)
		return key, 0, nil
	}
	if !hasKey {
		return 0, 0, fmt.Errorf("pdfops: page %d declares a /StructParents that is not a key (%v) — refusing to overwrite it",
			pageNr, pageDict["StructParents"])
	}
	slots, isSingle := parentTreeKey(ctx, tree, key)
	if isSingle {
		return 0, 0, fmt.Errorf("pdfops: page %d declares /StructParents %d and that "+
			"ParentTree entry is a single element, not an array — this document's tree is "+
			"already inconsistent and adding to it would hide that", pageNr, key)
	}
	return key, slots, nil
}

// structParentsOf is the ONE reader of a page's `/StructParents` (P07 phase-close review; ADR-009): its key, resolved
// through an indirect reference as every other number nib reads is (`pdfNumber`), and whether it has one. written says
// the entry is there at all — one that does not resolve to a whole, non-negative number is written and names no key,
// which a writer refuses rather than overwrites. Five sites read it as a bare `types.Integer`, so an indirect key read as
// NO key: `pageRow` gave the page a new one and orphaned its row.
func structParentsOf(xt *model.XRefTable, pageDict types.Dict) (key int, ok, written bool) {
	o, written := pageDict["StructParents"]
	if !written {
		return 0, false, false
	}
	key, ok = parentTreeKeyValue(xt, o)
	return key, ok, true
}

// parentTreeKeyValue reads o — a `/StructParents` or `/StructParent`, direct or indirect — as a `/ParentTree` key: a
// non-negative whole number in int32's range, or not a key at all. The one reading of a key's VALUE, shared by
// `structParentsOf` and the owners' census (`parentTreeOwners`), which had taken 2.5 as key 2 (P07 phase-close re-review).
func parentTreeKeyValue(xt *model.XRefTable, o types.Object) (int, bool) {
	v, isNum := pdfNumber(xt, o)
	if !isNum || v < 0 || v != math.Trunc(v) || v > math.MaxInt32 {
		return 0, false
	}
	return int(v), true
}

// parentTreeKey reports one key's `/ParentTree` entry — how many slots its array has, or whether it is a single
// reference — as `parentTreeLookup` finds it, so a writer sizes an MCID from the very pair `setParentTreeSlot` then
// writes (`/pending 786`: it read the last pair holding the key while the writer wrote the first).
//
// **It exists because the writers call it once per marked run.** `parentTreeEntries` builds every key's slot list, so
// calling it per run made tagging quadratic in the document: a 20,000-clause Markdown file (tier 4d's interrupt fixture)
// took over twenty minutes to convert where the untagged render takes seconds. This dereferences only the one value
// asked about.
func parentTreeKey(ctx *model.Context, tree *structTree, key int) (slots int, single bool) {
	holder, at := parentTreeLookup(ctx, tree.root["ParentTree"], key)
	if holder == nil {
		return 0, false
	}
	nums, _ := ctx.DereferenceArray(holder["Nums"])
	val := nums[at]
	if arr, ae := ctx.DereferenceArray(val); ae == nil && arr != nil {
		return len(arr), false
	}
	_, isInd := val.(types.IndirectRef)
	return 0, isInd
}

// parentTreeNextKey is the next key no `/ParentTree` entry uses — past every key in every node, whether or not a lookup
// would reach it: a key a node holds is spent for allocation even where `/Limits` hide it from a reader.
func parentTreeNextKey(ctx *model.Context, tree *structTree) (next int) {
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
				if n, ok := numsKey(ctx, nums[i]); ok && n >= next {
					next = n + 1
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
	return next
}

// parentTreeLookup is THE lookup of one `/ParentTree` key in this package (ADR-009; `/pending 786`): the node holding
// key and the index, in that node's `/Nums`, of the VALUE the key names — or nil and -1. Every writer acts on the pair
// it names (`setParentTreeSlot`, `clearParentTreeSlot`, `parentTreePlace`) and every reader reads it (`parentTreeKey`,
// `rowFor`, `parentTreeEntries`), so what a writer writes is what a reader reads. They disagreed: the writers took the
// FIRST pair holding a key in the first node holding it anywhere, `parentTreeKey` the LAST pair seen in a walk of every
// node, and on `0 [21] 0 [21 21]` `addMCIDTo` sized MCID 2 from one pair and wrote it into the other, where nothing reads.
//
// # The reading is uacheck's, which is veraPDF's
//
// `uacheck`'s `parentTreeEntry` measured it on veraPDF 1.30.2 (RR2-1): a depth-first search taking the FIRST node that
// holds the key and, within that node's `/Nums`, the LAST pair — veraPDF reads one /Nums into a map. A node with a
// `/Nums` key answers from it and its `/Kids` are never read; a node whose `/Limits` hold two integers excludes every key
// outside them, from itself and everything below. A key is an integer, direct or indirect, as uacheck reads it.
//
// ISO 32000-1 §7.9.7 (deferring to §7.9.6's name-tree rules) requires a number tree's keys sorted in ascending order and
// says nothing of a key that appears twice, so a repeat is malformed and which pair counts is the reader's choice.
// pdfcpu makes none — `validateNumberTreeDictNumsEntry` checks each key is an integer and never compares two — so the
// choice that decides what a user is told is the checker's, and nib's writers take it.
//
// A node already searched without finding the key is not searched again, so a `/Kids` DAG costs each node once per
// lookup (uacheck's RR3-1, which changes no answer), and a node met again inside itself is not re-entered.
func parentTreeLookup(ctx *model.Context, ptObj types.Object, key int) (holder types.Dict, at int) {
	return (&parentTreeReader{ctx: ctx}).lookup(ptObj, key)
}

// parentTreeReader is `parentTreeLookup` with each node's `/Nums` parsed once across many lookups — for a reader that
// asks every key (`parentTreeEntries`) and writes nothing in between. A writer never keeps one across a write: the
// parse is of the `/Nums` as it was.
type parentTreeReader struct {
	ctx    *model.Context
	parsed map[uintptr]map[int]int
}

func (r *parentTreeReader) lookup(ptObj types.Object, key int) (types.Dict, int) {
	searched, inside := map[uintptr]bool{}, map[uintptr]bool{}
	var search func(o types.Object, depth int) (types.Dict, int)
	search = func(o types.Object, depth int) (types.Dict, int) {
		if depth > maxStructDepth {
			return nil, -1
		}
		d, err := r.ctx.DereferenceDict(o)
		if err != nil || d == nil {
			return nil, -1
		}
		id := reflect.ValueOf(d).Pointer()
		if searched[id] || inside[id] {
			return nil, -1
		}
		if lo, hi, limited := numberTreeLimits(r.ctx, d); limited && (key < lo || key > hi) {
			return nil, -1
		}
		if _, leaf := d["Nums"]; leaf {
			if at, ok := r.index(d, id)[key]; ok {
				return d, at
			}
			searched[id] = true
			return nil, -1 // a node with /Nums answers from it; its /Kids are never read
		}
		inside[id] = true
		kids, _ := r.ctx.DereferenceArray(d["Kids"])
		for _, k := range kids {
			if h, at := search(k, depth+1); h != nil {
				return h, at
			}
		}
		delete(inside, id)
		searched[id] = true
		return nil, -1
	}
	return search(ptObj, 0)
}

// index is node's `/Nums` as key → index of the value of the LAST pair holding it; a /Nums that is not an array is empty.
func (r *parentTreeReader) index(d types.Dict, id uintptr) map[int]int {
	if m, done := r.parsed[id]; done {
		return m
	}
	m := map[int]int{}
	nums, _ := r.ctx.DereferenceArray(d["Nums"])
	for i := 0; i+1 < len(nums); i += 2 {
		if k, ok := numsKey(r.ctx, nums[i]); ok {
			m[k] = i + 1
		}
	}
	if r.parsed == nil {
		r.parsed = map[uintptr]map[int]int{}
	}
	r.parsed[id] = m
	return m
}

// numsKey reads a number tree's key or bound: an integer, direct or indirect, as uacheck's `intValue` reads one.
func numsKey(ctx *model.Context, o types.Object) (int, bool) {
	if o == nil {
		return 0, false
	}
	i, err := ctx.DereferenceInteger(o)
	if err != nil || i == nil {
		return 0, false
	}
	return i.Value(), true
}

// numberTreeLimits is node's `/Limits` as a reader honours it: its first two entries when both are integers (uacheck's
// `ptParse`, after veraPDF's `parseLimitsArray`), else no limits at all. `widenLimits` reads it here too, so a bound the
// lookup honours is one the writer raises.
func numberTreeLimits(ctx *model.Context, node types.Dict) (lo, hi int, limited bool) {
	lim, _ := ctx.DereferenceArray(node["Limits"])
	if len(lim) < 2 {
		return 0, 0, false
	}
	a, aok := numsKey(ctx, lim[0])
	b, bok := numsKey(ctx, lim[1])
	if !aok || !bok {
		return 0, 0, false
	}
	return a, b, true
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

// parentTreeDict is the `/ParentTree` node whose `/Nums` an entry for key is written in — the one place every entry
// writer goes through (ADR-009). The tree is created, empty, when the document has none.
//
// # A nested number tree is written, in the two shapes nib writes — `PLAN-text-reflow.md` P07.S07
//
// It was refused here: *"the write half does not rebalance"*. Measured on 2026-09-30, **7 of the 14** multi-page
// real-producer documents nest their `/ParentTree` under `/Kids` (Acrobat, Designer, InDesign, Word), so every writer
// refused half the documents a carry exists for. Nothing needs rebalancing for what nib writes:
//
//   - **a key some node already holds** is written in THAT node, in place — its `/Limits` already span it;
//   - **a new key above every key** (`allocParentTreeKey` hands out no other) goes at the end of the rightmost leaf, and
//     each `/Limits` on the way down is raised to it — so a reader following `/Limits` finds it where it looks.
//
// A new key INSIDE a nested tree's range is still refused: placing it means choosing a leaf and splitting or re-sorting
// it, which nothing nib does asks for.
func parentTreeDict(ctx *model.Context, tree *structTree, key int) (node types.Dict, at int, err error) {
	if _, has := tree.root["ParentTree"]; !has {
		ptRef, err := ctx.IndRefForNewObject(types.Dict{"Nums": types.Array{}})
		if err != nil {
			return nil, -1, err
		}
		tree.root["ParentTree"] = *ptRef
	}
	path, holder, at, err := parentTreePlace(ctx, tree, key)
	if err != nil {
		return nil, -1, err
	}
	if holder != nil {
		return holder, at, nil
	}
	// Every node on the way down — the root included, when it carries `/Limits` (P07 phase-close review) — now spans key.
	for _, node := range path {
		widenLimits(ctx, node, key)
	}
	return path[len(path)-1], -1, nil
}

// parentTreePlace is where parentTreeDict would write key, WITHOUT writing anything — the one predicate the writer and a
// carry's up-front check (`targetRefusal`) both ask (P07 phase-close review; ADR-009: the check restated one of these
// refusals and missed the rest). holder is the node already holding key as a reader finds it (`parentTreeLookup`), and at
// the index of its value in holder's `/Nums`; else path is the nodes from the `/ParentTree` down to the leaf the entry goes
// in, or the refusal. A document with no `/ParentTree` places every key in the flat one the writer creates.
//
// **A node with a `/Nums` is a leaf**, root included, whatever `/Kids` it also has — a reader answers from its /Nums and
// never reads its /Kids, so a key written below one would be unreachable (`/pending 786`).
//
// A flat tree's key the lookup does not reach — its root's `/Limits` exclude it — gets a NEW pair, which `insertNum` puts
// after any pair already holding the key and `widenLimits` brings into range, so the new pair is the one a reader then
// takes. Writing into the hidden pair instead would size an MCID from one pair and write it into another.
func parentTreePlace(ctx *model.Context, tree *structTree, key int) (path []types.Dict, holder types.Dict, at int, err error) {
	ptObj, has := tree.root["ParentTree"]
	if !has {
		return []types.Dict{{}}, nil, -1, nil
	}
	pt, err := ctx.DereferenceDict(ptObj)
	if err != nil || pt == nil {
		return nil, nil, -1, fmt.Errorf("pdfops: /ParentTree does not resolve to a dictionary: %w", err)
	}
	if h, i := parentTreeLookup(ctx, ptObj, key); h != nil {
		return nil, h, i, nil
	}
	_, leaf := pt["Nums"]
	if _, nested := pt["Kids"]; leaf || !nested {
		return []types.Dict{pt}, nil, -1, nil
	}
	if key < parentTreeNextKey(ctx, tree) {
		return nil, nil, -1, fmt.Errorf("pdfops: /ParentTree key %d falls inside a NESTED number tree's range, and placing it "+
			"means choosing and re-sorting a leaf — refusing rather than writing an entry a reader following /Limits "+
			"would never find", key)
	}
	node := pt
	path = []types.Dict{pt}
	for depth := 0; ; depth++ {
		if _, leaf := node["Nums"]; leaf && depth > 0 {
			return path, nil, -1, nil
		}
		kids, _ := ctx.DereferenceArray(node["Kids"])
		if len(kids) == 0 {
			if depth > 0 {
				return path, nil, -1, nil
			}
			return nil, nil, -1, fmt.Errorf("pdfops: the nested /ParentTree has an empty /Kids and no leaf to write key %d in", key)
		}
		if depth > maxStructDepth {
			return nil, nil, -1, fmt.Errorf("pdfops: the nested /ParentTree is deeper than %d", maxStructDepth)
		}
		child, cerr := ctx.DereferenceDict(kids[len(kids)-1])
		if cerr != nil || child == nil {
			return nil, nil, -1, fmt.Errorf("pdfops: the nested /ParentTree's last kid does not resolve: %w", cerr)
		}
		node = child
		path = append(path, child)
	}
}

// widenLimits makes node's `/Limits`, when a reader honours them (`numberTreeLimits`), span key. A `/Limits` of more than
// two entries is read by its first two and written back as two; one whose bounds are not integers is no limit, and is
// left alone.
func widenLimits(ctx *model.Context, node types.Dict, key int) {
	lo, hi, limited := numberTreeLimits(ctx, node)
	if !limited {
		return
	}
	node["Limits"] = types.Array{types.Integer(min(lo, key)), types.Integer(max(hi, key))}
}

// insertNum puts the pair (key, val) into a number tree's flat `[key value …]` array before the first larger key, so the
// keys stay ascending as ISO 32000-1 §7.9.7 requires. Appending was right only for a key above every key; a page that
// declares `/StructParents` with no row below the highest key put its row out of order (P07.S07's review).
func insertNum(nums types.Array, key int, val types.Object) types.Array {
	at := len(nums)
	for i := 0; i+1 < len(nums); i += 2 {
		if n, ok := nums[i].(types.Integer); ok && n.Value() > key {
			at = i
			break
		}
	}
	out := make(types.Array, 0, len(nums)+2)
	out = append(out, nums[:at]...)
	out = append(out, types.Integer(key), val)
	return append(out, nums[at:]...)
}

// setParentTreeSlot puts ref at index mcid of the array for key, growing the array and creating the
// entry as needed. The array is the one a reader finds (`parentTreeLookup`): of a key `/Nums` repeats, the LAST pair.
// An array written as its own object is updated in that object.
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
	pt, at, err := parentTreeDict(ctx, tree, key)
	if err != nil {
		return err
	}
	nums, _ := ctx.DereferenceArray(pt["Nums"])
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
		// An indirect slot array is updated where it lives, as `clearParentTreeSlot` updates one: writing the
		// array over the reference left the row's own object behind, unreferenced, and the entry direct —
		// read the same, and a different document than the edit asked for (ADR-124, measured on a delete).
		if ind, isInd := nums[at].(types.IndirectRef); isInd {
			if en, found := ctx.XRefTable.FindTableEntryForIndRef(&ind); found && en != nil {
				if _, isArr := en.Object.(types.Array); isArr {
					en.Object = arr
					return nil
				}
			}
		}
		nums[at] = arr
	} else {
		nums = insertNum(nums, key, arr)
		claimParentTreeKey(ctx, tree, key)
	}
	pt["Nums"] = nums
	return nil
}

// clearParentTreeSlot empties index mcid of the array for key — the content that slot named is no
// longer owned by any element (`PLAN-accessibility.md` P09.S03's artifact edit). A tree with no
// ParentTree, no entry for key, or a slot past the array's end has nothing to clear, and one is not
// created to be emptied. The array is the one a reader finds (`parentTreeLookup`).
func clearParentTreeSlot(ctx *model.Context, tree *structTree, key, mcid int) error {
	if _, has := tree.root["ParentTree"]; !has {
		return nil
	}
	if root, err := ctx.DereferenceDict(tree.root["ParentTree"]); err != nil || root == nil {
		return fmt.Errorf("pdfops: /ParentTree does not resolve to a dictionary: %w", err)
	}
	pt, at := parentTreeLookup(ctx, tree.root["ParentTree"], key)
	if pt == nil {
		return nil
	}
	nums, _ := ctx.DereferenceArray(pt["Nums"])
	arr, aerr := ctx.DereferenceArray(nums[at])
	if aerr != nil || arr == nil || mcid < 0 || mcid >= len(arr) {
		return nil
	}
	arr = append(types.Array{}, arr...)
	arr[mcid] = nil
	// An indirect slot array is updated where it lives, or the entry keeps pointing at the old one.
	if ind, isInd := nums[at].(types.IndirectRef); isInd {
		if en, found := ctx.XRefTable.FindTableEntryForIndRef(&ind); found && en != nil {
			en.Object = arr
			return nil
		}
	}
	nums[at] = arr
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

// addGroupingElement creates an element that has no marked content of its own — an `L` holding
// `LI`s, or an `LI` holding an `Lbl` and an `LBody`.
//
// **It takes no page and registers no ParentTree entry**, because it owns no MCID. A grouping
// element's `/Pg` is optional and its children carry the page; writing one here would name a page
// the element's content is not on when its children span a break.
func addGroupingElement(ctx *model.Context, tree *structTree, structType string,
	parent *types.IndirectRef) (*types.IndirectRef, error) {

	ref, err := newGroupingElement(ctx, structType, parent)
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

// newGroupingElement writes the element `addGroupingElement` adds — `/Type /StructElem`, its `/S`, a `/P`
// naming parent (the tree root when nil) and an empty `/K` — and lists it NOWHERE: the caller places it.
// The one shape of an element that owns no content (ADR-009): the tree editor's create writes the same
// dictionary and places it among its parent's kids rather than last (ADR-124).
func newGroupingElement(ctx *model.Context, structType string, parent *types.IndirectRef) (*types.IndirectRef, error) {
	parentRef := parent
	if parentRef == nil {
		rootRef, err := structTreeRootRef(ctx)
		if err != nil {
			return nil, err
		}
		parentRef = rootRef
	}
	return ctx.IndRefForNewObject(types.Dict{
		"Type": types.Name("StructElem"),
		"S":    types.Name(structType),
		"P":    *parentRef,
		"K":    types.Array{},
	})
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
	key, ok, _ := structParentsOf(ctx.XRefTable, pageDict)
	if !ok {
		return 0, fmt.Errorf("pdfops: page %d has no /StructParents, so it owns no ParentTree "+
			"entry to add a marked-content id to", pageNr)
	}
	mcid, _ := parentTreeKey(ctx, tree, key)

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
	// Through `kidsArray`, which reads every form `/K` takes: `DereferenceArray` fails on a single integer or a single
	// MCR, and appending to its nil answer overwrote the element's one kid (found by P07.S07's deepdive).
	kids, set := kidsArray(ctx, d)
	set(append(kids, types.Integer(mcid)))
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
	pt, _, err := parentTreeDict(ctx, tree, key)
	if err != nil {
		return err
	}
	nums, _ := ctx.DereferenceArray(pt["Nums"])
	for i := 0; i+1 < len(nums); i += 2 {
		n, ok := numsKey(ctx, nums[i])
		if !ok || n != key {
			continue
		}
		// An existing entry at this key is the caller having picked a key that is already taken.
		// Overwriting it would silently re-point whatever owned it at this element. Every pair in the
		// node counts, not only the one `parentTreeLookup` reads: a refusal may be wider than a read.
		return fmt.Errorf("pdfops: /ParentTree key %d is already taken — overwriting it would "+
			"re-point whatever owns it at a different element", key)
	}
	pt["Nums"] = insertNum(nums, key, ref)
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
	tree.keyFloor, tree.keyFloorKnown = parentTreeKeyFloor(ctx, tree.root), true
	return tree.keyFloor
}

// parentTreeKeyFloor is the lowest `/ParentTree` key nothing in ctx has spent: past every row the
// root's `/ParentTree` holds, past its declared `/ParentTreeNextKey` (a producer's declaration can lag),
// and past every key a page, annotation or form XObject CLAIMS — a claim spends its key whether or not
// a row exists for it. It is the one floor `allocParentTreeKey` and a merge's graft offset both read
// (ADR-009): the merge kept its own copy, which read only the first two, so a host whose page claimed a
// key above its rows handed the grafted document that key and the collision cost it the graft.
func parentTreeKeyFloor(ctx *model.Context, root types.Dict) int {
	floor := parentTreeNextKey(ctx, &structTree{root: root})
	if nk, ok := pdfNumber(ctx.XRefTable, root["ParentTreeNextKey"]); ok && int(nk) > floor {
		floor = int(nk)
	}
	for key := range parentTreeOwners(ctx) {
		if key+1 > floor {
			floor = key + 1
		}
	}
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

// byteString reads o — direct or indirect — as a PDF string's BYTES, the form an element identifier and a name
// tree's key are compared in, and returns the string object itself beside them. A string of no bytes is none.
func byteString(ctx *model.Context, o types.Object) (raw string, lit types.Object, ok bool) {
	if o == nil {
		return "", nil, false
	}
	v, err := ctx.Dereference(o)
	if err != nil {
		return "", nil, false
	}
	var b []byte
	switch s := v.(type) {
	case types.StringLiteral:
		b, err = types.Unescape(s.Value())
	case types.HexLiteral:
		b, err = s.Bytes()
	default:
		return "", nil, false
	}
	if err != nil || len(b) == 0 {
		return "", nil, false
	}
	return string(b), v, true
}

// elementID is a structure element's `/ID` — its bytes and the string object — and whether it has one.
func elementID(ctx *model.Context, elem types.Dict) (raw string, lit types.Object, ok bool) {
	return byteString(ctx, elem["ID"])
}

// setIDTreeEntry enters ref under id in the root's `/IDTree`, the name tree ISO 32000-1 Table 322 requires once any
// element carries an identifier — created, flat and indirect (pdfcpu's validator reads it only through a reference),
// when the document has none (ADR-119). key is id as the string object the element carries.
//
// A producer's tree is added to and never rebuilt: the entry goes into the leaf whose `/Limits` hold id — the first kid
// whose upper bound is not below it, else the last — in byte order, and each `/Limits` on the way down is widened to it,
// so a reader following `/Limits` finds it where it looks. An entry already under id is re-pointed: an identifier is
// allocated only where no element of the tree carries it, so what that entry named is no element of this tree.
func setIDTreeEntry(ctx *model.Context, tree *structTree, id string, key types.Object, ref types.IndirectRef) error {
	if _, has := tree.root["IDTree"]; !has {
		treeRef, err := ctx.IndRefForNewObject(types.Dict{"Names": types.Array{key, ref}})
		if err != nil {
			return err
		}
		tree.root["IDTree"] = *treeRef
		return nil
	}
	node, err := ctx.DereferenceDict(tree.root["IDTree"])
	if err != nil || node == nil {
		return fmt.Errorf("pdfops: /IDTree does not resolve to a dictionary: %w", err)
	}
	for depth := 0; depth <= maxStructDepth; depth++ {
		if depth > 0 {
			lim, _ := ctx.DereferenceArray(node["Limits"])
			lo, hi := key, key
			if len(lim) >= 2 {
				if s, _, ok := byteString(ctx, lim[0]); ok && s <= id {
					lo = lim[0]
				}
				if s, _, ok := byteString(ctx, lim[1]); ok && s >= id {
					hi = lim[1]
				}
			}
			node["Limits"] = types.Array{lo, hi}
		}
		kids, _ := ctx.DereferenceArray(node["Kids"])
		if _, leaf := node["Names"]; leaf || len(kids) == 0 {
			names, _ := ctx.DereferenceArray(node["Names"])
			at, replace := len(names), false
			for i := 0; i+1 < len(names); i += 2 {
				if s, _, ok := byteString(ctx, names[i]); ok && s >= id {
					at, replace = i, s == id
					break
				}
			}
			out := append(types.Array{}, names[:at]...)
			out = append(out, key, ref)
			if replace {
				at += 2
			}
			out = append(out, names[at:]...)
			// An indirect /Names is updated where it lives, or the node keeps pointing at the old array.
			if ind, isInd := node["Names"].(types.IndirectRef); isInd {
				if en, found := ctx.XRefTable.FindTableEntryForIndRef(&ind); found && en != nil {
					en.Object = out
					return nil
				}
			}
			node["Names"] = out
			return nil
		}
		next := kids[len(kids)-1]
		for _, k := range kids {
			kd, kerr := ctx.DereferenceDict(k)
			if kerr != nil || kd == nil {
				continue
			}
			if lim, _ := ctx.DereferenceArray(kd["Limits"]); len(lim) >= 2 {
				if s, _, ok := byteString(ctx, lim[1]); ok && s >= id {
					next = k
					break
				}
			}
		}
		child, cerr := ctx.DereferenceDict(next)
		if cerr != nil || child == nil {
			return fmt.Errorf("pdfops: a kid of the nested /IDTree does not resolve to a dictionary: %w", cerr)
		}
		node = child
	}
	return fmt.Errorf("pdfops: the nested /IDTree is deeper than %d", maxStructDepth)
}
