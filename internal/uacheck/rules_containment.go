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

// elementKids is every structure element an element's OWN `/K` lists, in order — not the tree walk's kids.
//
// **The walk visits an element once, and containment cannot read it** (P03.S02's review, confirmed on
// veraPDF 1.30.2): an element listed in two parents' `/K` sits in the walk under whichever it reached first,
// so the second parent's kids silently lost it — a `/P` shared by a `Div` and a `Table` passed 7.2 t3 in nib
// and failed 7.2-3 in veraPDF. Content (an MCID, a marked-content or object reference) is not a kid.
func (d *Document) elementKids(elem types.Dict) []types.Dict {
	k := elem["K"]
	entries := []types.Object{k}
	if arr, err := d.Ctx.DereferenceArray(k); err == nil && arr != nil {
		entries = arr
	}
	var out []types.Dict
	for _, en := range entries {
		// An MCID is an integer and a well-formed marked-content or object reference carries no /S — but a
		// crafted one can, and veraPDF still reads it as CONTENT: measured, a `<< /Type /MCR /S /Caption >>`
		// between two TRs leaves 7.2-16 passing. So `/Type` is asked as well, exactly as `structNodes` asks it.
		// (P03.S02 dropped this test as dead because no fixture carried the shape; P03.S03's review found it.)
		kid := d.dict(en)
		if kid == nil || d.name(kid["S"]) == "" {
			continue
		}
		if ty := d.name(kid["Type"]); ty == "MCR" || ty == "OBJR" {
			continue
		}
		out = append(out, kid)
	}
	return out
}

// checkContainment evaluates one clause of the matrix — the only function that does.
//
// **The parent is the element's own `/P`, never where the walk found it.** Measured on veraPDF 1.30.2: a
// `TD` listed in a `TR`'s `/K` whose `/P` names the `Document` FAILS 7.2-9, a `TD` with no `/P` fails it,
// and a `TD` listed under the `Document` whose `/P` names a `TR` PASSES. The subjects are still the walk's
// elements — each counted once — and only the relation is read from the objects themselves.
func checkContainment(d *Document, c containment) Result {
	nodes, unread := d.structNodes()
	subjects := 0
	for _, n := range nodes {
		if d.typedAs(n.dict) != c.subject {
			continue
		}
		subjects++
		where := nodeWhere(n, d.name(n.dict["S"]))
		if c.parents != nil {
			parent, in := "", "with no /P naming its parent"
			switch p := d.dict(n.dict["P"]); {
			case p == nil:
			case d.name(p["Type"]) == "StructTreeRoot":
				in = "directly under the structure tree root"
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
		for _, kid := range d.elementKids(n.dict) {
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
