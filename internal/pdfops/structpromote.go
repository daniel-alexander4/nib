package pdfops

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Giving every inline element an object number — ADR-126 (`/pending 855` item 3).
//
// An element written inline — a dictionary inside its parent's `/K` — has no object number. An edit names
// an element by its number, a `/P` names a parent by reference, and a `/ParentTree` slot names an owner by
// reference, so an inline element can be none of the three. There is no second address for one: a path or a
// position would be an identifier that every other edit in the same batch moves.
//
// So the one thing done to an inline element is to stop it being inline. `promote` takes no element: it
// writes EVERY inline element of the tree as an object of its own — the same dictionary, its place in its
// parent's `/K` taken by the reference — and then says the three things a reference makes sayable.
//
// # Top-down, because `/P` is then one line
//
// `readStructTree` lists a parent before its kids, and the elements are promoted in that order. So when an
// element is reached, its parent already has a number — its own, or the one it was given a moment ago — and
// the `/P` written is that reference. An inline element under an inline element needs no second pass.
//
// # What a promoted element is told, and what it is not
//
//   - **`/P`** names its parent, the root's reference for a top-level element. Written even where the inline
//     dictionary carried none: ISO 32000-1 Table 323 requires it.
//   - **Its element kids' `/P`** name it, where the tree lists the kid under it first (`deleteElement`'s
//     rule). Until now nothing could: whatever such a kid's `/P` said, it did not say its parent.
//   - **Each marked-content id it owns** gets its `/ParentTree` slot — the page's row, and for an MCR with a
//     `/Stm` that stream's row too — **where the slot is null or absent**. A slot that names another element
//     is a defect the tree already had, hidden because the checker could not ask it of an element with no
//     number (`checkStructConsistencyOn`); it is left exactly as it is, as `passSlot` leaves one.
//   - **An OBJR kid's annotation** gets its `/ParentTree` entry, where it carries a `/StructParent` that has
//     none. An annotation with no `/StructParent` is given no key: that is authoring the annotation, not
//     numbering the element.
//
// # What it never does
//
// No content stream changes by a byte, no `/K` changes its order, and the reading — every element's type,
// text, page, parent and kids — is what it was, with every id now above zero.
//
// # Bounds
//
// A row is grown to reach a marked-content id only up to `promoteMaxSlot`: the id is the document's own
// number, and a row filled with nulls up to two thousand million is a memory bomb one integer long. A row
// that does not exist is not created — the page's `/StructParents` named nothing before and names nothing
// after, which the checker already says.

// promoteMaxSlot is the highest marked-content id a `/ParentTree` row is GROWN to reach for a promoted
// element. A slot inside the row as written is filled whatever its index.
const promoteMaxSlot = 1 << 16

