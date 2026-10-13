package pdfops

import (
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The carried structure tree, ordered against the pages as they come OUT — `/pending 556`.
//
// The carry prunes the source tree onto the pages a selection keeps and leaves its elements in the order
// the SOURCE had them, so a reordered document was read aloud in the order its pages were in before the
// reorder. Measured before this was written, as the first page each top-level element's content is on:
//
//	Collect(["3","1","2"])      2 2 3 1 1      (the source's pages 1, 2, 3 — now 2, 3, 1)
//	Collect(["1","2","1","2"])  1 1 3 3 2 4    (each page followed by its own copy)
//
// Every element was anchored, every MCID resolved and the completeness gate was empty: nothing compares
// the tree's page sequence with the document's.
//
// # Who asks for it: a selection, never an imposition
//
// `treeOrder` is the caller's statement of what its page order IS. A reorder, a repeat or an extract in a
// chosen order is the order the document is now READ in, and its tree follows (`readInPageOrder`).
// `Booklet`'s sheet order is for the printer: page n sits beside page 1 so that the folded stack reads 1,
// 2, 3 — the reader's order is still the source's, and the tree keeps it (`readInSourceOrder`). Sorting
// there was built once and reverted: it put the census document's H2 before its H1 (ua1 7.4.2 t1).
//
// # The rule
//
// ONE level is ordered — the first, descending from the root, that holds more than one element
// (`orderLevel`) — so every element moves whole: a table's rows and a list's items stay as they were.
// It is sorted, stably, by the first OUTPUT page each element's content is on. Elements on one page keep
// the order they had; an element nothing locates travels with the element before it.
//
// **An element spanning pages the selection has separated keeps ONE place, by its first output page.** It
// is not split and not refused: a paragraph that runs from the source's page 1 onto its page 2 is read
// where the earlier of those two now comes out.
//
// **Only a tree that was in page order to begin with.** Read against the SOURCE's page numbers, the
// elements' first pages never go backwards. Then "tree order is page order" was the document's own
// convention and the sort continues it. Where an author put a note or a sidebar out of page order there is
// no telling which neighbour it belongs with once pages move, and the level is left exactly as it was.
//
// # What it can and cannot do to heading nesting
//
// Under that condition two elements change places only if a page of the later one now comes out before
// every page of the earlier one — that is, only where the selection itself turned two source pages round.
// So the sort introduces no mis-nesting among elements whose pages kept their relative order. Moving
// chapter 2 ahead of chapter 1 does put its headings first, and a document that then starts at an H2 fails
// ua1 7.4.2: that is what was asked for, the same honest consequence `RemovePages`' row in
// `knownUA1Deltas` records for deleting the page that held the only H1.
//
// Declared, not closed: a tree that was not in page order, or is deeper than the walk reads, is still
// read in source order after a reorder, and nothing tells the user.

// treeOrder says what a page selection's order means for the reading order of a carried tree.
type treeOrder bool

const (
	// readInSourceOrder leaves the carried tree as the source had it: the selection's order is not the
	// order the document is read in (an imposition).
	readInSourceOrder treeOrder = false
	// readInPageOrder orders the carried tree by the pages as they come out: the selection IS the
	// reading order (a reorder, a repeat, an extract).
	readInPageOrder treeOrder = true
)

// orderLevel descends from a structure root to the level where the document's order lives: the first that
// holds more than one element, whatever the wrappers above it are called — `/Document`, `/Part`, `/Sect`.
// It returns that level's kids and the element holding them (nil for the root itself).
//
// `placeInserted` (merge.go) makes the same descent inline; it predates this function.
func orderLevel(xt *model.XRefTable, kids types.Array) (parent types.Dict, own types.Array) {
	own = kids
	for depth := 0; len(own) == 1 && depth < 16; depth++ {
		_, isRef := own[0].(types.IndirectRef)
		d := derefDict(xt, own[0])
		below := rootKids(xt, d)
		if !isRef || d == nil || len(below) == 0 || !allElements(xt, below) {
			break
		}
		parent, own = d, below
	}
	return parent, own
}

// orderCarriedTree re-orders a carried tree's document level against the output's page order, under the
// rule above. keep is the selection: the source page number at each output position. It is called after
// `selectPages`, so the page tree it reads is the output's.
func orderCarriedTree(ctx *model.Context, keep []int) {
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		return
	}
	tree := derefDict(xt, root["StructTreeRoot"])
	leaves, _, err := collectLeaves(xt, root)
	if tree == nil || err != nil || len(leaves) != len(keep) {
		return
	}
	srcOf, outOf := map[int]int{}, map[int]int{}
	for i, l := range leaves {
		srcOf[l.ref.ObjectNumber.Value()] = keep[i]
		outOf[l.ref.ObjectNumber.Value()] = i + 1
	}
	parent, own := orderLevel(xt, rootKids(xt, tree))
	if len(own) < 2 || !allElements(xt, own) {
		return
	}
	keys := make([]int, len(own))
	lastSrc, lastKey := 0, 0
	for i, k := range own {
		on := map[int]bool{}
		if !contentPages(xt, k, 0, on, 0, map[int]bool{}) {
			return // deeper than the walk reads: not known, so not moved
		}
		firstSrc, firstOut := noPage, noPage
		for nr := range on {
			if out, kept := outOf[nr]; kept {
				firstSrc, firstOut = min(firstSrc, srcOf[nr]), min(firstOut, out)
			}
		}
		if firstOut == noPage {
			keys[i] = lastKey // travels with the element before it
			continue
		}
		// In page order to begin with.
		if firstSrc < lastSrc {
			return
		}
		lastSrc = firstSrc
		keys[i], lastKey = firstOut, firstOut
	}
	if sort.IntsAreSorted(keys) {
		return
	}
	idx := make([]int, len(own))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
	ordered := make(types.Array, len(own))
	for i, j := range idx {
		ordered[i] = own[j]
	}
	if parent == nil {
		tree["K"] = ordered
		return
	}
	parent["K"] = ordered
}

// contentPages records, in on, the object number of every page that content under o is on: an MCID is on
// the nearest `/Pg` above it, and a marked-content or annotation reference on its own. It is `firstPage`'s
// walk keeping the whole set, and reports false where it stopped at its depth bound instead of the end.
func contentPages(xt *model.XRefTable, o types.Object, inheritPg int, on map[int]bool, depth int, seen map[int]bool) bool {
	if depth > 64 {
		return false
	}
	if ref, ok := o.(types.IndirectRef); ok {
		if seen[ref.ObjectNumber.Value()] {
			return true
		}
		seen[ref.ObjectNumber.Value()] = true
	}
	if _, mcid := o.(types.Integer); mcid {
		on[inheritPg] = true
		return true
	}
	d := derefDict(xt, o)
	if d == nil {
		for _, k := range derefArray(xt, o) {
			if !contentPages(xt, k, inheritPg, on, depth+1, seen) {
				return false
			}
		}
		return true
	}
	if pg, ok := d["Pg"].(types.IndirectRef); ok {
		inheritPg = pg.ObjectNumber.Value()
	}
	if t, _ := d["Type"].(types.Name); t == "MCR" || t == "OBJR" {
		on[inheritPg] = true
		return true
	}
	return contentPages(xt, d["K"], inheritPg, on, depth+1, seen)
}
