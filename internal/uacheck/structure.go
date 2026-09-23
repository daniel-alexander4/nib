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
		// **A node nib cannot read is a REFUSAL, not an absence**, and the difference decides a verdict
		// rather than a message: with `ptErr` empty, `elementForStructParent` takes its DEFINITE branch
		// and reports "the /StructParent names no element in the parent tree" — a Fail over a tree nib
		// never finished reading. `elementForMCID` separates the two for its own array read, and this
		// walk did not (P05 phase close). An ABSENT key is not a refusal: a `/Nums` node has no `/Kids`
		// and a `/Kids` node has no `/Nums`, so only a present-and-unreadable value is recorded.
		nums, numsErr := d.Ctx.DereferenceArray(node["Nums"])
		if numsErr != nil {
			d.noteParentTreeUnread("/Nums")
		}
		for i := 0; i+1 < len(nums); i += 2 {
			if k, ok := d.intValue(nums[i]); ok {
				d.pt[k] = nums[i+1]
			}
		}
		kids, kidsErr := d.Ctx.DereferenceArray(node["Kids"])
		if kidsErr != nil {
			d.noteParentTreeUnread("/Kids")
		}
		for _, k := range kids {
			walk(k, depth+1)
		}
	}
	walk(root["ParentTree"], 0)
	return d.pt, d.ptErr
}

// noteParentTreeUnread records, first-wins, that part of the parent tree was present and unreadable.
// First-wins because the reason explains a refusal and the FIRST thing nib could not read is the one
// closest to what the caller was looking for; every other bound in this package writes its reason the
// same way.
func (d *Document) noteParentTreeUnread(key string) {
	if d.ptErr == "" {
		d.ptErr = "nib could not read a " + key + " entry of the parent tree, so the keys below it were never read"
	}
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
// verdict no rule may turn into a pass. A name mapped to ITSELF still RESOLVES — a conforming reader
// recognises the type before it consults the map — so it is the second result's exception and leaves the
// corpus unmoved, which is what the ten-hop loop did too.
//
// **It is not, however, a non-cycle, and this comment said it was until `/pending 548`.** Measured:
// `7.1 General/7.1-t06-fail-a.pdf` carries `/RoleMap << /LI /LI >>` and veraPDF FAILS ua1 7.1-6 on its two
// `LI` elements, passing the other twelve. Resolving and being circular are different questions, and since
// P03.S01 they are different walks: `roleMapCircular` has the reason.
//
// **The cycle is not hypothetical**: veraPDF's own corpus carries one, in `7.1 General/7.1-t05-fail-d.pdf`
// (`/Standard → /Text body → /Standard`). nib answered `NotApplicable` and `Pass` over that file's
// elements; it now answers `CannotCheck` for the three rules that ask an element's type, and since
// `/pending 548` it FAILS the clause the cycle actually breaks — **ua1 7.1 t6**, not 7.1 t5; the corpus
// file's name is the specification's test numbering and not veraPDF's, and `rules_structure.go` has the
// measurement. The claim "that is the whole corpus's only cycle" was wrong in the same breath:
// `7.1-t06-fail-a.pdf`'s self-map is a second one, and it is the one this comment's own reasoning missed.
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
		// Reached a name resolved earlier: this path resolves the same way — UNLESS that answer stops at a
		// type already on this path. Then the walk from here would revisit it, which is a cycle, and the
		// remembered answer is the other name's, not this one's (`/TR → /Zed → /TR`: /Zed alone types as
		// /TR, /TR is on a loop). Found by P03.S01's review: the answer depended on which element came first.
		if r, done := d.roles[at]; done && !(r.standard != "" && onPath[r.standard]) {
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
		// **A standard type reached BY MAPPING ends the walk** (P03.S01) — ISO 32000-1 §14.7.3 Note 2, a
		// reader "should follow the chain of associations until it either finds a structure type it
		// recognizes or returns to one it has already encountered". It stopped only at an unmapped name,
		// so `/Alpha → /P → /Zed` typed as `/Zed`; measured on veraPDF 1.30.2, that element is a `/P`.
		// The revisit is asked FIRST, which is veraPDF's order: `/TR → /Zed → /TR` is a cycle (7.1-6 fails
		// on it) even though `/TR` is standard. The START is not stopped at — a standard type the map
		// sends elsewhere types as where it is sent (`/TR → /TD` is a `/TD`), which 7.1 t7 forbids.
		if standardStructureTypes[mapped] && !onPath[mapped] {
			res = roleResolution{standard: mapped}
			break
		}
		at = mapped
	}
	// Every name on the path shares the start's answer, with ONE exception: a loop the walk closed by
	// returning to a STANDARD start. Each later name on it reaches that start by mapping and stops there —
	// so only the start is on a loop, and remembering the loop for the rest would type them wrongly for
	// whichever element came second.
	memo := path
	if res.unresolved != "" && len(path) > 0 && standardStructureTypes[path[0]] {
		memo = path[:1]
	}
	for _, p := range memo {
		d.roles[p] = res
	}
	return res.standard, res.unresolved
}