// promoteInline writes every inline element of tree as an indirect object (ADR-126) and returns the object
// numbers given. A tree with none is refused: a write that changes nothing would still be an undo step.
func promoteInline(ctx *model.Context, tree *structTree, ed structEdit) (map[int]bool, error) {
	if ed.elem != 0 || ed.value != "" || ed.parent != 0 || len(ed.headers) != 0 || ed.role != "" {
		return nil, fmt.Errorf("%w: making inline tags editable takes the whole document — it names no element, value, parent or header cells", ErrTagsReview)
	}
	rootRef, err := structTreeRootRef(ctx)
	if err != nil {
		return nil, err
	}
	promoted := map[int]bool{}
	// tree.elems is parent-first, so an inline element's parent has its number by the time it is reached.
	for _, e := range tree.elems {
		if e.objNr != 0 {
			continue
		}
		holder, parentRef := tree.root, *rootRef
		if e.parent != nil {
			if e.parent.objNr == 0 {
				return nil, fmt.Errorf("pdfops: an inline element was reached before the element that holds it had a number")
			}
			holder, parentRef = e.parent.dict, objectRef(ctx, e.parent.objNr)
		}
		ref, err := ctx.IndRefForNewObject(e.dict)
		if err != nil {
			return nil, err
		}
		// The dictionary's place in `/K` is found by identity: an inline dictionary is one Go map, listed once.
		kids, set := kidsArray(ctx, holder)
		at := -1
		for i, en := range kids {
			if d, ok := en.(types.Dict); ok && reflect.ValueOf(d).Pointer() == reflect.ValueOf(e.dict).Pointer() {
				at = i
				break
			}
		}
		if at < 0 {
			return nil, fmt.Errorf("pdfops: an inline element of type /%s is not in its parent's /K as read, so it cannot be given a number", e.kind)
		}
		kids[at] = *ref
		set(kids)
		e.objNr = ref.ObjectNumber.Value() // read by the element's own kids, promoted after it
		promoted[e.objNr] = true
		e.dict["P"] = parentRef

		for _, k := range e.kids {
			switch k.kind {
			case kidElement:
				// An inline kid is promoted in its own turn and given its `/P` there.
				if ir, isRef := k.raw.(types.IndirectRef); isRef && k.elem != nil && k.elem.parent == e {
					if kid := tree.byObj[ir.ObjectNumber.Value()]; kid == k.elem {
						kid.dict["P"] = *ref
					}
				}
			case kidMCID, kidMCR:
				if err := claimEmptySlot(ctx, tree, pageRowKey(ctx, k.pgObj), k.mcid, *ref); err != nil {
					return nil, err
				}
				// Content in another stream is owned through THAT stream's row as well (ADR-038).
				if k.stm != 0 {
					if err := claimEmptySlot(ctx, tree, streamRowKey(ctx, *types.NewIndirectRef(k.stm, 0)), k.mcid, *ref); err != nil {
						return nil, err
					}
				}
			case kidOBJR:
				d, derr := ctx.DereferenceDict(k.raw)
				if derr != nil || d == nil {
					continue
				}
				obj, oerr := ctx.DereferenceDict(d["Obj"])
				if oerr != nil || obj == nil {
					continue
				}
				key, ok := parentTreeKeyValue(ctx.XRefTable, obj["StructParent"])
				if !ok {
					continue
				}
				if h, _ := parentTreeLookup(ctx, tree.root["ParentTree"], key); h != nil {
					continue // it has an entry: this element's, or a defect the tree already had
				}
				// A key the number tree cannot take without re-sorting a leaf (`parentTreePlace`) stays without
				// an entry, as it was: the promotion is not refused over an annotation nothing named before.
				if _, _, _, perr := parentTreePlace(ctx, tree, key); perr != nil {
					continue
				}
				if err := setParentTreeSingle(ctx, tree, key, *ref); err != nil {
					return nil, err
				}
			}
		}
	}
	if len(promoted) == 0 {
		return nil, fmt.Errorf("%w: no tag in this document is written inline, so every tag can already be changed", ErrTagsReview)
	}
	return promoted, nil
}

// claimEmptySlot puts ref in the `/ParentTree` slot (key, mcid) where a reader finds NOTHING there: a null
// slot, or one past the end of the row as far as `promoteMaxSlot`. A slot that names anything, a key with no
// row, and a row that is not an array are left as they are. A negative key is "this page or stream has no
// key" (`pageRowKey`), never a row to look up: a malformed tree may hold a row under -1.
func claimEmptySlot(ctx *model.Context, tree *structTree, key, mcid int, ref types.IndirectRef) error {
	if key < 0 || mcid < 0 {
		return nil
	}
	holder, at := parentTreeLookup(ctx, tree.root["ParentTree"], key)
	if holder == nil {
		return nil
	}
	nums, _ := ctx.DereferenceArray(holder["Nums"])
	row, err := ctx.DereferenceArray(nums[at])
	if err != nil || row == nil {
		return nil
	}
	if mcid < len(row) {
		if row[mcid] != nil {
			return nil
		}
	} else if mcid > promoteMaxSlot {
		return nil
	}
	return setParentTreeSlot(ctx, tree, key, mcid, ref)
}

// defectSubject is the kind of a consistency defect and the object number of the element it is about, read
// from its key (`structDefect.key`: the kind, then `name=value` pairs); 0 where it names no element.
func defectSubject(key string) (kind string, obj int) {
	fields := strings.Fields(key)
	if len(fields) == 0 {
		return "", 0
	}
	for _, f := range fields[1:] {
		if v, ok := strings.CutPrefix(f, "obj="); ok {
			obj, _ = strconv.Atoi(v)
		}
	}
	return fields[0], obj
}

// defectsAPromotionUncovers adds, to the defects an edit batch is not blamed for, those the tree already
// had about an element `promoteInline` has just numbered. The checker keys a defect by the element's object
// number, and an inline element's was 0 — or, for a slot naming another element, the question could not be
// asked at all — so the same defect reads as new the moment the element can be named.
//
// **A null slot under an id the element claims is NOT among them**: filling those is the promotion's own
// job, and one left empty is a defect it added.
func defectsAPromotionUncovers(already map[string]bool, promoted map[int]bool, now []structDefect) {
	for _, d := range now {
		if kind, obj := defectSubject(d.key); promoted[obj] && kind != "mcid-unowned" {
			already[d.key] = true
		}
	}
}
