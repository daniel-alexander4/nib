package pdfops

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Creating a structure element and deleting one — ADR-124 (`/pending 855` item 1).
//
// # Create
//
// A new element owns nothing: it is the grouping element `addGroupingElement` writes
// (`newGroupingElement`), placed among its parent's element kids by the rule a move places by
// (`placeAmongElements`). It gets no `/Pg` and no `/ParentTree` entry. What goes in it is moved in.
//
// # Delete is not the artifact edit
//
// `artifactElement` declares an element's CONTENT decoration and rewrites the page to say so. A delete
// takes away the TAG and keeps everything it held: the element's `/K` entries take its place in its
// parent's `/K`, in order, and no content stream changes by a byte. What the deleted element owned, its
// parent owns afterwards.
//
// # The two page-inheritance rules
//
// Two kinds of kid take their page from the element that holds them, so moving them under another
// element would silently move their content to another page:
//
//   - **an integer MCID** means "on the owning element's page". It stays an integer only where the new
//     parent itself NAMES the deleted element's page (a page it merely reads by inheritance is this
//     reader's courtesy, not the file's); otherwise it is written as a marked-content reference
//     naming the page it was always on. The new parent's own `/Pg` is never changed.
//   - **an element, MCR or OBJR kid that names no page** reads the deleted element's
//     (`readStructTree` hands the page down). Where that page is not the new parent's, the kid is given
//     it in writing.
//
// The page compared is the one the new parent names in its own `/Pg`. A deleted element with no page at
// all — none named, none inherited — has nothing to hand on, and nothing is written.
//
// # What it refuses
//
//   - **a top-level element that holds content itself**: its content would land in the root's `/K`, which
//     may hold only structure elements (ISO 32000-1 Table 322);
//   - **the element whose going would leave the tree with none** (ADR-031, as the artifact edit);
//   - **an element inside one written inline**: the new owner has no object number, so no `/ParentTree`
//     slot and no `/P` can name it;
//   - an element itself written inline, which no edit can name (`applyStructEdit`).

// objectRef is the reference that names object nr, at the generation the xref table holds it.
func objectRef(ctx *model.Context, nr int) types.IndirectRef {
	gen := 0
	if en, ok := ctx.XRefTable.Table[nr]; ok && en != nil && en.Generation != nil {
		gen = *en.Generation
	}
	return *types.NewIndirectRef(nr, gen)
}

// createElement adds an element of type ed.value, owning nothing, as the ed.index-th element kid of
// ed.parent — the root for 0 or `rootParent`.
func createElement(ctx *model.Context, tree *structTree, ed structEdit) error {
	if ed.elem != 0 {
		return fmt.Errorf("%w: a new element has no number until it is written, and this edit names element %d — leave the element out", ErrTagsReview, ed.elem)
	}
	if !standardStructTypes[ed.value] {
		return fmt.Errorf("%w: %q is not a standard structure type", ErrTagsReview, ed.value)
	}
	holder := tree.root
	var parent *types.IndirectRef
	if ed.parent != 0 && ed.parent != rootParent {
		to := tree.byObj[ed.parent]
		if to == nil {
			return staleEdit{ed.parent}
		}
		ref := objectRef(ctx, to.objNr)
		holder, parent = to.dict, &ref
	}
	ref, err := newGroupingElement(ctx, ed.value, parent)
	if err != nil {
		return err
	}
	placeAmongElements(ctx, holder, *ref, ed.index)
	return nil
}

