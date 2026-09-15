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
}

// checkHeadingNesting evaluates ua1 7.4.2 t1.
func checkHeadingNesting(d *Document) Result {
	prev := 0
	for _, n := range d.structNodes() {
		level, ok := numberedHeading(d.standardType(n.dict))
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

func headingName(level int) string { return "H" + strconv.Itoa(level) }
