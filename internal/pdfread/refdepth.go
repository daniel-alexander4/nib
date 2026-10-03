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
// path through their condensation, where a component of n nodes is charged ((n+2)/2)² — a stack inside one component
// can revisit an unguarded node once per guarded object it passes, so n alone is not a bound; an acyclic node is 1.
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
	t := tarjan{g: g, nrs: nrs, at: map[int]*tnode{}}
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
	next       int
	scc        *component
}

// component is a finished strongly connected component: its nodes' depth is depth.
type component struct{ depth int }

type tarjan struct {
	g       *refGraph
	nrs     []int          // position → object number, `pos` inverted
	at      map[int]*tnode // by slot
	counter int
	stack   []*tnode
}

// get returns slot i's tnode, opening it — its edges, guarded ones included — the first time.
func (t *tarjan) get(i int, n node) *tnode {
	if tn, ok := t.at[i]; ok {
		return tn
	}
	tn := &tnode{n: n, index: -1}
	if e, ok := t.g.ctx.Table[n.nr]; ok && e != nil && !e.Free {
		t.g.expand(e.Object, n.role, "", func(to node, _ string) {
			if j := t.g.slot(to); j >= 0 {
				tn.edges = append(tn.edges, j)
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
	n := len(members)
	weight := 1
	if n > 1 || selfLoop {
		h := (n + 2) / 2
		weight = min(h*h, maxReferenceDepth+1)
	}
	c.depth = min(below+weight, maxReferenceDepth+1)
}