// deleteElement takes e out of the tree and puts its kids where it was, under its parent (ADR-124).
func deleteElement(ctx *model.Context, tree *structTree, e *structElem) error {
	parent := e.parent
	if parent != nil && parent.objNr == 0 {
		return fmt.Errorf("%w: element %d is inside a tag written inline, which has no object number, so nothing could name that tag as the owner of what element %d holds — make inline tags editable first (the promote edit), then delete it", ErrTagsReview, e.objNr, e.objNr)
	}
	entries, _ := kidsArray(ctx, e.dict)
	if parent == nil {
		elements := 0
		for _, en := range entries {
			if !isElementEntry(ctx, en) {
				return fmt.Errorf("%w: element %d is at the top of the structure tree and holds content itself, and the top of the tree may hold only tags — change its type instead, or mark it as decoration if it is not content", ErrTagsReview, e.objNr)
			}
			elements++
		}
		rootKids, _ := kidsArray(ctx, tree.root)
		for _, en := range rootKids {
			if ir, ok := en.(types.IndirectRef); ok && ir.ObjectNumber.Value() == e.objNr {
				continue
			}
			if isElementEntry(ctx, en) {
				elements++
			}
		}
		if elements == 0 {
			return fmt.Errorf("%w: element %d is the last element of the structure tree, and a document that claims tagging over an empty tree is the claim ADR-031 forbids — to start again, remove all tags", ErrTagsReview, e.objNr)
		}
	}

	// The new owner: e's parent, or the root for a top-level e (which, past the refusal above, holds only
	// elements — so the root is never asked to own content).
	var owner types.IndirectRef
	ownerPg := 0
	if parent != nil {
		owner = objectRef(ctx, parent.objNr)
		if ir, ok := parent.dict["Pg"].(types.IndirectRef); ok {
			ownerPg = ir.ObjectNumber.Value()
		}
	} else {
		rootRef, err := structTreeRootRef(ctx)
		if err != nil {
			return err
		}
		owner = *rootRef
	}
	// pageMoves: what took its page from e would take another under the new owner — or none that a reader
	// other than this one could find. ownerPg is the page the new owner NAMES, not the one it reads here by
	// inheritance: ISO 32000-1 hands no page down from element to element, so an integer left under an owner
	// that only inherits e's page is content another reader cannot place. The page is then written out.
	pageMoves := e.pgObj != 0 && e.pgObj != ownerPg
	ePg := objectRef(ctx, e.pgObj)

	moved := make(types.Array, 0, len(entries))
	for _, en := range entries {
		if n, ok := en.(types.Integer); ok {
			if pageMoves {
				moved = append(moved, types.Dict{"Type": types.Name("MCR"), "Pg": ePg, "MCID": n})
			} else {
				moved = append(moved, en)
			}
			if err := passSlot(ctx, tree, pageRowKey(ctx, e.pgObj), n.Value(), e.objNr, owner); err != nil {
				return err
			}
			continue
		}
		d, err := ctx.DereferenceDict(en)
		if err != nil || d == nil {
			return fmt.Errorf("pdfops: element %d's /K holds an entry that is neither an integer nor a dictionary (%T)", e.objNr, en)
		}
		ty := ""
		if n := d.NameEntry("Type"); n != nil {
			ty = *n
		}
		_, ownPage := d["Pg"].(types.IndirectRef)
		if ty == "MCR" || ty == "OBJR" {
			entry := en
			if pageMoves && !ownPage {
				// A copy, never the object written through: a reference may be shared.
				cp := types.Dict{}
				for k, v := range d {
					cp[k] = v
				}
				cp["Pg"] = ePg
				entry = cp
			}
			moved = append(moved, entry)
			if ty == "OBJR" {
				// The annotation's own `/ParentTree` entry — one reference, under its `/StructParent`.
				if obj, oerr := ctx.DereferenceDict(d["Obj"]); oerr == nil && obj != nil {
					if key, ok := parentTreeKeyValue(ctx.XRefTable, obj["StructParent"]); ok {
						passSingle(ctx, tree, key, e.objNr, owner)
					}
				}
				continue
			}
			mcid := 0
			if n := d.IntEntry("MCID"); n != nil {
				mcid = *n
			}
			pg := e.pgObj
			if ir, ok := d["Pg"].(types.IndirectRef); ok {
				pg = ir.ObjectNumber.Value()
			}
			if err := passSlot(ctx, tree, pageRowKey(ctx, pg), mcid, e.objNr, owner); err != nil {
				return err
			}
			// Content in another stream is owned through THAT stream's row (ADR-038).
			if stm, ok := d["Stm"].(types.IndirectRef); ok {
				if err := passSlot(ctx, tree, streamRowKey(ctx, stm), mcid, e.objNr, owner); err != nil {
					return err
				}
			}
			continue
		}
		// An element. One the tree lists under another parent first is that parent's, and is left as it is.
		if ir, isRef := en.(types.IndirectRef); isRef {
			if kid := tree.byObj[ir.ObjectNumber.Value()]; kid == nil || kid.parent == e {
				d["P"] = owner
				if pageMoves && !ownPage {
					d["Pg"] = ePg
				}
			}
		} else {
			if _, has := d["P"]; has {
				d["P"] = owner
			}
			if pageMoves && !ownPage {
				d["Pg"] = ePg
			}
		}
		moved = append(moved, en)
	}

	// e's identifier goes with it: its `/IDTree` entry, and its name in every `/Headers` — unless another
	// element carries the same identifier, which those then still name.
	if id, _, has := elementID(ctx, e.dict); has {
		removeIDTreeEntry(ctx, tree, id, e.objNr)
		shared := false
		for _, x := range tree.elems {
			if s, _, ok := elementID(ctx, x.dict); x != e && ok && s == id {
				shared = true
				break
			}
		}
		if !shared {
			for _, x := range tree.elems {
				if x != e {
					dropHeader(ctx, x, id)
				}
			}
		}
	}

	if _, err := replaceInParent(ctx, tree, e, moved); err != nil {
		return err
	}
	// Nothing lists e now. Should a `/ParentTree` slot it never claimed still name it, what is written is an
	// element that claims nothing, not a second owner of its old kids.
	delete(e.dict, "K")
	return nil
}

