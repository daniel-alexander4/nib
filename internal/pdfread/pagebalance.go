package pdfread

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The optimize pass's per-page resource step, made linear on a flat page tree — `/pending 825`, `/pending 763`.
//
// pdfcpu's `optimizeResourceDicts` (optimize.go:1592, v0.13.0) asks `PageDict(i, true)` for every page, and `PageDict`
// walks from the root: at a `/Pages` node it skips a child `/Pages` node whose `/Count` ends before the page, but it
// must look at every LEAF before the one it wants (`processPageTreeKidForPageDict` dereferences each to read its
// `/Type`). On a flat tree — what pdfcpu's own writer and nib's Markdown conversion produce — that is n²/2 lookups:
// 4.1 s on a clean 5,000-page document, ~15 s of a 7,059-page one, through every `ReadOptimized` whose command runs
// the step (VALIDATE, the default, among them) and every `Optimize`.
//
// **Nothing about the step is changed — only the shape of the tree it walks, and only while it walks.** For the
// pass, each `/Pages` node with more than `pageNodeFanout` kids has its `/Kids` replaced by a balanced tree of
// temporary `/Pages` nodes, each with the `/Count` of what it holds, so `PageDict` skips whole groups by count, as it
// already does for a real subtree: O(fanout · depth) a page. The temporary nodes carry no inheritable attribute,
// so every page inherits exactly what it did; they hold the same kids in the same order, so every page has the same
// number; and they live in the xref table only for the pass — numbered past every object in the file, under no
// `/Size`, removed and every `/Kids` restored before the pass returns, panic or not. What the pass then writes is the
// tree it was given. `TestBalancingThePageTreeChangesNothingThePassWrites` holds the written output identical over the
// corpora.
//
// It declines — leaving the tree as it is, and the step quadratic — on any node pdfcpu's own walk would read
// differently from a balanced one: a kid that is not a reference, a kid with no `/Type`, a `/Pages` kid with no
// `/Count`, a `/Kids` given by reference, or a node reached twice.

// pageNodeFanout is the most kids a `/Pages` node keeps through the pass, and so what a page lookup scans per level.
var pageNodeFanout = 64 // a var only so the differential can force the reshaping onto every document

// balancePageTreeForPass reshapes ctx's page tree as described above and returns the function that restores it.
func balancePageTreeForPass(ctx *model.Context) (restore func()) {
	root, err := ctx.Pages()
	if err != nil || root == nil {
		return func() {}
	}
	type saved struct {
		d    types.Dict
		kids types.Object
	}
	var undo []saved
	var temps []int
	restore = func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i].d["Kids"] = undo[i].kids
		}
		for _, nr := range temps {
			delete(ctx.Table, nr)
		}
		undo, temps = nil, nil
	}
	next := 0
	for nr := range ctx.Table {
		next = max(next, nr+1)
	}
	if ctx.Size != nil {
		next = max(next, *ctx.Size)
	}
	visited := map[int]bool{}
	stack := []types.IndirectRef{*root}
	for len(stack) > 0 {
		ir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[ir.ObjectNumber.Value()] {
			restore()
			return func() {}
		}
		visited[ir.ObjectNumber.Value()] = true
		d, err := ctx.DereferenceDict(ir)
		if err != nil || d == nil {
			restore()
			return func() {}
		}
		raw, has := d.Find("Kids")
		kids, direct := raw.(types.Array)
		if !has {
			continue
		}
		if !direct {
			restore()
			return func() {}
		}
		var refs []types.Object
		var counts []int
		for _, o := range kids {
			if o == nil {
				continue // pdfcpu's walk skips it too
			}
			kr, ok := o.(types.IndirectRef)
			if !ok {
				restore()
				return func() {}
			}
			kd, err := ctx.DereferenceDict(kr)
			if err != nil || kd == nil || kd.Type() == nil {
				restore()
				return func() {}
			}
			switch *kd.Type() {
			case "Page":
				counts = append(counts, 1)
			case "Pages":
				c := kd.IntEntry("Count")
				if c == nil {
					restore()
					return func() {}
				}
				counts = append(counts, *c)
				stack = append(stack, kr)
			default:
				restore()
				return func() {}
			}
			refs = append(refs, kr)
		}
		if len(refs) <= pageNodeFanout {
			continue
		}
		for len(refs) > pageNodeFanout {
			var up []types.Object
			var upCounts []int
			for i := 0; i < len(refs); i += pageNodeFanout {
				j := min(i+pageNodeFanout, len(refs))
				sum := 0
				for _, c := range counts[i:j] {
					sum += c
				}
				ctx.Table[next] = model.NewXRefTableEntryGen0(types.Dict{
					"Type":  types.Name("Pages"),
					"Kids":  append(types.Array(nil), refs[i:j]...),
					"Count": types.Integer(sum),
				})
				temps = append(temps, next)
				up = append(up, *types.NewIndirectRef(next, 0))
				upCounts = append(upCounts, sum)
				next++
			}
			refs, counts = up, upCounts
		}
		undo = append(undo, saved{d, raw})
		d["Kids"] = types.Array(refs)
	}
	return restore
}
