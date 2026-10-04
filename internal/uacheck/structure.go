package uacheck

import (
	"fmt"
	"sort"

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

// maxParentTreeReads bounds the parent-tree nodes every lookup of a document reads between them (the P07 phase-close
// re-review, RR3-1). A variable only so a test can put its ceiling beside a measured count.
//
// A lookup reads each node at most once — a node it has finished searching is not searched again for the same key —
// so its cost is the tree's nodes and /Kids entries, and the document's is that times the keys it looks up. The
// ceiling is for the shape where both are large at once: past it the key being looked up and every later one are
// refused unread, and `reportsNothing` refuses the document, since a lookup nib did not finish may be one veraPDF
// throws in.
var maxParentTreeReads = 1 << 23

// maxParentTreeDepth is how deep a lookup follows /Kids before it stops (RR3-2). **It is not `maxWalkDepth`**: a lookup
// needs no depth bound to end — it never re-enters a node it is inside — and veraPDF has none, reading on until its
// stack runs out. Measured on veraPDF 1.30.2, a chain of 4,000 nodes is read and answered and one of 8,000 overflows
// (StackOverflowError, no report). So nib reads as far as veraPDF certainly does and no further: a lookup past this
// depth refuses its key, and `parentTreeNothing` the document, since veraPDF may have stopped reporting anywhere past
// it. It was 64, and a key seventy levels down was refused where veraPDF answers.
const maxParentTreeDepth = 1000

// ptNode is one parent-tree node as `PDNumberTreeNode` reads it, parsed once per dictionary (`dictID`) however many
// lookups pass through it.
type ptNode struct {
	limited bool // /Limits holds two integers (`parseLimitsArray`), else it is no limit at all
	lo, hi  int64
	hasNums bool                 // the node has a /Nums key, so `getObject` answers from it and never reads /Kids
	nums    map[int]types.Object // `parseNums`: the last pair for a key wins; a non-array /Nums is empty (RR3-3)
	kids    []types.Object       // `parseKids`: a non-array /Kids is no kids (RR3-3)
	// numsUnread and kidsUnread are why a present value could not be read at all — a dereference that failed, not a
	// value of the wrong type, which veraPDF reads as empty and so does nib.
	numsUnread, kidsUnread string
}

// ptAnswer is one key's lookup: the value when found, else why it could not be looked up, if it could not.
type ptAnswer struct {
	entry  types.Object
	found  bool
	unread string
}

// parentTreeEntry is `PDNumberTreeNode.getObject(key)` over the catalog's `/StructTreeRoot /ParentTree`, walking a
// nested number tree (`/Kids`) as well as a flat `/Nums`. It is the ONE door for a parent-tree key (ADR-009); both
// readers of `/StructParent` and `/StructParents` go through it.
//
// The writer (`pdfops.parentTreeDict`) writes a nested number tree only where no rebalancing is needed — a key a node
// already holds, or a new key above every key, raising each `/Limits` on the way down (`PLAN-text-reflow.md` P07.S07) —
// and refuses the rest. A reader cannot refuse: real producers write them, and a checker that could not read one would
// report a correctly tagged document as broken.
//
// **The lookup is veraPDF's, key by key** (RR2-1, measured on veraPDF 1.30.2 over 7.18.1 t1): a depth-first search
// taking the FIRST node holding the key, and within one /Nums array the LAST pair, since that array is read into a map.
// A node with a /Nums key answers from it and its /Kids are never read (measured: a key only below such a node is
// absent), and a node's /Limits, when it holds two integers, excludes every key outside them from that node and all
// below it (measured).
//
// **It is a lookup per key, not one walk of the whole tree** (RR3-1). The walk it replaces answered every key at once
// and so had to carry the key range its ancestors' /Limits left open; it visited a node once per distinct range that
// reached it, and a 75 KB document of shared inline levels made that 12.5 million node reads, 60 s. For ONE key a
// node's /Limits is a yes or a no, so a node's answer does not depend on the path that reached it, and a node already
// searched for this key is not searched again: each lookup reads each node at most once. veraPDF re-searches a node
// reached twice and finds what it found the first time, so the skip changes no answer.
//
// **The third result is why the key could not be looked up** (`/pending 496`): a /Nums or /Kids nib could not read, the
// depth bound, the read budget — or a loop, where veraPDF reports nothing at all (`parentTreeNothing`). A key first
// met AFTER such a part is not answered: under first-found the unread part may hold it, so the value past it need not
// be the one veraPDF takes. Absent with no reason is definite.
func (d *Document) parentTreeEntry(key int) (entry types.Object, found bool, unread string) {
	if a, done := d.ptAns[key]; done {
		return a.entry, a.found, a.unread
	}
	a := d.lookUpParentTree(key)
	if d.ptAns == nil {
		d.ptAns = map[int]ptAnswer{}
	}
	d.ptAns[key] = a
	return a.entry, a.found, a.unread
}

func (d *Document) lookUpParentTree(key int) ptAnswer {
	root := d.dict(d.Catalog["StructTreeRoot"])
	if root == nil {
		return ptAnswer{}
	}
	if d.ptSpent != "" {
		return ptAnswer{unread: d.ptSpent}
	}
	k := int64(key)
	// searched is every node this lookup has finished without finding the key. The path is the nodes the search is
	// inside: by object number, which is what veraPDF's loop check compares, and by dictionary, which is how a node
	// written inline — no object number — is recognised as its own descendant.
	searched := map[uintptr]bool{}
	pathObj := map[int]bool{}
	pathDict := map[uintptr]bool{}
	var stop string
	var search func(o types.Object, depth int) (types.Object, bool)
	search = func(o types.Object, depth int) (types.Object, bool) {
		if d.ptNodes++; d.ptNodes > maxParentTreeReads {
			d.ptSpent = fmt.Sprintf("nib stopped reading the parent tree at its budget of %d node reads, so its keys from "+
				"%d on were never looked up", maxParentTreeReads, key)
			d.noteParentTreeUnknown(d.ptSpent)
			stop = d.ptSpent
			return nil, false
		}
		node := d.dict(o)
		if node == nil {
			return nil, false // `getObject` on a node that is not a dictionary answers null
		}
		id := dictID(node)
		if pathDict[id] {
			// Reached again inside itself, and not through an object number — so no loop check sees it, and veraPDF,
			// having found nothing the first time round, recurses round the same cycle until its stack runs out
			// (measured: StackOverflowError, no report).
			stop = fmt.Sprintf("veraPDF reports nothing on this document — a parent-tree node written inline is its own "+
				"descendant, and veraPDF's lookup of key %d recurses round it without end (StackOverflowError)", key)
			d.noteParentTreeLoop(stop)
			return nil, false
		}
		if searched[id] {
			return nil, false
		}
		if depth > maxParentTreeDepth {
			stop = fmt.Sprintf("the parent tree nests deeper than %d levels on the path to key %d; nib stops reading "+
				"there, so the keys below were never read", maxParentTreeDepth, key)
			d.noteParentTreeUnknown(stop)
			return nil, false
		}
		p := d.ptParse(node)
		if p.limited && (k < p.lo || k > p.hi) {
			return nil, false
		}
		if p.hasNums {
			if p.numsUnread != "" {
				stop = p.numsUnread
				d.noteParentTreeUnknown(stop) // the unread part may hold a loop veraPDF meets (RR round 3)
				return nil, false
			}
			if v, ok := p.nums[key]; ok {
				return v, true
			}
			searched[id] = true
			return nil, false // `getObject` answers from /Nums and never reads this node's /Kids
		}
		if p.kidsUnread != "" {
			stop = p.kidsUnread
			d.noteParentTreeUnknown(stop) // the unread part may hold a loop veraPDF meets (RR round 3)
			return nil, false
		}
		// `parseKids` builds EVERY kid before any is searched, and a kid whose object number is this node's or an
		// ancestor's throws `LoopedException` (measured: a node listing an ancestor AFTER a kid that holds the key still
		// reports nothing). A kid shared by two parents is not a loop — measured, a diamond reports normally.
		nr := 0
		if ir, ok := o.(types.IndirectRef); ok {
			nr = ir.ObjectNumber.Value()
			pathObj[nr] = true
		}
		for _, kid := range p.kids {
			if ir, ok := kid.(types.IndirectRef); ok && pathObj[ir.ObjectNumber.Value()] {
				stop = fmt.Sprintf("veraPDF reports nothing on this document — the parent tree's node (object %d) is "+
					"listed in the /Kids below itself, and veraPDF's lookup of key %d stops there (LoopedException: Loop "+
					"inside number tree)", ir.ObjectNumber.Value(), key)
				d.noteParentTreeLoop(stop)
				return nil, false
			}
		}
		pathDict[id] = true
		for _, kid := range p.kids {
			if v, ok := search(kid, depth+1); ok || stop != "" {
				return v, ok
			}
		}
		delete(pathDict, id)
		if nr != 0 {
			delete(pathObj, nr)
		}
		searched[id] = true
		return nil, false
	}
	v, ok := search(root["ParentTree"], 0)
	if stop != "" {
		return ptAnswer{unread: stop}
	}
	return ptAnswer{entry: v, found: ok}
}

// ptParse reads one node's /Limits, /Nums and /Kids as `PDNumberTreeNode` does, once per dictionary.
func (d *Document) ptParse(node types.Dict) *ptNode {
	id := dictID(node)
	if p, done := d.ptParsed[id]; done {
		return p
	}
	p := &ptNode{}
	if lim, err := d.Ctx.DereferenceArray(node["Limits"]); err == nil && len(lim) >= 2 {
		a, aok := d.intValue(lim[0])
		b, bok := d.intValue(lim[1])
		if aok && bok { // `parseLimitsArray`: two integers, else no limits at all
			p.limited, p.lo, p.hi = true, int64(a), int64(b)
		}
	}
	// **A /Nums or /Kids that is not an array is EMPTY, not unread** (RR3-3, measured on veraPDF 1.30.2: an integer
	// /Kids, a string /Nums, a dictionary /Nums — each is read past, and a string /Nums on the root still stops the
	// search there, since `knownKey(Nums)` holds). Only a value nib could not dereference at all is a refusal.
	if v, has := node["Nums"]; has {
		p.hasNums = true
		o, err := d.Ctx.Dereference(v)
		if err != nil {
			p.numsUnread = "nib could not read a /Nums entry of the parent tree, so the keys below it were never read"
		} else if nums, ok := o.(types.Array); ok {
			p.nums = map[int]types.Object{}
			for i := 0; i+1 < len(nums); i += 2 {
				if k, ok := d.intValue(nums[i]); ok {
					p.nums[k] = nums[i+1] // one node's /Nums is a map: the last pair wins
				}
			}
		}
	} else if v, has := node["Kids"]; has {
		o, err := d.Ctx.Dereference(v)
		if err != nil {
			p.kidsUnread = "nib could not read a /Kids entry of the parent tree, so the keys below it were never read"
		} else if kids, ok := o.(types.Array); ok {
			p.kids = kids
		}
	}
	if d.ptParsed == nil {
		d.ptParsed = map[uintptr]*ptNode{}
	}
	d.ptParsed[id] = p
	return p
}

// noteParentTreeLoop and noteParentTreeUnknown record, first-wins, the two things a lookup can tell `reportsNothing`: a
// loop veraPDF throws on, and a lookup nib stopped short of, past which one might be.
func (d *Document) noteParentTreeLoop(why string) {
	if d.ptLoop == "" {
		d.ptLoop = why
	}
}

func (d *Document) noteParentTreeUnknown(why string) {
	if d.ptUnknown == "" {
		d.ptUnknown = why
	}
}

// parentTreeNothing is `reportsNothing`'s parent-tree half (RR3-2): a document veraPDF reports nothing on because a
// lookup it makes meets a loop, or "" when none does.
//
// **Only a lookup that REACHES the loop throws** (measured on veraPDF 1.30.2): a looping node after the node that holds
// the key, one whose /Limits excludes the key, and a /Nums node listing itself in /Kids all report normally; a loop the
// search reaches first reports nothing, and so does a node written inline that is its own descendant (StackOverflowError).
// So the question is asked of the keys veraPDF looks up — the /StructParent and /StructParents of every indirect object
// in the file, and every holder the annotation-and-field walk reads (`scanAnnotsAndFields`, which asks each through the
// one door, `elementForStructParent`) — a SUPERSET of the holders veraPDF reads: a key no holder veraPDF reads ever asks
// costs a refusal nib need not have made, never an answer veraPDF did not give. A lookup nib stopped short of — past the
// depth bound, the budget, or a /Nums or /Kids it could not dereference — refuses the document too, since the part it
// did not read may hold a loop; and so does a holder population the walk could not finish, since an unread holder's key
// was never asked (the P07 phase-close re-review, round 3).
func (d *Document) parentTreeNothing() string {
	for _, k := range d.structParentKeys() {
		d.parentTreeEntry(k)
		if d.ptLoop != "" {
			return d.ptLoop
		}
	}
	if sc := d.scanAnnotsAndFields(); d.ptLoop != "" {
		return d.ptLoop
	} else if short := firstNonEmpty(sc.annots, sc.fields); short != "" && d.dict(d.Catalog["StructTreeRoot"])["ParentTree"] != nil {
		d.noteParentTreeUnknown("the annotations and form fields were not all read, so their /StructParent keys were " +
			"not all asked: " + short)
	}
	if d.ptUnknown != "" {
		return "nib stopped short of a parent-tree lookup veraPDF makes, so whether veraPDF reports nothing on this " +
			"document — a loop in the part nib did not read — is not known: " + d.ptUnknown
	}
	return ""
}

// structParentKeys is every integer /StructParent and /StructParents on an indirect object in the file, ascending.
// Holders written inline — an annotation in a page's /Annots, a field in /Fields or /Kids — are asked by
// `parentTreeNothing` through the annotation-and-field walk, which enumerates them and reports where it stopped short.
func (d *Document) structParentKeys() []int {
	seen := map[int]bool{}
	var keys []int
	add := func(dict types.Dict) {
		for _, name := range []string{"StructParent", "StructParents"} {
			if k, ok := d.intValue(dict[name]); ok && !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	for nr := range d.Ctx.XRefTable.Table {
		o, err := d.Ctx.Dereference(types.IndirectRef{ObjectNumber: types.Integer(nr)})
		if err != nil {
			continue
		}
		switch v := o.(type) {
		case types.Dict:
			add(v)
		case types.StreamDict:
			add(v.Dict)
		}
	}
	sort.Ints(keys)
	return keys
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
// It is the shape `structNodes` and `parentTreeEntry` already have — one call, two results, one guard at the
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

// maxStructEntries bounds the `/K` entries `structNodes` reads, repeats included (R3-8). A variable only so a
// test can put a control just under it without writing a file of four million entries; nothing else sets it.
// Measured before it existed: pdfcpu's own open costs about ten times the walk over the same shared arrays
// (L=18: open 810 ms, the walk 79 ms over 393,214 elements), so this binds only past what open already paid for.
var maxStructEntries = 1 << 22

// structNodes walks every structure element reachable from the root, a parent before its kids.
//
// Marked-content ids, marked-content references and object references are skipped: they are content,
// not elements. A dictionary with no `/S` is not an element either. The walk is bounded in depth; an
// indirect element reached twice is visited once, and an inline one is visited at each reach, under a bound on the
// entries read (the comment on `seen` below has why).
//
// **The second result is why part of the tree was not read** (`/pending 496`), and every rule reading the
// nodes answers `CannotCheck` when it is non-empty. The bound used to return silently, so a heading that
// skipped a level under seventy `Div`s passed 7.4.2 t1: the elements past it read as elements that are not
// there. It trips only on an ELEMENT past the bound — a leaf's MCID kid at that depth is not unread structure.
func (d *Document) structNodes() ([]structNode, string) {
	if built, why := d.nodesBuild.again(d.nodesErr, "the structure walk"); built {
		return d.nodes, why
	}
	root := d.dict(d.Catalog["StructTreeRoot"])
	if root == nil {
		d.nodesBuild.finish()
		return nil, ""
	}
	var out []structNode
	seen := map[int]bool{}
	entriesRead := 0
	var walk func(k types.Object, depth int)
	walk = func(k types.Object, depth int) {
		if k == nil || entriesRead > maxStructEntries {
			return
		}
		entries := []types.Object{k}
		if arr, err := d.Ctx.DereferenceArray(k); err == nil && arr != nil {
			entries = arr
		}
		for _, en := range entries {
			// **Every `/K` entry read is charged** (the P07 phase-close review, R3-8). An element written inline in
			// a `/K` array has no object number for `seen` to hold, so an indirect ARRAY two parents share is
			// walked once per parent, and L levels of it walk 2^L elements. It is NOT deduplicated: an inline
			// element reached twice is visited twice — veraPDF gives a key-less object a fresh identity at each
			// reach — and collapsing the repeats could move a rule that counts them. It is bounded instead, and
			// past the ceiling every rule reading the nodes refuses.
			if entriesRead++; entriesRead > maxStructEntries {
				if d.nodesErr == "" {
					d.nodesErr = fmt.Sprintf("the structure tree reaches more than %d /K entries (an array shared by "+
						"many parents is read once per parent); nib stops reading there, so the elements beyond were "+
						"never read", maxStructEntries)
				}
				return
			}
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
	d.nodeEntries = entriesRead
	d.nodes = out
	d.nodesBuild.finish()
	return d.nodes, d.nodesErr
}

// firstNonEmpty is the first of its arguments that is not "".
func firstNonEmpty(ss ...string) string {
	for _, v := range ss {
		if v != "" {
			return v
		}
	}
	return ""
}
