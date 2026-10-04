package pdfread

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The depth the validator recurses through GUARDED edges — /pending 803.
//
// The path count (`validatorPaths`) stops at every object pdfcpu marks before recursing through it, because pdfcpu
// handles a loop there and validates the object once. Once is not shallow. A loop-free chain of form XObjects, each
// naming the next in its `/Resources /XObject`, is validated one recursion per link
// (`validateXObjectStreamDict` → `validateFormStreamDict` → `validateResourceDict` → …), and the door followed none
// of it. Measured on `Validated`: 300,000 links (51 MB) 9.5 s and returned; 700,000 links (~120 MB, inside ADR-005's
// 512 MiB) `fatal error: stack overflow` — no recover holds that, so the process went with every open document.
//
// So the door also bounds the deepest recursion pdfcpu could take through every edge it follows, guarded or not.
// Which object pdfcpu reaches first depends on its map order, so this is not a simulation of one walk: it is an upper
// bound over all of them. Strongly connected components are found (Tarjan, iteratively), and the depth is the longest
// path through their condensation, where a component is charged (g+1)·l (ADR-081): pdfcpu marks a guarded object
// before recursing through it, so a stack inside one component enters each of its g guarded TARGETS at most once, and
// between two such entries it follows only unguarded edges, which carry no loop — at most l nodes, the longest
// unguarded path inside the component. An acyclic node is 1. ADR-077 charged ((n+2)/2)², the worst g and l any split
// of n nodes allows, and so refused 2,000 forms naming their own `/XObject` dictionary — every edge guarded, l = 1,
// and pdfcpu 2,000 levels deep, not a million (`/pending 803`'s regression).
// Past `maxReferenceDepth` the document is refused with `ErrReferenceDepth`, the bound the unguarded walk keeps.

// validatorDepth refuses ctx when the deepest recursion pdfcpu's validator could take through its references, guarded
// edges included, is past `maxReferenceDepth`. It runs after `validatorPaths`, so the unguarded edges carry no loop.
func validatorDepth(ctx *model.Context) error {
	nrs := make([]int, 0, len(ctx.Table))
	for nr, e := range ctx.Table {
		if e != nil && !e.Free && e.Object != nil {
			nrs = append(nrs, nr)
		}
	}
	sort.Ints(nrs)
	pos := make(map[int]int, len(nrs))
	for i, nr := range nrs {
		pos[nr] = i
	}
	g := &refGraph{ctx: ctx, pos: pos, guarded: true}
	t := tarjan{g: g, ug: &refGraph{ctx: ctx, pos: pos}, nrs: nrs, at: map[int]*tnode{}}
	for _, nr := range nrs {
		for _, r := range rootRoles(ctx.Table[nr].Object) {
			if d, root := t.depthFrom(node{nr, r}); d > maxReferenceDepth {
				return fmt.Errorf("nib will not read this document: %w (more than %d levels from object %d, through "+
					"its forms, fonts, images or appearance streams), and pdfcpu's validator would recurse through every "+
					"level", ErrReferenceDepth, maxReferenceDepth, root)
			}
		}
	}
	return nil
}

// tnode is one (object, role) in Tarjan's walk.
type tnode struct {
	n          node
	index, low int
	onStack    bool
	edges      []int // slots
	unguarded  []int // the slots of `edges` pdfcpu follows without a mark
	guarded    []int // the rest: their targets are entered once per walk
	next       int
	scc        *component
	target     bool // a guarded edge inside its component enters it (`finish`)
	long       int  // the longest unguarded path inside its component starting here, in nodes; 0 unmeasured
	opened     bool // on `longest`'s path
}

// component is a finished strongly connected component: its nodes' depth is depth.
type component struct{ depth int }

type tarjan struct {
	g       *refGraph
	ug      *refGraph      // g without its guarded edges: what tells the two kinds apart
	nrs     []int          // position → object number, `pos` inverted
	at      map[int]*tnode // by slot
	counter int
	stack   []*tnode
}

// get returns slot i's tnode, opening it — its edges, guarded ones included — the first time. The guarded edges are
// what the full expansion emits beyond the unguarded one, counted as a multiset: the same object can be named both
// ways (a form that is also a tiling pattern), and only the guarded naming enters it once.
func (t *tarjan) get(i int, n node) *tnode {
	if tn, ok := t.at[i]; ok {
		return tn
	}
	tn := &tnode{n: n, index: -1}
	if e, ok := t.g.ctx.Table[n.nr]; ok && e != nil && !e.Free {
		t.ug.expand(e.Object, n.role, "", func(to node, _ string) {
			if j := t.g.slot(to); j >= 0 {
				tn.unguarded = append(tn.unguarded, j)
			}
		})
		var plain map[int]int
		if len(tn.unguarded) > 0 {
			plain = make(map[int]int, len(tn.unguarded))
			for _, j := range tn.unguarded {
				plain[j]++
			}
		}
		t.g.expand(e.Object, n.role, "", func(to node, _ string) {
			if j := t.g.slot(to); j >= 0 {
				tn.edges = append(tn.edges, j)
				if plain[j] > 0 {
					plain[j]--
				} else {
					tn.guarded = append(tn.guarded, j)
				}
			}
		})
	}
	t.at[i] = tn
	return tn
}

