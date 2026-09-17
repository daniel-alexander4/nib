package uacheck

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Reading the structure tree — `PLAN-accessibility.md` P07.S03.
//
// # Why the checker walks the tree itself instead of borrowing `pdfops`' model
//
// `internal/pdfops` has a typed tree model (`readStructTree`, `parentTreeEntries`) and it is the
// model every tagging door WRITES through. A checker that read documents with the writer's own model
// would agree with the writer by construction: a defect in how the model reads a tree would be a
// defect the checker shares, and the two would certify each other. `pdfops`' own test suite already
// knows this — `countElementsIndependently` exists "so the model's count is compared against
// something that does not share its code".
//
// So this is a second, deliberately independent reading, and law 5 (veraPDF over the corpus) is what
// validates it. That is not ADR-009's two-opinions defect: the rule "how to build a tree" lives in
// `pdfops`, and the rule "what a conforming tree is" lives here — two different rules, each with one
// door.

// maxWalkDepth bounds every recursive read, because a malicious or broken document can make a
// structure tree, a number tree or a form XObject refer to itself.
const maxWalkDepth = 64

// parentTree resolves the catalog's `/StructTreeRoot /ParentTree` into key → value, walking a nested
// number tree (`/Kids`) as well as a flat `/Nums`.
//
// The writer (`pdfops.setParentTreeSlot`) REFUSES nested number trees rather than rebalancing them.
// A reader cannot refuse: real producers write them, and a checker that could not read one would
// report a correctly tagged document as broken.
//
// **The second result is why part of the tree was not read** (`/pending 496`). A key the map lacks may sit
// below the depth bound, so a caller whose key is missing answers `CannotCheck` when it is non-empty —
// until then a widget keyed seventy levels down failed 7.18.4 t1 as though it had no element at all.
func (d *Document) parentTree() (map[int]types.Object, string) {
	if d.pt != nil {
		return d.pt, d.ptErr
	}
	d.pt = map[int]types.Object{}
	root := d.dict(d.Catalog["StructTreeRoot"])
	if root == nil {
		return d.pt, ""
	}
	seen := map[int]bool{}
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if ir, ok := o.(types.IndirectRef); ok {
			if seen[ir.ObjectNumber.Value()] {
				return
			}
			seen[ir.ObjectNumber.Value()] = true
		}
		node := d.dict(o)
		if node == nil {
			return
		}
		if depth > maxWalkDepth {
			if d.ptErr == "" {
				d.ptErr = fmt.Sprintf("the parent tree nests deeper than %d levels; nib stops reading there, so the "+
					"keys below were never read", maxWalkDepth)
			}
			return
		}
		if nums, err := d.Ctx.DereferenceArray(node["Nums"]); err == nil {
			for i := 0; i+1 < len(nums); i += 2 {
				if k, ok := d.intValue(nums[i]); ok {
					d.pt[k] = nums[i+1]
				}
			}
		}
		if kids, err := d.Ctx.DereferenceArray(node["Kids"]); err == nil {
			for _, k := range kids {
				walk(k, depth+1)
			}
		}
	}
	walk(root["ParentTree"], 0)
	return d.pt, d.ptErr
}

