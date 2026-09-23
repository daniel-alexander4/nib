package uacheck

import (
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The containment matrix — `PLAN-ua-coverage.md` P03.S02.
//
// veraPDF writes seventeen of ua1 7.2's clauses as one of two predicates over RESOLVED role names — the
// parent's standard type is one of a set (`parentStandardType == 'TR'`), or every kid's is
// (`kidsStandardTypes.split('&').filter(...)`). Tables, lists and the table of contents differ only in which
// names fill them. So the relation is written once, as data, and one door evaluates it (ADR-009); each clause
// keeps its own number, summary and `corpusReach` row, but none keeps its own code.
// `TestContainmentIsWrittenOnlyInTheMatrix` is the guard that no clause escapes into a rule of its own.
//
// # What was measured, on veraPDF 1.30.2, before a line was written
//
// Thirty hand-built trees (`treeDoc`), each clause in both directions, and these edges:
//
//   - **An untyped kid is ignored**, not refused: a kid on a role-map loop, one dead-ending at a private
//     name, and one mapped to itself all leave `Table`'s 7.2-3 passing. veraPDF has no standard type for
//     it, so `kidsStandardTypes` never holds it.
//   - **An untyped PARENT satisfies nothing**: a `TD` under an element on a loop fails 7.2-9, and so does a
//     `TR` directly under the structure tree root (7.2-4).
//   - **Role-mapped names resolve first**: `/Z → /TR` as a `Table`'s kid, and `/W → /Table` as a `TR`'s
//     parent, both pass.
//   - **Content is not a kid**: an MCID or an object reference in a `Table`'s `/K` does not fail 7.2-3.
//   - **An element with no element kids passes every "may contain only" clause** (`kidsStandardTypes == ''`).
//
// The subject is every element of the clause's type; a document with none is `NotApplicable`, which is
// veraPDF's 0-check answer.

// containment is one clause of the matrix. Exactly one of parents and kids is set.
type containment struct {
	clause  string
	subject string   // the standard type the clause is about
	parents []string // the subject's parent must resolve to one of these
	kids    []string // every kid that resolves to a standard type must be one of these
}

// containmentMatrix is the only place a containment relation is written.
var containmentMatrix = []containment{
	{clause: "7.2 t3", subject: "Table", kids: []string{"TR", "THead", "TBody", "TFoot", "Caption"}},
	{clause: "7.2 t4", subject: "TR", parents: []string{"Table", "THead", "TBody", "TFoot"}},
	{clause: "7.2 t5", subject: "THead", parents: []string{"Table"}},
	{clause: "7.2 t6", subject: "TBody", parents: []string{"Table"}},
	{clause: "7.2 t7", subject: "TFoot", parents: []string{"Table"}},
	{clause: "7.2 t8", subject: "TH", parents: []string{"TR"}},
	{clause: "7.2 t9", subject: "TD", parents: []string{"TR"}},
	{clause: "7.2 t10", subject: "TR", kids: []string{"TH", "TD"}},
	{clause: "7.2 t17", subject: "LI", parents: []string{"L"}},
	{clause: "7.2 t18", subject: "LBody", parents: []string{"LI"}},
	{clause: "7.2 t19", subject: "L", kids: []string{"L", "LI", "Caption"}},
	{clause: "7.2 t20", subject: "LI", kids: []string{"Lbl", "LBody"}},
	{clause: "7.2 t26", subject: "TOCI", parents: []string{"TOC"}},
	{clause: "7.2 t27", subject: "TOC", kids: []string{"TOC", "TOCI", "Caption"}},
	{clause: "7.2 t36", subject: "THead", kids: []string{"TR"}},
	{clause: "7.2 t37", subject: "TBody", kids: []string{"TR"}},
	{clause: "7.2 t38", subject: "TFoot", kids: []string{"TR"}},
}

func init() {
	for _, c := range containmentMatrix {
		c := c
		register(Rule{Clause: c.clause, Summary: c.summary(), Check: func(d *Document) Result { return checkContainment(d, c) }})
	}
}

// summary is the clause's one line, generated from the relation so the words cannot drift from it.
func (c containment) summary() string {
	if c.parents != nil {
		return fmt.Sprintf("%s %s element shall be contained in %s", article(c.subject), c.subject, oneOf(c.parents))
	}
	return fmt.Sprintf("%s %s element may contain only %s elements", article(c.subject), c.subject, oneOf(c.kids))
}

// article is the indefinite article a type name takes read aloud: "an L", "an LI", "a TR".
func article(name string) string {
	if strings.HasPrefix(name, "L") {
		return "an"
	}
	return "a"
}

// oneOf is "A", "A or B", "A, B or C".
func oneOf(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// typedAs is an element's standard structure type, or "" when it has none — a role-map loop, or a chain
// that ends at a name ISO 32000-1 §14.8.4 does not define. veraPDF gives such an element no standard type,
// and the matrix reads it the same way.
func (d *Document) typedAs(elem types.Dict) string {
	std, unresolved := d.standardType(elem)
	if unresolved != "" || !standardStructureTypes[std] {
		return ""
	}
	return std
}

// passThrough are the standard types veraPDF looks THROUGH when it asks an element's kids or its parent:
// `NonStruct`, `Div` and `Part` (veraPDF-parser `PDStructElem.isPassThroughTag`). A `Table` whose rows sit in a
// `Div` is a table of those rows, and a `TR` inside that `Div` has the `Table` as its parent.
//
// **Measured on veraPDF 1.30.2, and shipped wrong in P03.S02 and S03** — found while reading veraPDF's own
// table code for P03.S04: `Table(Div(TR(TD)))` passes 7.2-3 and 7.2-4 in veraPDF and failed both in nib, and a
// second `THead` hidden in a `Div` FAILS 7.2-11 in veraPDF and passed in nib. The corpus holds none of these
// shapes, so it scored 0 disagreements over code that was wrong on all of them.
var passThrough = map[string]bool{"NonStruct": true, "Div": true, "Part": true}

// elementKids is every structure element an element's OWN `/K` lists, in order — not the tree walk's kids —
// with a pass-through kid replaced by ITS kids, recursively (veraPDF's structurally significant children).
//
// **The walk visits an element once, and containment cannot read it** (P03.S02's review, confirmed on
// veraPDF 1.30.2): an element listed in two parents' `/K` sits in the walk under whichever it reached first,
// so the second parent's kids silently lost it — a `/P` shared by a `Div` and a `Table` passed 7.2 t3 in nib
// and failed 7.2-3 in veraPDF. Content (an MCID, a marked-content or object reference) is not a kid.
//
// **The second result is why the kids could not be listed, and a caller answers CannotCheck on it** (P03's
// phase-close review). A pass-through element listed twice is expanded twice — veraPDF counts its kids twice,
// so the duplicates are kept — and a chain of them doubles at every level: twenty shared `Div`s took 1.6 s and
// a `Div` listing itself twice never finished, inside a rule with no deadline. So the expansion is memoised
// per dictionary, a pass-through element that contains itself is refused, and the whole document shares one
// budget (`maxKidExpansion`) — spent on every entry read and every kid a list gains, so an exponential
// list runs it out rather than the process.
func (d *Document) elementKids(elem types.Dict) ([]types.Dict, string) {
	kids, why, _, _ := d.kidsOf(elem, map[uintptr]bool{}, 0)
	return kids, why
}

// maxKidExpansion is how many `/K` entries every elementKids walk in one document may read, and kids its lists
// may hold, between them. A real tree spends about one per element (measured at P03's phase close: 12,710 for a
// 12,711-node tree, the largest of sixty local files over 2 MB); the budget is spent only by sharing.
const maxKidExpansion = 1 << 22

// kidsResult is one memoised elementKids answer, with the height of the pass-through chain below the element:
// a memo hit met deeper than `maxWalkDepth - height` is the depth refusal a fresh walk would have given.
type kidsResult struct {
	kids   []types.Dict
	why    string
	height int
}

// kidsOf answers elementKids, the height of the pass-through chain below elem, and whether the answer may be
// memoised. **The answer must not depend on which element was asked first** (the fix pass's re-review): a depth
// refusal belongs to the walk that met the element, not to the element — memoising it made a Div whose own chain
// was 31 deep answer "deeper than 64" after a walk from 40 levels above had met it — and a memoised success must
// not let a deeper walk skip the bound either. So a depth refusal is never memoised, and a memo hit carries its
// height and is refused when the walk meets it too deep for that height. A cycle is on the element wherever the
// walk starts, and a spent budget stays spent, so both are memoised. Every entry read is charged to the budget,
// so an answer that cannot be memoised is still paid for each time it is recomputed.
func (d *Document) kidsOf(elem types.Dict, onPath map[uintptr]bool, depth int) ([]types.Dict, string, int, bool) {
	tooDeep := fmt.Sprintf("pass-through elements (NonStruct, Div, Part) nest deeper than %d levels; nib stops "+
		"reading there, so the elements below were never read", maxWalkDepth)
	id := dictID(elem)
	if r, ok := d.kids[id]; ok {
		if r.why == "" && depth+r.height > maxWalkDepth {
			return nil, tooDeep, 0, false
		}
		return r.kids, r.why, r.height, true
	}
	if onPath[id] {
		return nil, fmt.Sprintf("a /%s element lists itself among its own kids, so its kids cannot be listed", d.name(elem["S"])), 0, true
	}
	if depth > maxWalkDepth {
		return nil, tooDeep, 0, false
	}
	onPath[id] = true
	defer delete(onPath, id)
	if d.kids == nil {
		d.kids = map[uintptr]kidsResult{}
		d.kidBudget = maxKidExpansion
	}
	var out []types.Dict
	why, cacheable, height := "", true, 0
	spend := func(n int) bool {
		if d.kidBudget -= n; d.kidBudget >= 0 {
			return true
		}
		why = fmt.Sprintf("the structure tree's pass-through elements (NonStruct, Div, Part) expand past %d kids "+
			"between them; nib stops reading there, so the elements beyond were never read", maxKidExpansion)
		return false
	}
	k := elem["K"]
	entries := []types.Object{k}
	if arr, err := d.Ctx.DereferenceArray(k); err == nil && arr != nil {
		entries = arr
	}
	for _, en := range entries {
		if !spend(1) {
			break
		}
		// An MCID is an integer and a well-formed marked-content or object reference carries no /S — but a
		// crafted one can, and veraPDF still reads it as CONTENT: measured, a `<< /Type /MCR /S /Caption >>`
		// between two TRs leaves 7.2-16 passing. So `/Type` is asked as well, through the same predicate
		// `structNodes` asks. (P03.S02 dropped this test as dead because no fixture carried the shape; P03.S03's
		// review found it.)
		kid := d.elementKid(en)
		if kid == nil {
			continue
		}
		if !passThrough[d.typedAs(kid)] {
			out = append(out, kid)
			continue
		}
		inner, innerWhy, innerHeight, innerCacheable := d.kidsOf(kid, onPath, depth+1)
		if innerWhy != "" {
			why, cacheable = innerWhy, innerCacheable
			break
		}
		if innerHeight+1 > height {
			height = innerHeight + 1
		}
		if !spend(len(inner)) {
			break
		}
		out = append(out, inner...)
	}
	if why != "" {
		out = nil
	}
	if cacheable {
		d.kids[id] = kidsResult{kids: out, why: why, height: height}
	}
	return out, why, height, cacheable
}

// elementKid is the structure element a `/K` entry names, or nil when the entry is content — an MCID, a
// marked-content or object reference — or no element at all. It is the one answer to "is this kid an
// element", read by the tree walk (`structNodes`) and by every relation (`elementKids`), which had drifted
// apart once already (the `/Type` test above).
func (d *Document) elementKid(en types.Object) types.Dict {
	kid := d.dict(en)
	if kid == nil || d.name(kid["S"]) == "" {
		return nil
	}
	if ty := d.name(kid["Type"]); ty == "MCR" || ty == "OBJR" {
		return nil
	}
	return kid
}

// significantParent is the element's parent as veraPDF reads it: its own `/P`, climbed past every pass-through
// ancestor. It answers the parent's dictionary and whether the climb reached the structure tree root — nil and
// false when a `/P` is missing on the way — and, third, why the climb could not finish: a `/P` loop or a chain
// past the depth bound. That is not a missing `/P`, and a caller answers CannotCheck on it, never Fail with a
// reason that is not true (P03's phase-close review: a `/P` loop between two `Div`s read as "no /P").
func (d *Document) significantParent(elem types.Dict) (types.Dict, bool, string) {
	seen := map[uintptr]bool{}
	for p := d.dict(elem["P"]); p != nil; p = d.dict(p["P"]) {
		if d.name(p["Type"]) == "StructTreeRoot" {
			return nil, true, ""
		}
		if !passThrough[d.typedAs(p)] {
			return p, false, ""
		}
		if id := dictID(p); seen[id] {
			return nil, false, "the /P entries of its pass-through ancestors (NonStruct, Div, Part) form a loop, so its parent cannot be read"
		} else if len(seen) >= maxWalkDepth {
			return nil, false, fmt.Sprintf("its /P chain climbs through more than %d pass-through ancestors; nib stops reading there", maxWalkDepth)
		} else {
			seen[id] = true
		}
	}
	return nil, false, ""
}

// checkContainment evaluates one clause of the matrix — the only function that does.
//
// **The parent is the element's own `/P`, never where the walk found it** — climbed past `NonStruct`, `Div` and
// `Part`, which veraPDF looks through (`passThrough`). Measured on veraPDF 1.30.2: a
// `TD` listed in a `TR`'s `/K` whose `/P` names the `Document` FAILS 7.2-9, a `TD` with no `/P` fails it,
// and a `TD` listed under the `Document` whose `/P` names a `TR` PASSES. The subjects are still the walk's
// elements — each counted once — and only the relation is read from the objects themselves.
func checkContainment(d *Document, c containment) Result {
	nodes, unread := d.structNodes()
	subjects := 0
	cannot := ""
	for _, n := range nodes {
		if d.typedAs(n.dict) != c.subject {
			continue
		}
		subjects++
		where := nodeWhere(n, d.name(n.dict["S"]))
		if c.parents != nil {
			parent, in := "", "with no /P naming its parent"
			p, atRoot, why := d.significantParent(n.dict)
			if why != "" {
				if cannot == "" {
					cannot = fmt.Sprintf("%s: %s", where, why)
				}
				continue
			}
			switch {
			case atRoot:
				in = "directly under the structure tree root"
			case p == nil:
			default:
				parent = d.typedAs(p)
				in = fmt.Sprintf("in a /%s element, whose type does not resolve to a standard one", d.name(p["S"]))
				if parent != "" {
					in = "in " + article(parent) + " /" + parent + " element"
				}
			}
			if !contains(c.parents, parent) {
				return Result{Verdict: Fail, Where: where,
					Why: fmt.Sprintf("%s /%s element sits %s; it may sit only in %s", article(c.subject), c.subject, in, oneOf(c.parents))}
			}
			continue
		}
		kids, why := d.elementKids(n.dict)
		if why != "" {
			if cannot == "" {
				cannot = fmt.Sprintf("%s: %s", where, why)
			}
			continue
		}
		for _, kid := range kids {
			kt := d.typedAs(kid)
			if kt == "" || contains(c.kids, kt) {
				continue
			}
			return Result{Verdict: Fail, Where: where,
				Why: fmt.Sprintf("%s /%s element contains %s /%s element; it may contain only %s", article(c.subject), c.subject, article(kt), kt, oneOf(c.kids))}
		}
	}
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	if cannot != "" {
		return Result{Verdict: CannotCheck, Why: cannot}
	}
	if subjects == 0 {
		return Result{Verdict: NotApplicable, Why: fmt.Sprintf("the document has no /%s element", c.subject)}
	}
	return Result{Verdict: Pass}
}

func contains(set []string, s string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}
