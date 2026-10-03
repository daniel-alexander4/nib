package pdfread

import (
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The tolerant walk — /pending 818.
//
// `walkPages` answers only the trees where one walk provably reproduces pdfcpu, and every other tree used to be
// handed to `ctx.PageDict(p, false)` page by page, which walks from the root at every call: quadratic on a flat tree.
// ONE attacker-written key reached it — a leaf's `/Count -1` or `/Kids []` — and 20,000 minimal pages (~1.2 MB) took
// 1m40-2m at every `Pages()` call (Scan, Inspect, uacheck, ceremony hashing), against 34 ms for the clean tree.
//
// `simulatePages` replays pdfcpu's own per-page algorithm (`processPageTreeForPageDictDepth`, model/xreftable.go
// v0.13.0) for every page, and shares the work between consecutive pages instead of re-walking from the root:
//
//   - The walk for page P consults P at exactly two kinds of decision — a node's `/Count` skip (`p + Count < P`) and
//     a leaf's descent (`p == P`). Everything else (types, dereferences, attributes, the visit set) is a function of
//     where the walk is, not of P.
//   - So the walk for P+1 IS the walk for P up to the first decision whose answer differs for P+1, and that decision
//     is found by key, not by search: a `/Count` decision differs exactly when `p + Count == P`, a leaf's when
//     `p == P` (see `loop` for why `p == P+1` never needs a key). Every decision is recorded on a trail with the state before it (frames are persistent,
//     so a snapshot is a pointer), indexed by the target at which it would flip; page P+1 resumes from the earliest
//     decision indexed under P+1, or — none being there — is answered exactly as P was.
//
// On a flat tree with one odd leaf every page resumes one decision back, so the walk is linear. A tree hostile to the
// sharing itself (every leaf odd, so pdfcpu's own answer re-walks the rest of the tree for every page) is bounded by
// `simBudget` and refused past it, page by page, with ErrPageTreeTooIrregular — never paid in full.
//
// An error is P-independent once reached, but its wording is pdfcpu's to give: the first page to reach an error site
// asks `PageDict` for it, and later pages reaching the same site reuse it.

// ErrPageTreeTooIrregular is the answer for a page beyond the tolerant walk's work budget.
var ErrPageTreeTooIrregular = errors.New("its page tree is too irregular to number its pages")

// simBudget is the work the tolerant walk may spend: a fixed allowance plus a multiple of the document, so a large
// legitimate document with an odd node is not refused for being large. One step is one kid visited or one decision
// replayed; a flat tree with one odd leaf costs about two per page (measured by TestATolerantWalkIsLinear).
func simBudget(xt *model.XRefTable) int {
	return 1<<16 + 8*(len(xt.Table)+max(xt.PageCount, 0))
}

type simFrame struct {
	parent *simFrame
	ref    types.IndirectRef
	kids   types.Array
	next   int
	depth  int
}

// simState is everything the walk carries. It is copied by value at every decision; frames are never mutated once
// made, so the copy is a snapshot.
type simState struct {
	top     *simFrame
	attrs   pathAttrs // pdfcpu threads ONE attributes value through the whole call, so a subtree entered and left leaks
	p       int
	entered int // the length of the visit set when this state was taken
}

type simKind uint8

const (
	simLeaf  simKind = iota // a `/Page` kid, after pdfcpu's `*p++`: descend iff p == target
	simCount                // a node's `/Count`: skip iff p + count < target
)

type simEvent struct {
	kind  simKind
	st    simState
	ref   types.IndirectRef
	dict  types.Dict
	count int
	depth int
	keys  []int
}

type simOutcome struct {
	dict  types.Dict
	ref   *types.IndirectRef
	attrs pathAttrs
	site  string // non-empty: pdfcpu answers with an error, reached here
	over  bool   // the budget ran out
}

type pageSim struct {
	xt     *model.XRefTable
	target int
	trail  []simEvent
	byKey  map[int][]int // target → trail positions whose decision flips at that target, ascending
	seen   []int         // PageTreeVisit's seen set, in entry order (so a resume truncates it)
	seenAt map[int]bool
	steps  int
	budget int
}

func (s *pageSim) record(ev simEvent) {
	pos := len(s.trail)
	s.trail = append(s.trail, ev)
	for _, k := range ev.keys {
		s.byKey[k] = append(s.byKey[k], pos)
	}
}

// rewind drops the trail from pos on, and the visit set back to what it was there.
func (s *pageSim) rewind(pos int, entered int) {
	for i := len(s.trail) - 1; i >= pos; i-- {
		for _, k := range s.trail[i].keys {
			l := s.byKey[k]
			if len(l) == 1 {
				delete(s.byKey, k)
			} else {
				s.byKey[k] = l[:len(l)-1]
			}
		}
	}
	s.trail = s.trail[:pos]
	for len(s.seen) > entered {
		delete(s.seenAt, s.seen[len(s.seen)-1])
		s.seen = s.seen[:len(s.seen)-1]
	}
}

func errAt(kind string, ref types.IndirectRef) simOutcome {
	return simOutcome{site: fmt.Sprintf("%s %d", kind, ref.ObjectNumber.Value())}
}

// enter is processPageTreeForPageDictDepth's prologue on kr, through its `/Count` decision.
func (s *pageSim) enter(st *simState, kr types.IndirectRef, depth int) (bool, simOutcome) {
	if s.xt.CheckRecursionDepth("page tree", depth) != nil {
		return true, errAt("depth", kr)
	}
	d, err := s.xt.DereferenceDict(kr)
	if err != nil {
		return true, errAt("node", kr)
	}
	if c := d.IntEntry("Count"); c != nil {
		s.record(simEvent{kind: simCount, st: *st, ref: kr, dict: d, count: *c, depth: depth, keys: []int{st.p + *c + 1}})
		return s.decideCount(st, kr, d, *c, depth)
	}
	return s.open(st, kr, d, depth)
}

func (s *pageSim) decideCount(st *simState, kr types.IndirectRef, d types.Dict, c, depth int) (bool, simOutcome) {
	if st.p+c < s.target {
		st.p += c // skip the subtree by its count
		return false, simOutcome{}
	}
	return s.open(st, kr, d, depth)
}

// open is the rest of the prologue: the attributes, then the node answered as the page or its kids pushed.
func (s *pageSim) open(st *simState, kr types.IndirectRef, d types.Dict, depth int) (bool, simOutcome) {
	if !st.attrs.apply(s.xt, d) {
		return true, errAt("attributes", kr)
	}
	kids := d.ArrayEntry("Kids")
	if kids == nil {
		r := kr
		return true, simOutcome{dict: d, ref: &r, attrs: st.attrs}
	}
	if nr := kr.ObjectNumber.Value(); nr != 0 { // PageTreeVisit.Enter ignores 0
		if s.seenAt[nr] {
			kind := "duplicate"
			for f := st.top; f != nil; f = f.parent {
				if f.ref.ObjectNumber.Value() == nr {
					kind = "cycle"
				}
			}
			return true, errAt(kind, kr)
		}
		s.seenAt[nr] = true
		s.seen = append(s.seen, nr)
		st.entered = len(s.seen)
	}
	st.top = &simFrame{parent: st.top, ref: kr, kids: kids, depth: depth}
	return false, simOutcome{}
}

func (s *pageSim) decideLeaf(st *simState, kr types.IndirectRef, depth int) (bool, simOutcome) {
	if st.p == s.target {
		return s.enter(st, kr, depth)
	}
	return false, simOutcome{}
}

// objType is pdfcpu's pageObjType on an already-dereferenced kid.
func objType(xt *model.XRefTable, d types.Dict) string {
	if t := d.Type(); t != nil {
		return *t
	}
	if xt.ValidationMode == model.ValidationRelaxed {
		if _, ok := d.Find("Count"); ok {
			if _, ok := d.Find("Kids"); ok {
				return "Pages"
			}
		}
	}
	return ""
}

// loop walks kids from st until the walk answers, errs, or runs out of tree.
func (s *pageSim) loop(st *simState) simOutcome {
	for {
		s.steps++
		if s.steps > s.budget {
			return simOutcome{over: true}
		}
		f := st.top
		if f == nil {
			return simOutcome{attrs: st.attrs} // the whole tree walked without reaching the page
		}
		if f.next >= len(f.kids) {
			st.top = f.parent
			continue
		}
		o := f.kids[f.next]
		nf := *f
		nf.next++
		st.top = &nf
		if o == nil {
			continue
		}
		kr, ok := o.(types.IndirectRef)
		if !ok {
			return errAt("kid", f.ref)
		}
		kd, err := s.xt.DereferenceDict(kr)
		if err != nil {
			return errAt("kid", kr)
		}
		var done bool
		var out simOutcome
		switch objType(s.xt, kd) {
		case "Pages":
			done, out = s.enter(st, kr, f.depth+1)
		case "Page":
			st.p++
			// The leaf flips at p+1 (descended for p, not for p+1). It also flips at p, but a leaf reaching p from
			// below always follows one at p-1 on the same trail — p climbs only by this increment, and a /Count skip
			// lands below the target — so that earlier leaf is found first and the key would never be read
			// (red-proof: dropping it survived as an equivalent mutant).
			s.record(simEvent{kind: simLeaf, st: *st, ref: kr, depth: f.depth + 1, keys: []int{st.p + 1}})
			done, out = s.decideLeaf(st, kr, f.depth+1)
		default:
			if s.xt.ValidationMode != model.ValidationRelaxed {
				return errAt("type", kr)
			}
		}
		if done {
			return out
		}
	}
}

// simulatePages answers PageDict(p, false) for p = 1..PageCount, in order. steps is the work it spent.
func simulatePages(ctx *model.Context) (out []Page, steps int) {
	xt := ctx.XRefTable
	n := max(ctx.PageCount, 0)
	out = make([]Page, 0, n)
	rootRef, err := xt.Pages()
	if err != nil || rootRef == nil {
		for p := 1; p <= n; p++ { // every call fails at the root, at no cost
			d, ref, attrs, err := ctx.PageDict(p, false)
			out = append(out, Page{Nr: p, Dict: d, Ref: ref, Attrs: attrs, Err: err})
		}
		return out, n
	}
	s := &pageSim{xt: xt, byKey: map[int][]int{}, seenAt: map[int]bool{}, budget: simBudget(xt)}
	errs := map[string]error{}
	var last simOutcome
	for p := 1; p <= n; p++ {
		s.target = p
		var o simOutcome
		var st simState
		var done bool
		switch l := s.byKey[p]; {
		case p == 1:
			done, o = s.enter(&st, *rootRef, 0)
		case len(l) > 0:
			ev := s.trail[l[0]]
			s.rewind(l[0], ev.st.entered)
			s.record(ev)
			st = ev.st
			if ev.kind == simLeaf {
				done, o = s.decideLeaf(&st, ev.ref, ev.depth)
			} else {
				done, o = s.decideCount(&st, ev.ref, ev.dict, ev.count, ev.depth)
			}
		default:
			o, done = last, true // no decision flips at p: pdfcpu walks exactly as it did for p-1
		}
		if !done {
			o = s.loop(&st)
		}
		last = o
		pg := Page{Nr: p}
		switch {
		case o.over || s.steps > s.budget:
			pg.Err = fmt.Errorf("nib will not number this document's pages: %w (more than %d steps)", ErrPageTreeTooIrregular, s.budget)
		case o.site != "":
			if e, ok := errs[o.site]; ok {
				pg.Err = e
				break
			}
			s.steps += p // PageDict walks from the root; charge it
			d, ref, attrs, err := ctx.PageDict(p, false)
			if err == nil { // the replay and pdfcpu parted: pdfcpu's answer stands
				pg = Page{Nr: p, Dict: d, Ref: ref, Attrs: attrs}
				break
			}
			errs[o.site], pg.Err = err, err
		default:
			attrs, ok := o.attrs.resolve(xt)
			if !ok { // apply admitted every rectangle, so this is unreachable; pdfcpu's answer stands
				d, ref, a, err := ctx.PageDict(p, false)
				pg = Page{Nr: p, Dict: d, Ref: ref, Attrs: a, Err: err}
				break
			}
			pg.Dict, pg.Ref, pg.Attrs = o.dict, o.ref, attrs
			if o.ref != nil {
				r := *o.ref
				pg.Ref = &r
			}
		}
		out = append(out, pg)
	}
	return out, s.steps
}