// standardType resolves an element's `/S` through the tree's `/RoleMap` to a standard structure
// type, and says why it could not when it could not (`/pending 507`).
//
// A producer may call a form element `/MyFormField` and map it to `/Form`; veraPDF resolves the map
// and so must this, or a correctly tagged form from another producer fails 7.18.4.
//
// # Why the chain is followed to its end and not for ten hops
//
// It used to stop after ten hops and **return the intermediate name**, which read exactly like an
// element of that type. Measured against veraPDF 1.30.2 on a one-heading document whose `/S` is
// `/T0` and whose role map chains `T0 → … → T29 → H3`: veraPDF resolves all thirty hops and FAILS
// 7.4.2 t1 (a document's first numbered heading is H1), while nib answered `NotApplicable` — "the
// structure tree has no numbered heading" — because the tenth name was `T10` and `T10` is not a
// heading. The same chain ending `H1` veraPDF passes and nib also called not applicable, so the bound
// lost a real failure AND a real pass. A hop bound reported as `CannotCheck` would have kept the
// second loss: veraPDF settles these, so a checker that cannot is a checker with less reach.
//
// A role map is a finite dictionary, so following it terminates: every step either stops or reaches a
// name not yet on the path. **Only a CYCLE is unresolvable**, and that is the second result — a
// verdict no rule may turn into a pass. A name mapped to ITSELF is a fixed point, not a cycle: it
// resolves to itself, which is what the ten-hop loop did and what leaves the corpus unmoved.
//
// **The cycle is not hypothetical**: veraPDF's own corpus carries one, in `7.1 General/7.1-t05-fail-d.pdf`
// (`/Standard → /Text body → /Standard`), which is the file written to fail ua1 7.1 t5 — *"RoleMap shall
// not contain a circular mapping"*. nib does not implement 7.1 t5, and it answered `NotApplicable` and
// `Pass` over that file's elements; it now answers `CannotCheck` for the three rules that ask an element's
// type. That is the whole corpus's only cycle: 0 false pass and 0 false fail either way, and `corpusReach`
// unmoved, because those three pairs were never settled.
//
// Measured cost with the per-document memo below, on a 5,000-entry role map forming one chain with
// 5,000 elements each starting at a different point in it — the worst shape there is: 1.9 ms for all
// 5,000 resolutions, against 1.8 ms for the ten-hop bound that resolved none of them.
func (d *Document) standardType(elem types.Dict) (standard string, unresolved string) {
	name := d.name(elem["S"])
	if name == "" {
		return "", ""
	}
	if d.roles == nil {
		d.roles = map[string]roleResolution{}
	}
	if r, done := d.roles[name]; done {
		return r.standard, r.unresolved
	}
	var roleMap types.Dict
	if root := d.dict(d.Catalog["StructTreeRoot"]); root != nil {
		roleMap = d.dict(root["RoleMap"])
	}
	var path []string
	onPath := map[string]bool{}
	var res roleResolution
	for at := name; ; {
		if r, done := d.roles[at]; done {
			// Reached a name resolved earlier: everything on this path resolves the same way.
			res = r
			break
		}
		if onPath[at] {
			res = roleResolution{unresolved: fmt.Sprintf("the role map sends /%s back to /%s, a loop with no standard "+
				"structure type at the end of it, so nib cannot say what type this element has", path[len(path)-1], at)}
			break
		}
		onPath[at] = true
		path = append(path, at)
		mapped := ""
		if roleMap != nil {
			mapped = d.name(roleMap[at])
		}
		if mapped == "" || mapped == at {
			res = roleResolution{standard: at}
			break
		}
		at = mapped
	}
	for _, p := range path {
		d.roles[p] = res
	}
	return res.standard, res.unresolved
}

// standardTypes resolves every node's standard type in one call, indexed like nodes, with the first
// element whose role map could not be followed as the second result.
//
// It is the shape `structNodes` and `parentTree` already have — one call, two results, one guard at the
// top of the rule — so a rule that walks the tree cannot answer over an element it could not type.
func (d *Document) standardTypes(nodes []structNode) ([]string, string) {
	out := make([]string, len(nodes))
	unresolved := ""
	for i, n := range nodes {
		std, why := d.standardType(n.dict)
		out[i] = std
		if why != "" && unresolved == "" {
			unresolved = why
		}
	}
	return out, unresolved
}

// declaresLangFor reports whether elem or any ancestor declares a `/Lang`, walking `/P` up to the
// structure tree root — present counts, even empty (`declaresLang` says why).
//
// Language inherits down the tree (ISO 32000-1 §14.9.2), which is what lets one `/Lang` on a
// `Document` element cover every paragraph beneath it.
//
// **The second result is why the climb stopped short of the root**, when it did (`/pending 496`): an
// ancestor past the bound was never read, which is not the same as no ancestor declaring a language.
func (d *Document) declaresLangFor(elem types.Dict) (bool, string) {
	for depth := 0; elem != nil; depth++ {
		if d.name(elem["Type"]) == "StructTreeRoot" {
			return false, ""
		}
		if depth >= maxWalkDepth {
			return false, fmt.Sprintf("the element describing this text sits deeper than %d levels below the structure "+
				"root; nib stops climbing there, so an ancestor's /Lang was never read", maxWalkDepth)
		}
		if d.declaresLang(elem["Lang"]) {
			return true, ""
		}
		elem = d.dict(elem["P"])
	}
	return false, ""
}

