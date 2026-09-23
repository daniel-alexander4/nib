package uacheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P03.S02 — every row below is a `treeDoc` veraPDF 1.30.2 was run on before the matrix was written, and
// its verdicts are veraPDF's. Every clause is driven in BOTH directions: for thirteen of the seventeen the
// corpus holds only fail files, and a clause that answered Fail unconditionally would score perfectly there.

// containmentCases is each clause's measured pass and fail document.
var containmentCases = []struct {
	clause, pass, fail string
}{
	{"7.2 t3", goodTable, "Document(Table(TR(TD),Span))"},
	{"7.2 t4", goodTable, "Document(TR(TD))"},
	{"7.2 t5", goodTable, "Document(THead(TR(TH)))"},
	{"7.2 t6", goodTable, "Document(TBody(TR(TD)))"},
	{"7.2 t7", goodTable, "Document(TFoot(TR(TD)))"},
	{"7.2 t8", goodTable, "Document(Table(TH))"},
	{"7.2 t9", goodTable, "Document(Table(TD))"},
	{"7.2 t10", goodTable, "Document(Table(TR(TD,P)))"},
	{"7.2 t17", goodList, "Document(P(LI(LBody)))"},
	{"7.2 t18", goodList, "Document(L(LBody))"},
	{"7.2 t19", goodList, "Document(L(P))"},
	{"7.2 t20", goodList, "Document(L(LI(P)))"},
	{"7.2 t26", goodTOC, "Document(TOCI)"},
	{"7.2 t27", goodTOC, "Document(TOC(TOCI,P))"},
	{"7.2 t36", goodTable, "Document(Table(THead(TD)))"},
	{"7.2 t37", goodTable, "Document(Table(TBody(TD)))"},
	{"7.2 t38", goodTable, "Document(Table(TFoot(TD)))"},
}

const (
	goodTable = "Document(Table(Caption,THead(TR(TH)),TBody(TR(TD)),TFoot(TR(TD))))"
	goodList  = "Document(L(Caption,LI(Lbl,LBody),L(LI(LBody))))"
	goodTOC   = "Document(TOC(Caption,TOCI,TOC(TOCI)))"
)

func TestEveryContainmentClauseAgreesWithVeraPDFBothWays(t *testing.T) {
	// The SETS, not the counts: a duplicated case would hide a missing one from a count.
	cased := map[string]bool{}
	for _, tc := range containmentCases {
		cased[tc.clause] = true
	}
	for _, c := range containmentMatrix {
		if !cased[c.clause] {
			t.Errorf("%s has no measured pass and fail document — every clause in the matrix owes both", c.clause)
		}
	}
	if len(cased) != len(containmentCases) || len(cased) != len(containmentMatrix) {
		t.Fatalf("%d cases over %d distinct clauses for a matrix of %d — a case is duplicated or names a clause the matrix lacks", len(containmentCases), len(cased), len(containmentMatrix))
	}
	for _, tc := range containmentCases {
		if got := verdictOf(t, treeDoc("", tc.pass), tc.clause); got.Verdict != Pass {
			t.Errorf("%s over %s = %v (%s), want Pass", tc.clause, tc.pass, got.Verdict, got.Why)
		}
		if got := verdictOf(t, treeDoc("", tc.fail), tc.clause); got.Verdict != Fail {
			t.Errorf("%s over %s = %v (%s), want Fail", tc.clause, tc.fail, got.Verdict, got.Why)
		}
	}
}