// depthFrom finishes every component reachable from root and returns root's depth, with the object it was measured
// from for the refusal's wording.
func (t *tarjan) depthFrom(root node) (int, int) {
	ri := t.g.slot(root)
	if ri < 0 {
		return 0, root.nr
	}
	r := t.get(ri, root)
	if r.scc != nil {
		return r.scc.depth, root.nr
	}
	if r.index >= 0 {
		return 0, root.nr // on the stack of a walk still running: that walk measures it
	}
	call := []*tnode{r}
	t.visit(r)
	for len(call) > 0 {
		v := call[len(call)-1]
		if v.next < len(v.edges) {
			j := v.edges[v.next]
			v.next++
			w := t.at[j]
			if w == nil {
				w = t.get(j, node{t.nrs[j/int(numRoles)], role(j % int(numRoles))})
			}
			switch {
			case w.index < 0:
				t.visit(w)
				call = append(call, w)
			case w.onStack:
				v.low = min(v.low, w.index)
			}
			continue
		}
		call = call[:len(call)-1]
		if len(call) > 0 {
			p := call[len(call)-1]
			p.low = min(p.low, v.low)
		}
		if v.low == v.index {
			t.finish(v)
		}
	}
	return r.scc.depth, root.nr
}

func (t *tarjan) visit(v *tnode) {
	v.index, v.low = t.counter, t.counter
	t.counter++
	v.onStack = true
	t.stack = append(t.stack, v)
}

// finish pops v's component and charges it: its own weight plus the deepest component any of its edges leaves for,
// every one of which Tarjan has already finished.
func (t *tarjan) finish(v *tnode) {
	c := &component{}
	var members []*tnode
	for {
		w := t.stack[len(t.stack)-1]
		t.stack = t.stack[:len(t.stack)-1]
		w.onStack = false
		w.scc = c
		members = append(members, w)
		if w == v {
			break
		}
	}
	below := 0
	selfLoop := false
	for _, w := range members {
		for _, j := range w.edges {
			x := t.at[j]
			if x.scc == c {
				selfLoop = true
				continue
			}
			below = max(below, x.scc.depth)
		}
	}
	weight := 1
	if passThrough(v.n.role) {
		weight = 0
	}
	if len(members) > 1 || selfLoop {
		weight = t.charge(c, members)
	}
	c.depth = min(below+weight, maxReferenceDepth+1)
}

// passThrough is a role that is no level of pdfcpu's recursion: an indirect dictionary the depth pass makes a node only
// so that sharing it costs edges linearly (refgraph.go). Every edge into and out of one is guarded.
func passThrough(r role) bool { return r >= roleSharedResources }

// charge is a cyclic component's weight, (g+1)·l: g the members a guarded edge inside the component enters, l the
// longest unguarded path inside it (see the file comment). Capped past the bound.
func (t *tarjan) charge(c *component, members []*tnode) int {
	g := 0
	for _, w := range members {
		for _, j := range w.guarded {
			if x := t.at[j]; x.scc == c && !x.target && !passThrough(x.n.role) {
				x.target = true
				g++
			}
		}
	}
	l := 0
	for _, w := range members {
		if passThrough(w.n.role) {
			continue // no unguarded edge leaves one, and it is no level
		}
		d, loop := t.longest(c, w)
		if loop {
			l = max(len(members), t.g.ctx.MaxRecursionDepth()+1)
			break
		}
		l = max(l, d)
	}
	return int(min(int64(g+1)*int64(l), maxReferenceDepth+1))
}

// longest is the longest path from v through c's unguarded edges, in nodes, walked iteratively. `validatorPaths` has
// refused every unguarded loop but a tree's, which pdfcpu cuts at `MaxRecursionDepth` (100) levels itself
// (nameTree.go:705, numberTree.go:173), so one found here reports loop and `charge` takes the larger of that cut and
// every node once.
func (t *tarjan) longest(c *component, v *tnode) (long int, loop bool) {
	if v.long > 0 {
		return v.long, false
	}
	type step struct {
		v    *tnode
		next int
	}
	call := []step{{v: v}}
	v.opened = true
	for len(call) > 0 {
		s := &call[len(call)-1]
		if s.next < len(s.v.unguarded) {
			w := t.at[s.v.unguarded[s.next]]
			s.next++
			switch {
			case w.scc != c:
			case w.opened:
				return 0, true
			case w.long == 0:
				w.opened = true
				call = append(call, step{v: w})
			}
			continue
		}
		u := s.v
		u.long = 1
		for _, j := range u.unguarded {
			if w := t.at[j]; w.scc == c {
				u.long = max(u.long, 1+w.long)
			}
		}
		u.opened = false
		call = call[:len(call)-1]
	}
	return v.long, false
}