// resourcesOf returns a page's `/Resources`, climbing `/Parent` for the inherited case.
//
// Resources are inheritable through the page tree, and a document whose pages share one resource
// dictionary on their `/Pages` node is ordinary. Without the climb every `Do` and every named
// property list on such a page would look unresolvable.
func (d *Document) resourcesOf(page types.Dict) types.Dict {
	for depth := 0; page != nil && depth < maxWalkDepth; depth++ {
		if r := d.dict(page["Resources"]); r != nil {
			return r
		}
		page = d.dict(page["Parent"])
	}
	return nil
}

// structNode is one structure element reached from the root — `PLAN-accessibility.md` P09.S05.
type structNode struct {
	dict types.Dict
	// obj is the element's object number, or 0 for one written inline in its parent's `/K`.
	obj int
	// parent is the index of the node holding this one, or -1 for one directly under the root.
	parent int
	// kids are the indices of its element kids, in `/K` order.
	kids []int
}

// structNodes walks every structure element reachable from the root, a parent before its kids.
//
// Marked-content ids, marked-content references and object references are skipped: they are content,
// not elements. A dictionary with no `/S` is not an element either. The walk is bounded in depth and an
// element reached twice is visited once, for the reason `parentTree` gives.
//
// **The second result is why part of the tree was not read** (`/pending 496`), and every rule reading the
// nodes answers `CannotCheck` when it is non-empty. The bound used to return silently, so a heading that
// skipped a level under seventy `Div`s passed 7.4.2 t1: the elements past it read as elements that are not
// there. It trips only on an ELEMENT past the bound — a leaf's MCID kid at that depth is not unread structure.
func (d *Document) structNodes() ([]structNode, string) {
	if d.nodesDone {
		return d.nodes, d.nodesErr
	}
	d.nodesDone = true
	root := d.dict(d.Catalog["StructTreeRoot"])
	if root == nil {
		return nil, ""
	}
	var out []structNode
	seen := map[int]bool{}
	var walk func(k types.Object, parent, depth int)
	walk = func(k types.Object, parent, depth int) {
		if k == nil {
			return
		}
		entries := []types.Object{k}
		if arr, err := d.Ctx.DereferenceArray(k); err == nil && arr != nil {
			entries = arr
		}
		for _, en := range entries {
			obj := 0
			if ir, ok := en.(types.IndirectRef); ok {
				obj = ir.ObjectNumber.Value()
				if seen[obj] {
					continue
				}
			}
			el := d.dict(en)
			if el == nil || d.name(el["S"]) == "" {
				continue
			}
			if ty := d.name(el["Type"]); ty == "MCR" || ty == "OBJR" {
				continue
			}
			if depth > maxWalkDepth {
				if d.nodesErr == "" {
					d.nodesErr = fmt.Sprintf("the structure tree nests deeper than %d levels; nib stops reading there, "+
						"so the elements below were never read", maxWalkDepth)
				}
				return
			}
			if obj != 0 {
				seen[obj] = true
			}
			i := len(out)
			out = append(out, structNode{dict: el, obj: obj, parent: parent})
			if parent >= 0 {
				out[parent].kids = append(out[parent].kids, i)
			}
			walk(el["K"], i, depth+1)
		}
	}
	walk(root["K"], -1, 0)
	d.nodes = out
	return d.nodes, d.nodesErr
}

// tableAttribute returns key from elem's Table attribute object. `/A` may be one attribute object or
// an array of them with revision numbers between; only an object owned by `/O /Table` answers.
func (d *Document) tableAttribute(elem types.Dict, key string) types.Object {
	if elem["A"] == nil {
		return nil
	}
	o, err := d.Ctx.Dereference(elem["A"])
	if err != nil || o == nil {
		return nil
	}
	objs := []types.Object{o}
	if arr, ok := o.(types.Array); ok {
		objs = arr
	}
	for _, x := range objs {
		ad := d.dict(x)
		if ad == nil {
			continue
		}
		if d.name(ad["O"]) != "Table" {
			continue
		}
		if v, ok := ad[key]; ok {
			return v
		}
	}
	return nil
}
