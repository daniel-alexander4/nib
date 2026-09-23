package uacheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P03.S03 — every document below was run through veraPDF 1.30.2 before the rule it grades was written, and
// each verdict is veraPDF's. Four of these clauses (7.2 t16, t28, t39, t40) have no corpus file at all.

func TestEveryKidSequenceClauseAgreesWithVeraPDFBothWays(t *testing.T) {
	const good = "Document(Table(Caption,THead(TR(TH)),TBody(TR(TD)),TFoot(TR(TD))),L(Caption,LI(LBody)),TOC(Caption,TOCI))"
	cases := []struct{ clause, fail string }{
		{"7.2 t11", "Document(Table(THead(TR(TH)),THead(TR(TH)),TBody(TR(TD))))"},
		{"7.2 t12", "Document(Table(TBody(TR(TD)),TFoot(TR(TD)),TFoot(TR(TD))))"},
		{"7.2 t13", "Document(Table(TFoot(TR(TD))))"},
		{"7.2 t14", "Document(Table(THead(TR(TH))))"},
		{"7.2 t16", "Document(Table(TR(TD),Caption,TR(TD)))"},
		{"7.2 t39", "Document(Table(Caption,TR(TD),Caption))"},
		{"7.2 t28", "Document(TOC(TOCI,Caption))"},
		{"7.2 t40", "Document(L(LI(LBody),Caption))"},
		{"7.4.4 t1", "Document(H,H)"},
	}
	// The SETS, not the counts, and the row count pinned, as the matrix's is.
	if len(kidSequenceRules) != 9 {
		t.Fatalf("%d kid-sequence rows, want 9 (P03.S03's eight and S05's 7.4.4 t1) — a row added or lost moves this", len(kidSequenceRules))
	}
	cased := map[string]bool{}
	for _, tc := range cases {
		cased[tc.clause] = true
	}
	for _, r := range kidSequenceRules {
		if !cased[r.clause] {
			t.Errorf("%s has no measured failing document", r.clause)
		}
	}
	if len(cased) != len(cases) || len(cased) != len(kidSequenceRules) {
		t.Fatalf("%d cases over %d distinct clauses for %d rows — a case is duplicated or names no row", len(cases), len(cased), len(kidSequenceRules))
	}
	for _, tc := range cases {
		if got := verdictOf(t, treeDoc("", good), tc.clause); got.Verdict != Pass {
			t.Errorf("%s over the well-formed document = %v (%s), want Pass", tc.clause, got.Verdict, got.Why)
		}
		if got := verdictOf(t, treeDoc("", tc.fail), tc.clause); got.Verdict != Fail {
			t.Errorf("%s over %s = %v (%s), want Fail", tc.clause, tc.fail, got.Verdict, got.Why)
		}
	}
}

// TestAKidSequenceIsTheTypedKidsInOrder — the edges, each measured.
func TestAKidSequenceIsTheTypedKidsInOrder(t *testing.T) {
	const loop = "/X /Y /Y /X"
	for _, tc := range []struct {
		name, roleMap, spec, clause string
		want                        Verdict
	}{
		{"a Caption last is at an end", "", "Document(Table(TR(TD),Caption))", "7.2 t16", Pass},
		{"a second leading Caption is in the middle", "", "Document(Table(Caption,Caption,TR(TD)))", "7.2 t16", Fail},
		{"an untyped first kid is dropped, so the Caption is first (Table)", loop, "Document(Table(X,Caption,TR(TD)))", "7.2 t16", Pass},
		{"an untyped last kid is dropped, so the Caption is last", loop, "Document(Table(TR(TD),Caption,X))", "7.2 t16", Pass},
		{"an untyped first kid is dropped, so the Caption is first (L)", loop, "Document(L(X,Caption,LI(LBody)))", "7.2 t40", Pass},
		{"an untyped first kid is dropped, so the Caption is first (TOC)", loop, "Document(TOC(X,Caption,TOCI))", "7.2 t28", Pass},
		{"a role-mapped Caption is a Caption", "/Cap /Caption", "Document(Table(TR(TD),Cap,TR(TD)))", "7.2 t16", Fail},
		{"an empty Table passes", "", "Document(Table)", "7.2 t13", Pass},
		{"no subject is not applicable", "", "Document(P)", "7.2 t40", NotApplicable},
	} {
		if got := verdictOf(t, treeDoc(tc.roleMap, tc.spec), tc.clause); got.Verdict != tc.want {
			t.Errorf("%s: %s over %s = %v (%s), want %v", tc.name, tc.clause, tc.spec, got.Verdict, got.Why, tc.want)
		}
	}
}

// TestKidSequencesAreWrittenOnlyInTheirTable — the same one-door guard as the matrix's (ADR-009).
func TestKidSequencesAreWrittenOnlyInTheirTable(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range kidSequenceRules {
		if _, ok := registry[r.clause]; !ok {
			t.Errorf("%s is in the table and not registered", r.clause)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") || f == "rules_kidsequence.go" {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(src), `"`+r.clause+`"`) {
				t.Errorf("%s names %s outside rules_kidsequence.go", f, r.clause)
			}
		}
	}
}

// TestAContentReferenceIsNeverAKidEvenWithAnS — found by P03.S03's review and measured on veraPDF 1.30.2: a
// `/Type /MCR` dictionary carrying `/S /Caption` between two TRs is content, and 7.2-16 passes. The `/Type`
// test in `elementKids` had been removed as dead because no fixture carried the shape.
func TestAContentReferenceIsNeverAKidEvenWithAnS(t *testing.T) {
	pdf := relationDoc(map[int]string{
		20: "<< /Type /StructElem /S /Document /P 7 0 R /K [22 0 R 23 0 R] >>",
		22: "<< /Type /StructElem /S /Table /P 20 0 R /K [24 0 R << /Type /MCR /S /Caption /Pg 3 0 R /MCID 1 >> 26 0 R] >>",
		24: "<< /Type /StructElem /S /TR /P 22 0 R /K [25 0 R] >>",
		25: "<< /Type /StructElem /S /TD /P 24 0 R /Pg 3 0 R /K [0] >>",
		26: "<< /Type /StructElem /S /TR /P 22 0 R /K [] >>",
		23: "<< /Type /StructElem /S /P /P 20 0 R /Pg 3 0 R /K [] >>",
	})
	if got := verdictOf(t, pdf, "7.2 t16"); got.Verdict != Pass {
		t.Errorf("7.2 t16 = %v (%s), want Pass — an MCR is content, whatever /S it carries", got.Verdict, got.Why)
	}
}
