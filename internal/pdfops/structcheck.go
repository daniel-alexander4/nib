package pdfops

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure tree's consistency invariants — `PLAN-accessibility.md` P05.S03, D8.
//
// # What `/ParentTree` actually is, measured
//
// It is a number tree with **two different kinds of entry**, and a model that treats it as
// "key → element" destroys one of them. Measured on a LibreOffice HTML conversion:
//
//	key 0 -> an ARRAY of 23 element references   ← a PAGE's entry, indexed BY MCID
//	key 1 -> a single element reference          ← an ANNOTATION's entry
//
// A page carries `/StructParents` (plural) and its entry is an array whose *index is the MCID*: slot
// 7 is the element owning `/MCID 7` on that page. An annotation or Form XObject carries
// `/StructParent` (singular) and its entry is one reference. The two spellings differ by one letter
// and mean different shapes.
//
// # Why a checker before a writer
//
// The write half's job is to keep these true, and a post-condition nobody can evaluate is a
// post-condition nobody has. Building the checker first also makes it a reader for documents that
// already exist — the corpus, nib's authored output, and what `carryTagsThroughNUp` produces —
// which is where a defect would already be sitting rather than where one might later be introduced.

// structDefect is one broken invariant, named in the document's own terms.
type structDefect struct {
	// key identifies the defect by what is broken — a page's key, an MCID, the owning object — and never
	// by an element's type, so an edit that retypes an element does not make an old defect read as new
	// (P09.S02 compares keys before and after an edit).
	key  string
	what string
}

func (d structDefect) String() string { return d.what }

// checkStructConsistency reports every way a document's structure tree contradicts itself.
//
// An empty result means the four invariants below hold. It is NOT a PDF/UA verdict — a document can
// be perfectly self-consistent and carry no useful structure at all.
//
// The invariants, each stated as the failure it catches:
//
//  1. **A page's `/StructParents` resolves to a ParentTree entry.** Without it the page declares a
//     key into a table that has no such row, and every MCID on that page is unreachable from the
//     tree — which looks exactly like a tagged page and is not one.
//  2. **A page's entry is an ARRAY**, because it is indexed by MCID. A single reference there means
//     every MCID past 0 resolves to nothing.
//  3. **Every MCID an element claims is within its page's array**, or the element owns marked
//     content the ParentTree cannot name.
//  4. **An element reached through a page's array is the element that claims that MCID.** The two
//     directions are stored separately — `/K` holds MCIDs, the ParentTree holds elements — so
//     nothing but a check makes them agree.
func checkStructConsistency(ctx *model.Context, tree *structTree) []structDefect {
	var out []structDefect
	add := func(key, f string, a ...any) { out = append(out, structDefect{key: key, what: fmt.Sprintf(f, a...)}) }

	nums, single := parentTreeEntries(ctx, tree)

	// Page object number -> its /StructParents key, and the reverse.
	pageKey := map[int]int{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil || d == nil {
			continue
		}
		ir, e := ctx.PageDictIndRef(p)
		if e != nil || ir == nil {
			continue
		}
		spRaw, has := d["StructParents"]
		if !has {
			continue // an untagged page in a tagged document is ordinary
		}
		sp, ok := spRaw.(types.Integer)
		if !ok {
			add(fmt.Sprintf("structparents-type page=%d", p), "page %d has a /StructParents that is not an integer (%T)", p, spRaw)
			continue
		}
		key := sp.Value()
		pageKey[ir.ObjectNumber.Value()] = key
		arr, isArray := nums[key]
		if !isArray {
			if _, isSingle := single[key]; isSingle {
				add(fmt.Sprintf("structparents-single page=%d key=%d", p, key), "page %d declares /StructParents %d, and that ParentTree entry is a single "+
					"element rather than an array — every MCID on the page past 0 resolves to nothing",
					p, key)
			} else {
				add(fmt.Sprintf("structparents-missing page=%d key=%d", p, key), "page %d declares /StructParents %d, and the ParentTree has no entry %d — "+
					"every MCID on that page is unreachable from the tree", p, key, key)
			}
			continue
		}
		_ = arr
	}

	// Every MCID an element claims must be in range, and must map back to that element.
	for _, e := range tree.elems {
		for _, k := range e.kids {
			if k.kind != kidMCID && k.kind != kidMCR {
				continue
			}
			pg := k.pgObj
			if pg == 0 {
				add(fmt.Sprintf("mcid-no-page obj=%d mcid=%d", e.objNr, k.mcid), "an element of type /%s owns /MCID %d and names no page", e.kind, k.mcid)
				continue
			}
			key, known := pageKey[pg]
			if !known {
				// The page is not in the page tree, or carries no /StructParents. `orphaned()`
				// already reports the first; the second is this.
				continue
			}
			arr := nums[key]
			if k.mcid < 0 || k.mcid >= len(arr) {
				add(fmt.Sprintf("mcid-range obj=%d key=%d mcid=%d", e.objNr, key, k.mcid), "an element of type /%s claims /MCID %d, and the ParentTree array for its "+
					"page (key %d) has %d slot(s)", e.kind, k.mcid, key, len(arr))
				continue
			}
			if e.objNr != 0 && arr[k.mcid] != 0 && arr[k.mcid] != e.objNr {
				add(fmt.Sprintf("mcid-owner key=%d mcid=%d obj=%d", key, k.mcid, e.objNr), "/MCID %d on the page with key %d maps to object %d in the ParentTree, but "+
					"object %d is the element that claims it", k.mcid, key, arr[k.mcid], e.objNr)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].what < out[j].what })
	return out
}

// parentTreeEntries reads the `/ParentTree` number tree into the two shapes it actually holds:
// array entries (a page's, indexed by MCID, as object numbers with 0 for an empty slot) and single
// entries (an annotation's or XObject's).
//
// **`/Kids` is followed**, because a number tree is allowed to be nested and a flat `/Nums` is only
// what small documents happen to produce. A reader that handled only `/Nums` would report every
// entry of a large document as missing — the loudest possible wrong answer, which is at least
// better than the quiet one, but still wrong.
func parentTreeEntries(ctx *model.Context, tree *structTree) (arrays map[int][]int, singles map[int]int) {
	arrays, singles = map[int][]int{}, map[int]int{}
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
				val := nums[i+1]
				if arr, ae := ctx.DereferenceArray(val); ae == nil && arr != nil {
					slots := make([]int, len(arr))
					for j, x := range arr {
						if ind, isInd := x.(types.IndirectRef); isInd {
							slots[j] = ind.ObjectNumber.Value()
						}
					}
					arrays[n.Value()] = slots
					continue
				}
				if ind, isInd := val.(types.IndirectRef); isInd {
					singles[n.Value()] = ind.ObjectNumber.Value()
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
	return arrays, singles
}
