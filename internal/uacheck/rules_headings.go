package uacheck

import (
	"fmt"
	"strconv"
)

// The numbered-heading rule — `/pending 487`.
//
// ua1 7.4.2 t1 is veraPDF's `hasCorrectNestingLevel` over every `Hn` element. Its behaviour was read off
// veraPDF's own corpus rather than off the description's prose, heading sequence by heading sequence:
//
//	7.4.2-t01-fail-a  H2 → H3 → H4                     failed, 1 check (the first heading)
//	7.4.2-t01-fail-b  H1 → H2 → H4                     failed, 1 check (the skip)
//	7.4.2-t01-pass-a  H1 → H2 → H3                     passed
//	7.4.2-t01-pass-b  H1 → H1 → H1 → H1                passed
//	7.4.2-t01-pass-c  H1 H2 H3 H4 H3 H4 H3 H4 H2 H3    passed — returning to a shallower level is fine
//	7.4.2-t01-pass-d  H1 → H2 → H3, each nested deeper passed — nesting in the tree does not matter
//
// So: in structure-tree order, the first numbered heading is H1, and each later one is no more than one
// level deeper than the heading before it. `veracorpus_test.go` holds the reading against all six.

func init() {
	register(Rule{
		Clause:  "7.4.2 t1",
		Summary: "numbered headings shall start at H1 and shall not skip a level when descending",
		Check:   checkHeadingNesting,
	})
	register(Rule{
		Clause:  "7.4.4 t2",
		Summary: "a document shall be strongly or weakly structured, not both — no H where there are Hn",
		Check:   func(d *Document) Result { return checkHeadingStyles(d, true) },
	})
	register(Rule{
		Clause:  "7.4.4 t3",
		Summary: "a document shall be strongly or weakly structured, not both — no Hn where there is H",
		Check:   func(d *Document) Result { return checkHeadingStyles(d, false) },
	})
}

// checkHeadingStyles evaluates ua1 7.4.4 t2 (subjects: every H, onH) and t3 (subjects: every Hn) —
// `PLAN-ua-coverage.md` P03.S05.
//
// veraPDF writes them as profile VARIABLES (`usesH` set by any SEH, `usesHn` by any SEHn), which read as though
// the answer depended on traversal order. Measured on veraPDF 1.30.2, it does not: in fourteen trees with the two
// kinds in every order and nesting, a document holding both fails EVERY H on t2 and EVERY Hn on t3, and one
// holding a single kind passes that kind's clause and has no subject for the other. So the rule is the plain
// document-level one, over typed elements.
func checkHeadingStyles(d *Document, onH bool) Result {
	nodes, unread := d.structNodes()
	var hs, hns []structNode
	for _, n := range nodes {
		switch ty := d.typedAs(n.dict); {
		case ty == "H":
			hs = append(hs, n)
		case isNumberedHeading(ty):
			hns = append(hns, n)
		}
	}
	subjects, others, own, other := hs, hns, "H", "numbered headings (H1-H6)"
	if !onH {
		subjects, others, own, other = hns, hs, "numbered heading", "unnumbered H headings"
	}
	if len(subjects) > 0 && len(others) > 0 {
		return Result{Verdict: Fail, Where: nodeWhere(subjects[0], d.name(subjects[0].dict["S"])),
			Why: fmt.Sprintf("the document uses this %s alongside %s, so it is both strongly and weakly structured", own, other)}
	}
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	if len(subjects) == 0 {
		return Result{Verdict: NotApplicable, Why: fmt.Sprintf("the document has no %s", map[bool]string{true: "H element", false: "numbered heading"}[onH])}
	}
	return Result{Verdict: Pass}
}

// checkHeadingNesting evaluates ua1 7.4.2 t1.
func checkHeadingNesting(d *Document) Result {
	nodes, unread := d.structNodes()
	if unread != "" {
		// The order of the headings past the bound is unknown, so no heading sequence can be said to hold.
		return Result{Verdict: CannotCheck, Why: unread}
	}
	std, untyped := d.standardTypes(nodes)
	if untyped != "" {
		// An element nib cannot type may be a heading, and one unplaced heading moves the whole sequence.
		return Result{Verdict: CannotCheck, Why: untyped}
	}
	prev := 0
	for i, n := range nodes {
		level, ok := numberedHeading(std[i])
		if !ok {
			continue
		}
		where := "structure element " + headingName(level)
		if n.obj != 0 {
			where = fmt.Sprintf("structure element %s (object %d)", headingName(level), n.obj)
		}
		switch {
		case prev == 0 && level != 1:
			return Result{
				Verdict: Fail,
				Why:     fmt.Sprintf("the first heading is %s; a document's numbered headings start at H1", headingName(level)),
				Where:   where,
			}
		case prev != 0 && level > prev+1:
			return Result{
				Verdict: Fail,
				Why:     fmt.Sprintf("%s follows %s, skipping a level", headingName(level), headingName(prev)),
				Where:   where,
			}
		}
		prev = level
	}
	if prev == 0 {
		return Result{Verdict: NotApplicable, Why: "the structure tree has no numbered heading (H1–H6)"}
	}
	return Result{Verdict: Pass}
}

// numberedHeading reports the level of a standard structure type H1 to H6.
func numberedHeading(standard string) (int, bool) {
	if len(standard) != 2 || standard[0] != 'H' {
		return 0, false
	}
	n, err := strconv.Atoi(standard[1:])
	if err != nil || n < 1 || n > 6 {
		return 0, false
	}
	return n, true
}

// isNumberedHeading is numberedHeading's yes-or-no — the one door for "is this an Hn" (ADR-009), which 7.4.4
// t2/t3 read as well as 7.4.2 t1.
func isNumberedHeading(standard string) bool { _, ok := numberedHeading(standard); return ok }

func headingName(level int) string { return "H" + strconv.Itoa(level) }