// roleMapCircular reports whether following the role map from elem's `/S` revisits a name — the fact ua1
// 7.1 t6 is about.
//
// **It is its own walk, and until P03.S01 it was `standardType`'s** (`/pending 548` had derived it from the
// typing walk "rather than from a second one"). The typing walk now STOPS at the first standard type it
// reaches by mapping, and the circularity question does not: measured on veraPDF 1.30.2, `/Alpha → /H1`
// with `/H1 → /H1` FAILS 7.1-6 on `/Alpha` while typing `/Alpha` as `/H1` (7.4.2 t1 passes over it). A
// reader asking "what is this?" stops at what it recognises; a validator asking "does a cycle exist on this
// element's mapping?" follows the map itself. One walk cannot answer both, so each has one.
//
// **A self-map is circular** — `/LI → /LI` (7.1-t06-fail-a) fails 7.1-6 on its two `LI` elements — though
// it types cleanly, which is why this is not derivable from `standardType`'s results.
func (d *Document) roleMapCircular(elem types.Dict) bool {
	name := d.name(elem["S"])
	if name == "" {
		return false
	}
	if c, done := d.circular[name]; done {
		return c
	}
	var roleMap types.Dict
	if root := d.dict(d.Catalog["StructTreeRoot"]); root != nil {
		roleMap = d.dict(root["RoleMap"])
	}
	if d.circular == nil {
		d.circular = map[string]bool{}
	}
	// Every name on one walk's path shares its answer — each reaches the same loop or the same dead end —
	// so the whole path is memoised, as `standardType`'s is, and a 5,000-name chain costs one walk, not
	// 5,000 of them.
	var path []string
	seen := map[string]bool{}
	circular := false
	for at := name; roleMap != nil; {
		if c, done := d.circular[at]; done {
			circular = c
			break
		}
		if seen[at] {
			circular = true
			break
		}
		seen[at] = true
		path = append(path, at)
		next := d.name(roleMap[at])
		if next == "" {
			break
		}
		at = next
	}
	d.circular[name] = circular
	for _, p := range path {
		d.circular[p] = circular
	}
	return circular
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
}

// **A node carries no parent and no kids, on purpose.** The walk reaches a shared element once, under whichever
// parent it met first, so the walk's position is not the element's relation: every relation is read from the
// element's own `/P` and `/K` (`significantParent`, `elementKids`). The two fields that held the walk's position
// had no reader after P03.S02 and were removed at the phase close so no later rule reaches for them.

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
	var walk func(k types.Object, depth int)
	walk = func(k types.Object, depth int) {
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
			el := d.elementKid(en)
			if el == nil {
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
			out = append(out, structNode{dict: el, obj: obj})
			walk(el["K"], depth+1)
		}
	}
	walk(root["K"], 0)
	d.nodes = out
	return d.nodes, d.nodesErr
}