// TestContainmentReadsTypesTheWayVeraPDFDoes — the edges, each measured.
func TestContainmentReadsTypesTheWayVeraPDFDoes(t *testing.T) {
	for _, tc := range []struct {
		name, roleMap, spec, clause string
		want                        Verdict
	}{
		{"a kid on a role-map loop is ignored", "/X /Y /Y /X", "Document(Table(TR(TD),X))", "7.2 t3", Pass},
		{"a kid dead-ending at a private name is ignored", "", "Document(Table(TR(TD),Q))", "7.2 t3", Pass},
		{"a kid mapped to itself is ignored", "/Z /Z", "Document(Table(TR(TD),Z))", "7.2 t3", Pass},
		{"a role-mapped kid resolves first", "/Z /TR", "Document(Table(Z(TD)))", "7.2 t3", Pass},
		{"a role-mapped parent resolves first", "/W /Table", "Document(W(TR(TD)))", "7.2 t4", Pass},
		{"a parent on a role-map loop satisfies nothing", "/X /Y /Y /X", "Document(Table(X(TD)))", "7.2 t9", Fail},
		{"the structure tree root is not a Table", "", "TR(TD)", "7.2 t4", Fail},
		{"content in the /K is not a kid", "", "Document(Table(TR(TD),#mcid,#objr))", "7.2 t3", Pass},
		{"an element with no element kids passes", "", "Document(Table)", "7.2 t3", Pass},
		{"no subject is not applicable", "", "Document(P)", "7.2 t3", NotApplicable},
	} {
		if got := verdictOf(t, treeDoc(tc.roleMap, tc.spec), tc.clause); got.Verdict != tc.want {
			t.Errorf("%s: %s over %s = %v (%s), want %v", tc.name, tc.clause, tc.spec, got.Verdict, got.Why, tc.want)
		}
	}
}

// TestContainmentIsWrittenOnlyInTheMatrix — ADR-009's guard: it checks the DOOR, not seventeen messages.
// A containment clause registered anywhere but the matrix would be a second opinion on the same relation.
func TestContainmentIsWrittenOnlyInTheMatrix(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range containmentMatrix {
		registered, ok := registry[c.clause]
		if !ok {
			t.Errorf("%s is in the matrix and not registered", c.clause)
			continue
		}
		if registered.Summary != c.summary() {
			t.Errorf("%s is registered with a summary the matrix did not generate: %q", c.clause, registered.Summary)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") || f == "rules_containment.go" {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(src), `"`+c.clause+`"`) {
				t.Errorf("%s names %s outside the matrix; a containment clause is written only in rules_containment.go", f, c.clause)
			}
		}
	}
	if len(containmentMatrix) != 17 {
		t.Errorf("the matrix holds %d clauses; P03.S02 landed 17", len(containmentMatrix))
	}
}

// relationDoc is a table document written object by object, so an element's `/P` and its place in a `/K`
// can disagree and one element can sit in two parents' `/K` — shapes `treeDoc` cannot write, because it
// always agrees with itself.
func relationDoc(elems map[int]string) []byte {
	c := "/Span << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (a) Tj ET EMC\n/Span << /MCID 1 >> BDC BT /F1 12 Tf 72 680 Td (b) Tj ET EMC\n"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /ViewerPreferences << /DisplayDocTitle true >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Tabs /S /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [20 0 R] /ParentTree 8 0 R >>",
		8: "<< /Nums [0 [25 0 R 23 0 R]] >>",
	}
	for k, v := range elems {
		objs[k] = v
	}
	return buildPDF(objs)
}

