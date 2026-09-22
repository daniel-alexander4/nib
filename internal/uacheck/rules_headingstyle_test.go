package uacheck

import "testing"

// P03.S05 — every tree below was run through veraPDF 1.30.2 before it was pinned; each verdict is veraPDF's.
func TestHeadingStructureAgreesWithVeraPDF(t *testing.T) {
	for _, tc := range []struct {
		roleMap, spec string
		t1, t2, t3    Verdict
	}{
		{"", "Document(H1,H)", Pass, Fail, Fail},
		{"", "Document(H,H1)", Pass, Fail, Fail},
		{"", "Document(Sect(Sect(H)),H1)", Pass, Fail, Fail},
		{"", "Document(H,H1,H)", Fail, Fail, Fail},
		{"", "Document(H,H)", Fail, Pass, NotApplicable},
		{"", "Document(Sect(H),Sect(H))", Pass, Pass, NotApplicable},
		{"", "Document(Div(H),H)", Fail, Pass, NotApplicable},
		{"", "Document(H1,P,H2,P)", Pass, NotApplicable, Pass},
		// Role-mapped headings are headings, and a non-standard parent is a subject of t1 (veraPDF: PDStructElem).
		{"/Heading /H", "Document(Heading,H1)", Pass, Fail, Fail},
		{"/Title /H1", "Document(H,Title)", Pass, Fail, Fail},
		{"", "Document(Link(H,H))", Fail, Pass, NotApplicable},
	} {
		for _, c := range []struct {
			clause string
			want   Verdict
		}{{"7.4.4 t1", tc.t1}, {"7.4.4 t2", tc.t2}, {"7.4.4 t3", tc.t3}} {
			if got := verdictOf(t, treeDoc(tc.roleMap, tc.spec), c.clause); got.Verdict != c.want {
				t.Errorf("%s over %s = %v (%s), want %v", c.clause, tc.spec, got.Verdict, got.Why, c.want)
			}
		}
	}
}
