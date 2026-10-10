package pdfread

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The depth pdfcpu recurses through every reference under a font or an image — /pending 840.
//
// The two passes before this one follow the edges pdfcpu's VALIDATOR follows. pdfcpu's optimize pass has three walks
// that follow every reference, whatever key it sits under, one recursion per link and with no bound on depth (v0.13.0):
//
//   - `fixReferencesToFreeObjects` (optimize.go:1526, first step of `OptimizeXRefTable`, :1642), from the catalog. nib
//     makes that walk itself, iteratively, and pdfcpu's then does not recurse (`fixFreeReferences`, optimize.go) — so
//     it needs no bound here, and gets none: see below.
//   - `traverseObjectGraphAndMarkDuplicates` (optimize.go:1042, with `traverse` :1019; from `calcRedundantObjects`
//     :1096, :1104), from every duplicate font and image.
//   - `model.EqualObjects` (model/equal.go:51 → `equalArrays` :121, `equalDicts` :214), from optimize.go:274 (two
//     fonts), :486 (two images) and :581 (two forms, which `Unaffordable`'s `shape` bounds at `maxShapeDepth`).
//
// This pass bounds the last two: the deepest path through all references from any font dictionary or image XObject —
// what pdfcpu takes for one (`DereferenceFontDict`, model/dereference.go:414-419; `/Subtype /Image`) — counted in
// LEVELS, one per container pdfcpu steps into: the object a reference names, and each array or dictionary nested
// directly inside it on the way to the next reference (the parser bounds that nesting, model/parse.go:325).
//
// Which entry pdfcpu takes first is its map order, so this is an upper bound over every order: strongly connected
// components (Tarjan, iteratively), each charged the levels of ALL its members, and the longest path through their
// condensation. **That charge is why the walk starts at fonts and images and nowhere else.** From the catalog it
// would reach the page tree, which names itself back through `/Parent`, and charge a whole document as one chain:
// 37,383 levels for a 67-page document whose real pass is 12 to 19 objects deep, so a thousand pages would be
// refused. A font or an image names its descriptor, its programs, its masks and colour spaces — nothing that names
// the page tree — so an honest document measures the same however many pages it has (`TestALargeHonestDocument…`).

// maxObjectLevels is the most levels pdfcpu may recurse through the references under a font or an image. Measured on
// the one all-reference walk that could be run alone (the free-reference pass, stack capped at 64 MiB): a chain of
// dictionaries ran out between 120,002 and 140,002 levels (at most 559 bytes a level), of arrays between 160,002 and
// 180,002. Go ends a goroutine whose stack would grow past 512 MiB, so this is about 55% of what that leaves.
// `traverse`'s and `EqualObjects`' own cost a level is NOT measured.
const maxObjectLevels = 1 << 19

// refuseDeepReferences refuses ctx when a chain of references under one of its fonts or images, under any key, is
// deeper than `maxObjectLevels`.
func refuseDeepReferences(ctx *model.Context) error {
	if d, nr := objectLevels(ctx); d > maxObjectLevels {
		return fmt.Errorf("nib will not read this document: %w (more than %d levels under the font or image in "+
			"object %d, counting every reference under any key), and pdfcpu's optimizer would recurse through every level",
			ErrReferenceDepth, maxObjectLevels, nr)
	}
	return nil
}

// objectLevels is the deepest chain of levels through the references under any font or image of ctx, capped just past
// `maxObjectLevels`, and the font or image it was measured from.
func objectLevels(ctx *model.Context) (deepest, from int) {
	nrs := make([]int, 0, len(ctx.Table))
	for nr, e := range ctx.Table {
		if e != nil && !e.Free && e.Object != nil {
			nrs = append(nrs, nr)
		}
	}
	sort.Ints(nrs)
	pos := make(map[int]int32, len(nrs))
	for i, nr := range nrs {
		pos[nr] = int32(i)
	}
	w := &levelWalk{ctx: ctx, nrs: nrs, pos: pos, state: make([]uint8, len(nrs)), index: make([]int32, len(nrs)),
		low: make([]int32, len(nrs)), depth: make([]int32, len(nrs)), weight: make([]int32, len(nrs))}
	for i, nr := range nrs {
		if !fontOrImage(ctx.Table[nr].Object) {
			continue
		}
		if w.state[i] == 0 {
			w.from(int32(i))
		}
		if d := int(w.depth[i]); d > deepest {
			deepest, from = d, nrs[i]
		}
	}
	return deepest, from
}