// pageRowKey is the `/ParentTree` key of the page at object pgObj — its `/StructParents` — or -1.
func pageRowKey(ctx *model.Context, pgObj int) int {
	if pgObj == 0 {
		return -1
	}
	d, err := ctx.DereferenceDict(*types.NewIndirectRef(pgObj, 0))
	if err != nil || d == nil {
		return -1
	}
	if key, ok, _ := structParentsOf(ctx.XRefTable, d); ok {
		return key
	}
	return -1
}

// streamRowKey is the `/ParentTree` key of the content stream an MCR's `/Stm` names — a form XObject's
// `/StructParents` — or -1.
func streamRowKey(ctx *model.Context, stm types.IndirectRef) int {
	sd, _, err := ctx.DereferenceStreamDict(stm)
	if err != nil || sd == nil {
		return -1
	}
	if key, ok, _ := structParentsOf(ctx.XRefTable, sd.Dict); ok {
		return key
	}
	return -1
}

// parentTreeSlotOwner is the object number slot mcid of key's `/ParentTree` array names, as a reader finds
// it (`parentTreeLookup`), or 0: no such row, a row that is not an array, a slot past its end, a null.
func parentTreeSlotOwner(ctx *model.Context, tree *structTree, key, mcid int) int {
	holder, at := parentTreeLookup(ctx, tree.root["ParentTree"], key)
	if holder == nil {
		return 0
	}
	nums, _ := ctx.DereferenceArray(holder["Nums"])
	arr, err := ctx.DereferenceArray(nums[at])
	if err != nil || arr == nil || mcid < 0 || mcid >= len(arr) {
		return 0
	}
	if ir, ok := arr[mcid].(types.IndirectRef); ok {
		return ir.ObjectNumber.Value()
	}
	return 0
}

// passSlot hands the `/ParentTree` slot (key, mcid) from the element at object from to the element to
// names — where the slot names from. A slot that names something else, or nothing, was not from's to pass
// on: a defect the tree already had is not this edit's to repair (`applyStructEdits`).
func passSlot(ctx *model.Context, tree *structTree, key, mcid, from int, to types.IndirectRef) error {
	if key < 0 || parentTreeSlotOwner(ctx, tree, key, mcid) != from {
		return nil
	}
	return setParentTreeSlot(ctx, tree, key, mcid, to)
}

