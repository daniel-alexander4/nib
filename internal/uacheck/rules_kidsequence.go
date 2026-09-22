package uacheck

import "fmt"

// Cardinality and placement — `PLAN-ua-coverage.md` P03.S03.
//
// Eight of ua1 7.2's clauses are predicates over ONE fact: the sequence of an element's kids' standard
// types. veraPDF writes them over `kidsStandardTypes` joined with `&` — counts (`filter(e => e == 'THead')
// .length <= 1`) and positions (`indexOf('&Caption&') < 0`). The containment matrix asks whether each kid is
// allowed; these ask how many and where, which the matrix deliberately does not express. So they are a second
// table with its own one door, reading the SAME kid list the matrix reads (`elementKids`, `typedAs`).
//
// # What was measured, on veraPDF 1.30.2, before a line was written
//
// Eighteen `treeDoc` trees, each clause in both directions — four of the eight (`t16`, `t28`, `t39`, `t40`)
// have no corpus file at all, so these fixtures are their only oracle. The one edge the joined string leaves
// open, settled: **an untyped kid is DROPPED from the sequence, never an empty slot in it.** A Caption after
// an untyped first kid is still first (7.2-40 and 7.2-28 pass), and one before an untyped last kid is still
// last (7.2-16 passes). And a second leading Caption is in the middle — `Caption&Caption&TR` fails 7.2-16 as
// well as 7.2-39.

// kidSequence is one clause over an element's typed kids.
type kidSequence struct {
	clause  string
	subject string // the standard type the clause is about, or "" for every structure element
	summary string
	// broken says why kids breaks the clause, or "" when it holds.
	broken func(kids []string) string
}

// kidSequenceRules is the only place a cardinality or placement clause is written.
var kidSequenceRules = []kidSequence{
	{clause: "7.2 t11", subject: "Table", summary: "a Table element shall contain at most one THead", broken: atMostOne("THead")},
	{clause: "7.2 t12", subject: "Table", summary: "a Table element shall contain at most one TFoot", broken: atMostOne("TFoot")},
	{clause: "7.2 t13", subject: "Table", summary: "a Table element with a TFoot shall contain a TBody", broken: needsIfPresent("TFoot", "TBody")},
	{clause: "7.2 t14", subject: "Table", summary: "a Table element with a THead shall contain a TBody", broken: needsIfPresent("THead", "TBody")},
	{clause: "7.2 t16", subject: "Table", summary: "a Table element's Caption shall be its first or last kid", broken: captionAtEnds},
	{clause: "7.2 t39", subject: "Table", summary: "a Table element shall contain at most one Caption", broken: atMostOne("Caption")},
	{clause: "7.2 t28", subject: "TOC", summary: "a TOC element's Caption shall be its first kid", broken: captionFirstOnly},
	{clause: "7.2 t40", subject: "L", summary: "an L element's Caption shall be its first kid", broken: captionFirstOnly},
	// P03.S05. veraPDF's object is every PDStructElem, so the subject is every element ("") — measured: two H
	// kids fail, one of them behind a Div fails too (the kid list looks through pass-through tags), and an H in
	// each of two Sects passes.
	{clause: "7.4.4 t1", subject: "", summary: "each structure element shall contain at most one H kid", broken: atMostOne("H")},
}

func init() {
	for _, r := range kidSequenceRules {
		r := r
		register(Rule{Clause: r.clause, Summary: r.summary, Check: func(d *Document) Result { return checkKidSequence(d, r) }})
	}
}

func count(kids []string, name string) int {
	n := 0
	for _, k := range kids {
		if k == name {
			n++
		}
	}
	return n
}

func atMostOne(name string) func([]string) string {
	return func(kids []string) string {
		if n := count(kids, name); n > 1 {
			return fmt.Sprintf("it contains %d /%s elements", n, name)
		}
		return ""
	}
}

func needsIfPresent(have, need string) func([]string) string {
	return func(kids []string) string {
		if count(kids, have) > 0 && count(kids, need) == 0 {
			return fmt.Sprintf("it contains a /%s and no /%s", have, need)
		}
		return ""
	}
}

func captionAtEnds(kids []string) string {
	for i, k := range kids {
		if k == "Caption" && i > 0 && i < len(kids)-1 {
			return fmt.Sprintf("its /Caption is kid %d of %d", i+1, len(kids))
		}
	}
	return ""
}

func captionFirstOnly(kids []string) string {
	for i, k := range kids {
		if k == "Caption" && i > 0 {
			return fmt.Sprintf("its /Caption is kid %d of %d", i+1, len(kids))
		}
	}
	return ""
}

// checkKidSequence evaluates one cardinality or placement clause — the only function that does. The kid
// sequence is each element's own `/K`, typed, with untyped kids dropped (measured, above).
func checkKidSequence(d *Document, r kidSequence) Result {
	nodes, unread := d.structNodes()
	subjects := 0
	for _, n := range nodes {
		if r.subject != "" && d.typedAs(n.dict) != r.subject {
			continue
		}
		subjects++
		var kids []string
		for _, kid := range d.elementKids(n.dict) {
			if kt := d.typedAs(kid); kt != "" {
				kids = append(kids, kt)
			}
		}
		if why := r.broken(kids); why != "" {
			// An every-element row names the element as written, whose article `article` cannot know ("an /Link"),
			// so it takes "the"; a typed row keeps its type's article.
			lead := fmt.Sprintf("%s /%s", article(r.subject), r.subject)
			if r.subject == "" {
				lead = "the /" + d.name(n.dict["S"])
			}
			return Result{Verdict: Fail, Where: nodeWhere(n, d.name(n.dict["S"])),
				Why: fmt.Sprintf("%s element breaks it: %s", lead, why)}
		}
	}
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	if subjects == 0 {
		if r.subject == "" {
			return Result{Verdict: NotApplicable, Why: "the document has no structure elements"}
		}
		return Result{Verdict: NotApplicable, Why: fmt.Sprintf("the document has no /%s element", r.subject)}
	}
	return Result{Verdict: Pass}
}
