package pdfread

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The depth pdfcpu recurses through EVERY reference — /pending 840.
//
// The two passes before this one follow the edges pdfcpu's VALIDATOR follows. pdfcpu's optimize pass and its writer
// follow all of them, whatever key they sit under, one recursion per link and with no bound on depth (v0.13.0):
//
//   - `fixReferencesToFreeObjects` (optimize.go:1526, first step of `OptimizeXRefTable`, :1642) walks from the catalog
//     through every dictionary entry and array element: `fixDirectObject` :1440 → `fixDeepObject` :1516 →
//     `fixIndirectObject` :1467 → `fixDeepDict` :1412 / `fixDeepArray` :1426 → `fixDeepObject`. It enters an object
//     once (`ctx.Optimize.Cache`, :1470-1473), which bounds its work and not its depth.
//   - `traverseObjectGraphAndMarkDuplicates` (optimize.go:1042, with `traverse` :1019; from `calcRedundantObjects`
//     :1096, :1104) walks every reference under a duplicate font or image, and marks nothing it has entered.
//   - `model.EqualObjects` (model/equal.go:51 → `equalArrays` :121, `equalDicts` :214), from optimize.go:274, :486 and
//     :581, follows both objects' references pairwise.
//   - The writer: `writeDeepObject` (writeObjects.go:794) → `writeIndirectObject` :754 → `writeObjectGeneric` :714 →
//     `writeDeepDict` :639 / `writeDeepStreamDict` :668 / `writeDeepArray` :688 → `writeDeepObject`, entering an
//     object once (`HasWriteOffset`, :758).
//
// So `N 0 obj [N+1 0 R]` chained under a catalog key the validator ignores passed the door: 100,000 links read, and
// 1,500,000 (80 MB, inside ADR-005's 512 MiB) were a fatal stack overflow in `fixIndirectObject`, which no recover
// holds. This pass bounds the deepest path through all of a document's references, counted in LEVELS — one per
// container pdfcpu steps into: the object a reference names, and each array or dictionary nested directly inside it on
// the way to the next reference (the parser bounds that nesting at `MaxRecursionDepth`, model/parse.go:325).
//
// Which entry pdfcpu takes first is its map order, so this is an upper bound over every order, as `validatorDepth` is:
// strongly connected components (Tarjan, iteratively), each charged the levels of ALL its members — a walk that enters
// an object once crosses a component along a simple path, and no simple path is longer than that — and the depth is
// the longest path through their condensation. A document's pages, structure tree, outlines and annotations name each
// other back and forth and are ordinarily ONE component, so the bound is in effect on how many objects a document
// links mutually; see `maxObjectLevels` for where it sits.

// maxObjectLevels is the most levels pdfcpu may recurse through a document's references. Measured on the optimize
// pass with the stack capped at 64 MiB: a chain of dictionaries ran out between 120,002 and 140,002 levels (at most
// 559 bytes a level), of arrays between 160,002 and 180,002, of arrays nested twenty deep between 240,002 and 320,002.
// Go ends a goroutine whose stack would grow past 512 MiB, so this bound is about 55% of what the worst of those
// leaves. The writer's and `EqualObjects`' cost a level is NOT measured (the writer did not follow the two chains
// tried, under a catalog key and under a page key).
//
// **Declared:** the bound counts a component whole, and pdfcpu's own walk is far shallower on a real document — the
// largest of nib's producer corpus measures 37,383 levels here and pdfcpu's path through it is 12 to 19 objects
// (simulated, five map orders). So a document linking more than this many levels mutually — about fourteen times that
// one — is refused though pdfcpu would read it.
const maxObjectLevels = 1 << 19

// refuseDeepReferences refuses ctx when a chain of its references, under any key, is deeper than `maxObjectLevels`.
func refuseDeepReferences(ctx *model.Context) error {
	if d, nr := objectLevels(ctx); d > maxObjectLevels {
		return fmt.Errorf("nib will not read this document: %w (more than %d levels through object %d, counting "+
			"every reference under any key), and pdfcpu's optimizer and writer would recurse through every level",
			ErrReferenceDepth, maxObjectLevels, nr)
	}
	return nil
}

// objectLevels is the deepest chain of levels through ctx's references, capped just past `maxObjectLevels`, and the
// object it was measured from.
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
	for i := range nrs {
		if w.state[i] == 0 {
			w.from(int32(i))
		}
		if d := int(w.depth[i]); d > deepest {
			deepest, from = d, nrs[i]
		}
	}
	return deepest, from
}

// levelWalk is Tarjan's walk over every live object, by dense position. depth holds, for an object still open, the
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