// TestContainmentReadsTheElementsOwnRelationsNotTheWalks — found by P03.S02's review and confirmed on
// veraPDF 1.30.2, each row: the tree walk visits an element once and remembers where it FOUND it, and
// veraPDF reads the element's own `/P` and each element's own `/K`. Both false passes were live.
func TestContainmentReadsTheElementsOwnRelationsNotTheWalks(t *testing.T) {
	const (
		doc   = "<< /Type /StructElem /S /Document /P 7 0 R /K [%s] >>"
		table = "<< /Type /StructElem /S /Table /P 20 0 R /K [%s] >>"
		tr    = "<< /Type /StructElem /S /TR /P 22 0 R /K [%s] >>"
		p     = "<< /Type /StructElem /S /P /P %s /Pg 3 0 R /K [1] >>"
	)
	for _, tc := range []struct {
		name   string
		elems  map[int]string
		clause string
		want   Verdict
	}{
		{"a P listed in both a Div's and a Table's /K is the Table's kid too", map[int]string{
			20: fmt.Sprintf(doc, "21 0 R 22 0 R"),
			21: "<< /Type /StructElem /S /Div /P 20 0 R /K [23 0 R] >>",
			22: fmt.Sprintf(table, "24 0 R 23 0 R"),
			24: fmt.Sprintf(tr, "25 0 R"),
			25: "<< /Type /StructElem /S /TD /P 24 0 R /Pg 3 0 R /K [0] >>",
			23: fmt.Sprintf(p, "21 0 R"),
		}, "7.2 t3", Fail},
		{"a TD in a TR's /K whose /P names the Document", map[int]string{
			20: fmt.Sprintf(doc, "22 0 R 23 0 R"),
			22: fmt.Sprintf(table, "24 0 R"),
			24: fmt.Sprintf(tr, "25 0 R"),
			25: "<< /Type /StructElem /S /TD /P 20 0 R /Pg 3 0 R /K [0] >>",
			23: fmt.Sprintf(p, "20 0 R"),
		}, "7.2 t9", Fail},
		{"a TD in a TR's /K with no /P", map[int]string{
			20: fmt.Sprintf(doc, "22 0 R 23 0 R"),
			22: fmt.Sprintf(table, "24 0 R"),
			24: fmt.Sprintf(tr, "25 0 R"),
			25: "<< /Type /StructElem /S /TD /Pg 3 0 R /K [0] >>",
			23: fmt.Sprintf(p, "20 0 R"),
		}, "7.2 t9", Fail},
		{"a TD in the Document's /K whose /P names a TR", map[int]string{
			20: fmt.Sprintf(doc, "22 0 R 25 0 R 23 0 R"),
			22: fmt.Sprintf(table, "24 0 R"),
			24: fmt.Sprintf(tr, "26 0 R"),
			26: "<< /Type /StructElem /S /TD /P 24 0 R /Pg 3 0 R /K [] >>",
			25: "<< /Type /StructElem /S /TD /P 24 0 R /Pg 3 0 R /K [0] >>",
			23: fmt.Sprintf(p, "20 0 R"),
		}, "7.2 t9", Pass},
	} {
		if got := verdictOf(t, relationDoc(tc.elems), tc.clause); got.Verdict != tc.want {
			t.Errorf("%s: %s = %v (%s), want %v", tc.name, tc.clause, got.Verdict, got.Why, tc.want)
		}
	}
}

// TestNonStructDivAndPartAreLookedThrough — veraPDF reads an element's kids and its parent THROUGH `NonStruct`,
// `Div` and `Part` (veraPDF-parser `isPassThroughTag`). Measured on veraPDF 1.30.2 for every row; P03.S02 and
// S03 shipped without it and false-failed the first six, and false-PASSED the last.
func TestNonStructDivAndPartAreLookedThrough(t *testing.T) {
	for _, tc := range []struct {
		spec, clause string
		want         Verdict
	}{
		{"Document(Table(Div(TR(TD))))", "7.2 t3", Pass},
		{"Document(Table(Div(TR(TD))))", "7.2 t4", Pass},
		{"Document(L(NonStruct(LI(LBody))))", "7.2 t17", Pass},
		{"Document(Table(TR(Part(TD))))", "7.2 t9", Pass},
		{"Document(L(Div(Caption),LI(LBody)))", "7.2 t40", Pass},
		{"Document(TOC(Div(TOCI)))", "7.2 t26", Pass},
		{"Document(Table(THead(TR(TH)),Div(THead(TR(TH))),TBody(TR(TD))))", "7.2 t11", Fail},
		// A Div holding a Span inside a Table: the Span is the Table's kid, and a Span is not allowed.
		{"Document(Table(TR(TD),Div(Span)))", "7.2 t3", Fail},
	} {
		if got := verdictOf(t, treeDoc("", tc.spec), tc.clause); got.Verdict != tc.want {
			t.Errorf("%s over %s = %v (%s), want %v", tc.clause, tc.spec, got.Verdict, got.Why, tc.want)
		}
	}
}
