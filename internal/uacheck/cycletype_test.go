package uacheck

import "testing"

// TestAnElementOnARoleMapLoopIsNotAFigureOrAFormula — `/pending 634`.
//
// An element whose type the role map sends round a loop has no standard type. 7.3 t1 and 7.7 t1 answered CannotCheck
// for the WHOLE document wherever one existed — every file on which 7.1 t6 already fails — on the grounds that the
// element might be a Figure. veraPDF types it as nothing: measured on 1.30.2, each row below, and asked again here
// whenever veraPDF is present. 7.4.2 t1 is beside them as the control that already read it this way.
func TestAnElementOnARoleMapLoopIsNotAFigureOrAFormula(t *testing.T) {
	el := func(s, extra string) string { return "<< /Type /StructElem /S /" + s + " /P 7 0 R " + extra + " >>" }
	const loop = "/A /B /B /A"
	cases := []struct {
		name string
		pdf  []byte
		want [3]Verdict // 7.3 t1, 7.7 t1, 7.4.2 t1
	}{
		{"only the element on the loop", semanticDoc(map[int]string{10: el("A", "")}, []int{10}, loop), [3]Verdict{NotApplicable, NotApplicable, NotApplicable}},
		{"beside it, a Figure with no alt", semanticDoc(map[int]string{10: el("A", ""), 11: el("Figure", "")}, []int{10, 11}, loop), [3]Verdict{Fail, NotApplicable, NotApplicable}},
		{"beside it, a Figure with an alt", semanticDoc(map[int]string{10: el("A", ""), 11: el("Figure", "/Alt (x)")}, []int{10, 11}, loop), [3]Verdict{Pass, NotApplicable, NotApplicable}},
		{"beside it, a Formula with no alt", semanticDoc(map[int]string{10: el("A", ""), 11: el("Formula", "")}, []int{10, 11}, loop), [3]Verdict{NotApplicable, Fail, NotApplicable}},
		{"beside it, a Formula with an alt", semanticDoc(map[int]string{10: el("A", ""), 11: el("Formula", "/Alt (x)")}, []int{10, 11}, loop), [3]Verdict{NotApplicable, Pass, NotApplicable}},
		{"beside it, H1 then H3", semanticDoc(map[int]string{10: el("A", ""), 11: el("H1", ""), 12: el("H3", "")}, []int{10, 11, 12}, loop), [3]Verdict{NotApplicable, NotApplicable, Fail}},
		{"between H1 and H3", semanticDoc(map[int]string{10: el("A", ""), 11: el("H1", ""), 12: el("H3", "")}, []int{11, 10, 12}, loop), [3]Verdict{NotApplicable, NotApplicable, Fail}},
		{"beside it, H1 then H2", semanticDoc(map[int]string{10: el("A", ""), 11: el("H1", ""), 12: el("H2", "")}, []int{10, 11, 12}, loop), [3]Verdict{NotApplicable, NotApplicable, Pass}},
		// No loop: the same Figure answers the same way, so the loop is what the rows above are about.
		{"a Figure with no alt, no loop", semanticDoc(map[int]string{11: el("Figure", "")}, []int{11}, ""), [3]Verdict{Fail, NotApplicable, NotApplicable}},
		{"an unmapped type beside a Figure with no alt", semanticDoc(map[int]string{10: el("Zed", ""), 11: el("Figure", "")}, []int{10, 11}, ""), [3]Verdict{Fail, NotApplicable, NotApplicable}},
	}
	clauses := [3]string{"7.3 t1", "7.7 t1", "7.4.2 t1"}
	words := map[Verdict]string{Pass: "passed", Fail: "failed", NotApplicable: "none"}
	var docs [][]byte
	for _, c := range cases {
		docs = append(docs, c.pdf)
	}
	vera := veraAsk(t, docs)
	for i, c := range cases {
		for k, clause := range clauses {
			if got := verdictOf(t, c.pdf, clause); got.Verdict != c.want[k] {
				t.Errorf("%s: %s reports %v (%s), want %v", c.name, clause, got.Verdict, got.Why, c.want[k])
			}
			if vera != nil && vera[i][clause] != words[c.want[k]] {
				t.Errorf("%s: veraPDF now says %q for %s where %q was measured — the row is stale", c.name, vera[i][clause], clause, words[c.want[k]])
			}
		}
	}
}