// passSingle hands a single-reference `/ParentTree` entry — an annotation's, under its `/StructParent` —
// from the element at object from to the element to names, where it names from. `setParentTreeSingle`
// CREATES an entry and refuses a key that has one; this is the re-pointing it deliberately is not.
func passSingle(ctx *model.Context, tree *structTree, key, from int, to types.IndirectRef) {
	holder, at := parentTreeLookup(ctx, tree.root["ParentTree"], key)
	if holder == nil {
		return
	}
	nums, _ := ctx.DereferenceArray(holder["Nums"])
	if arr, err := ctx.DereferenceArray(nums[at]); err == nil && arr != nil {
		return // a page's row, not an annotation's entry
	}
	if ir, ok := nums[at].(types.IndirectRef); ok && ir.ObjectNumber.Value() == from {
		nums[at] = to // in place: the array is the one the node holds, direct or indirect
	}
}

// removeIDTreeEntry takes the entry under id out of the root's `/IDTree` where it names the element at
// object objNr. A leaf left with no entry leaves its parent's `/Kids`, and a tree left with none leaves
// the root: an empty name tree names nothing, and Table 322 asks for one only while an element has an
// identifier. A leaf's `/Limits` are restated from the entries it keeps.
func removeIDTreeEntry(ctx *model.Context, tree *structTree, id string, objNr int) {
	// write puts arr where node[key] lives: into the indirect array object, or onto the node.
	write := func(node types.Dict, key string, arr types.Array) {
		if ind, isInd := node[key].(types.IndirectRef); isInd {
			if en, found := ctx.XRefTable.FindTableEntryForIndRef(&ind); found && en != nil {
				en.Object = arr
				return
			}
		}
		node[key] = arr
	}
	var prune func(o types.Object, depth int) (empty bool)
	prune = func(o types.Object, depth int) bool {
		node, err := ctx.DereferenceDict(o)
		if err != nil || node == nil || depth > maxStructDepth {
			return false
		}
		if _, leaf := node["Names"]; leaf {
			names, _ := ctx.DereferenceArray(node["Names"])
			kept := make(types.Array, 0, len(names))
			for i := 0; i+1 < len(names); i += 2 {
				if s, _, ok := byteString(ctx, names[i]); ok && s == id {
					if ir, isRef := names[i+1].(types.IndirectRef); isRef && ir.ObjectNumber.Value() == objNr {
						continue
					}
				}
				kept = append(kept, names[i], names[i+1])
			}
			if len(kept) == len(names)-len(names)%2 {
				return false
			}
			write(node, "Names", kept)
			if _, limited := node["Limits"]; limited && len(kept) >= 2 {
				node["Limits"] = types.Array{kept[0], kept[len(kept)-2]}
			}
			return len(kept) == 0
		}
		kids, _ := ctx.DereferenceArray(node["Kids"])
		kept := make(types.Array, 0, len(kids))
		for _, k := range kids {
			if !prune(k, depth+1) {
				kept = append(kept, k)
			}
		}
		if len(kept) == len(kids) {
			return false
		}
		write(node, "Kids", kept)
		return len(kept) == 0
	}
	if o, has := tree.root["IDTree"]; has && prune(o, 0) {
		delete(tree.root, "IDTree")
	}
}

// dropHeader takes the identifier id out of x's `/Headers`, through the one writer of a Table attribute
// (`withTableAttribute`, ADR-119). A `/Headers` left naming nothing is removed.
func dropHeader(ctx *model.Context, x *structElem, id string) {
	for _, a := range tableAttributes(ctx, x.dict["A"]) {
		names, err := ctx.DereferenceArray(a["Headers"])
		if err != nil || names == nil {
			continue
		}
		kept := make(types.Array, 0, len(names))
		for _, n := range names {
			if s, _, ok := byteString(ctx, n); ok && s == id {
				continue
			}
			kept = append(kept, n)
		}
		if len(kept) != len(names) {
			var value types.Object
			if len(kept) > 0 {
				value = kept
			}
			setTableAttribute(ctx, x, "Headers", value)
		}
		return
	}
}