// fontOrImage reports whether pdfcpu's optimize pass could take o for a font or an image: the two kinds it compares
// with `EqualObjects` and walks with `traverse`.
func fontOrImage(o types.Object) bool {
	switch o := o.(type) {
	case types.Dict:
		return nameOf(o, "Type") == "Font"
	case types.StreamDict:
		return nameOf(o.Dict, "Subtype") == "Image" || nameOf(o.Dict, "Type") == "Font"
	}
	return false
}

// levelWalk is Tarjan's walk over the live objects it reaches, by dense position. depth holds, for an object still open, the
// deepest finished component one of its references leaves for, and for a finished one its component's depth.
type levelWalk struct {
	ctx     *model.Context
	nrs     []int
	pos     map[int]int32
	state   []uint8 // 0 unvisited, 1 on Tarjan's stack, 2 finished
	index   []int32
	low     []int32
	depth   []int32
	weight  []int32 // the levels an object adds to a path through it
	counter int32
	stack   []int32
	adj     []int32 // the open objects' references, each frame's above its parent's
	nested  []nestedObject
}

type nestedObject struct {
	o     types.Object
	level int32
}

type levelFrame struct {
	v                int32
	start, next, end int32
}

// open puts v on Tarjan's stack and appends the objects it names to adj. Its weight is the deepest nesting it holds a
// reference at: the levels pdfcpu is inside it when it leaves for another object.
func (w *levelWalk) open(v int32) levelFrame {
	w.index[v], w.low[v] = w.counter, w.counter
	w.counter++
	w.state[v] = 1
	w.stack = append(w.stack, v)
	f := levelFrame{v: v, start: int32(len(w.adj))}
	weight := int32(1)
	w.nested = append(w.nested[:0], nestedObject{w.ctx.Table[w.nrs[v]].Object, 1})
	for len(w.nested) > 0 {
		n := w.nested[len(w.nested)-1]
		w.nested = w.nested[:len(w.nested)-1]
		visit := func(o types.Object) {
			switch o := o.(type) {
			case types.IndirectRef:
				if j, ok := w.pos[o.ObjectNumber.Value()]; ok {
					w.adj = append(w.adj, j)
					weight = max(weight, n.level)
				}
			case types.Dict, types.Array:
				w.nested = append(w.nested, nestedObject{o, n.level + 1})
			}
		}
		switch o := n.o.(type) {
		case types.Dict:
			for _, x := range o {
				visit(x)
			}
		case types.StreamDict:
			for _, x := range o.Dict {
				visit(x)
			}
		case types.Array:
			for _, x := range o {
				visit(x)
			}
		}
	}
	w.weight[v] = weight
	f.next, f.end = f.start, int32(len(w.adj))
	return f
}

func (w *levelWalk) from(root int32) {
	call := []levelFrame{w.open(root)}
	for len(call) > 0 {
		f := &call[len(call)-1]
		v := f.v
		if f.next < f.end {
			x := w.adj[f.next]
			f.next++
			switch w.state[x] {
			case 0:
				call = append(call, w.open(x))
			case 1:
				w.low[v] = min(w.low[v], w.index[x])
			default:
				w.depth[v] = max(w.depth[v], w.depth[x])
			}
			continue
		}
		w.adj = w.adj[:f.start]
		call = call[:len(call)-1]
		if w.low[v] == w.index[v] {
			w.finish(v)
		}
		if len(call) > 0 {
			p := call[len(call)-1].v
			if w.state[v] == 2 {
				w.depth[p] = max(w.depth[p], w.depth[v])
			} else {
				w.low[p] = min(w.low[p], w.low[v])
			}
		}
	}
}

// finish pops v's component and charges it: every member's levels, on top of the deepest component any of them
// leaves for.
func (w *levelWalk) finish(v int32) {
	i := len(w.stack) - 1
	for w.stack[i] != v {
		i--
	}
	members := w.stack[i:]
	w.stack = w.stack[:i]
	var below, sum int64
	for _, m := range members {
		below = max(below, int64(w.depth[m]))
		sum += int64(w.weight[m])
	}
	d := int32(min(below+sum, maxObjectLevels+1))
	for _, m := range members {
		w.depth[m], w.state[m] = d, 2
	}
}
