package uacheck

import (
	"bytes"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// The first rule, against documents whose veraPDF verdict this repo already measured — P07.S01.

// TestStructTreeRootAgreesWithWhatP05AndP06Measured.
//
// This is law 5 in miniature, on one clause. The expected verdicts are not invented: each is what
// veraPDF reported about that exact document shape during P05 and P06, recorded in the plan and in
// the instrument inventory. P07.S05 generalises it to every clause over the corpus.
func TestStructTreeRootAgreesWithWhatP05AndP06Measured(t *testing.T) {
	plain, err := testpdf.Text("no structure here")
	if err != nil {
		t.Fatal(err)
	}
	tagged := committedProposal(t, plain)

	for _, c := range []struct {
		name string
		pdf  []byte
		want Verdict
	}{
		// veraPDF failed 7.1 t11 on every untagged nib document measured in P03–P06.
		{"an untagged page", plain, Fail},
		// And passed it on every document with a tree that reaches content — P05.S05 onward.
		{"a committed proposal", tagged, Pass},
	} {
		rep, err := Check(c.pdf)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		var got Result
		for _, r := range rep.Results {
			if r.Clause == "7.1 t11" {
				got = r
			}
		}
		if got.Clause == "" {
			t.Errorf("%s: 7.1 t11 is not in the report at all, so the registry is not being run",
				c.name)
			continue
		}
		if got.Verdict != c.want {
			t.Errorf("%s: 7.1 t11 reports %v (%s), want %v — this is the verdict veraPDF gave "+
				"this document shape throughout P05 and P06", c.name, got.Verdict, got.Why, c.want)
		}
		if c.want == Fail {
			if got.Why == "" {
				t.Errorf("%s: the failure carries no reason", c.name)
			}
			if got.Where == "" {
				t.Errorf("%s: the failure does not say WHERE. P01 spent a slice discovering that "+
					"veraPDF points at `xObject[0]/contentStream[0]/content[2]`, and a checker "+
					"that says only `7.1 t11 fails` reproduces the problem it exists to solve",
					c.name)
			}
		}
	}
}

// TestAClaimedStructureThatHoldsNothingFAILS — ADR-031 law 1's first case, read from the other side.
//
// A catalog naming a `/StructTreeRoot` whose tree is empty is `tagState.orphaned()` — the state P01's
// phase review caught being rated `carried`. The checker must call that a failure and not a
// pass-on-a-technicality: the clause asks for logical structure, and an empty root is a claim with
// nothing behind it.
func TestAClaimedStructureThatHoldsNothingFAILS(t *testing.T) {
	rep, err := Check(emptyRootFixture())
	if err != nil {
		t.Fatalf("the fixture could not be read: %v", err)
	}
	var got Result
	for _, r := range rep.Results {
		if r.Clause == "7.1 t11" {
			got = r
		}
	}
	if got.Verdict != Fail {
		t.Errorf("a catalog naming a structure root that holds nothing reports %v (%s), want Fail",
			got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "no children") && !strings.Contains(got.Why, "holds nothing") {
		t.Errorf("the reason does not say the root is empty: %q", got.Why)
	}
}

// TestADocumentThatCannotBeReadIsAnErrorNotAFailingReport.
//
// A checker that answered "fails every clause" for a file it could not parse would tell the user
// their document is inaccessible when what happened is that nib could not read it. Two different
// facts, and the second is not the user's to fix.
func TestADocumentThatCannotBeReadIsAnErrorNotAFailingReport(t *testing.T) {
	for _, c := range []struct {
		name string
		pdf  []byte
	}{
		{"nothing at all", nil},
		{"not a PDF", []byte("this is not a PDF at all, not even close")},
	} {
		rep, err := Check(c.pdf)
		if err == nil {
			t.Errorf("%s: Check returned no error and a report of %d result(s) — a document nib "+
				"cannot read must not be reported as a document that fails clauses",
				c.name, len(rep.Results))
		}
		if len(rep.Results) != 0 {
			t.Errorf("%s: an unreadable document produced %d verdict(s)", c.name, len(rep.Results))
		}
	}
}

// emptyRootFixture is a document whose catalog names a `/StructTreeRoot` with an empty `/K`.
//
// Generated rather than committed, for `corpus_test.go`'s stated reason: *"A checked-in binary
// fixture is opaque in review — you cannot see what changed, or what it contains, without opening
// it in something."*
func emptyRootFixture() []byte {
	return buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length 44 >>\nstream\nBT /F1 24 Tf 72 700 Td (Untagged) Tj ET\nendstream",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [] >>",
	})
}

// buildPDF assembles numbered objects into a minimal valid PDF with a correct xref table.
//
// The same assembly `internal/pdfops/corpus_test.go` uses, written here rather than exported from
// there because a test helper exported across packages to save twenty lines is a dependency between
// two test suites — and this package's fixtures are about the checker, not about pdfops.
// The circular-role-map rule — `/pending 548`.
//
// `shortread_test.go` already builds the documents: `roleMapDoc(resolvedRoleMap)` types its three elements
// through a role map that terminates, and `roleMapDoc(cyclicRoleMap)` sends `/Alpha` around
// `/Delta → /Echo → /Alpha`. Those fixtures asserted what the THREE TREE RULES do over a cycle; these
// assert the rule that is about the cycle itself.

// unusedCycleRoleMap resolves every type an element actually uses and leaves a loop no element enters.
//
// It is the shape a document-scoped reading of *"a circular mapping shall not exist"* fails, and the shape
// a producer leaves behind when a private type stops being emitted but its mapping stays.
const unusedCycleRoleMap = resolvedRoleMap + " /Xray /Yankee /Yankee /Xray"

func TestACircularRoleMapFailsTheClauseThatIsAboutIt(t *testing.T) {
	// Control: the same document with the map resolved must PASS, or a Fail below would be a rule that
	// fails whatever it is shown.
	if got := verdictOf(t, roleMapDoc(resolvedRoleMap), "7.1 t6"); got.Verdict != Pass {
		t.Fatalf("control: with the role map resolved, 7.1 t6 reports %v (%s), want Pass", got.Verdict, got.Why)
	}
	got := verdictOf(t, roleMapDoc(cyclicRoleMap), "7.1 t6")
	if got.Verdict != Fail {
		t.Fatalf("with /Alpha sent around /Delta → /Echo → /Alpha, 7.1 t6 reports %v (%s), want Fail — nib holds "+
			"the cycle and this is the clause it breaks", got.Verdict, got.Why)
	}
	// The reason must name the loop, and the location the element standing on it. A Fail that says only
	// "7.1 t6 fails" reproduces the problem P01 spent a slice discovering veraPDF had already solved.
	if !strings.Contains(got.Why, "loop") || !strings.Contains(got.Why, "circular") {
		t.Errorf("the reason %q does not say the role map is circular", got.Why)
	}
	if !strings.Contains(got.Where, "/Alpha") {
		t.Errorf("the location %q does not name the element whose type is on the loop", got.Where)
	}
}

// TestACircularRoleMapNoElementUsesIsNotAFailure — veraPDF's object for 7.1-6 is `PDStructElem`, so the
// subject is the element and not the dictionary. Measured on `7.1-t05-fail-d.pdf`: 2 passed checks and 2
// failed over four elements, the two passes being the ones whose `/S` is off the loop.
//
// A document-scoped implementation passes every other assertion in this file and fails only here.
func TestACircularRoleMapNoElementUsesIsNotAFailure(t *testing.T) {
	got := verdictOf(t, roleMapDoc(unusedCycleRoleMap), "7.1 t6")
	if got.Verdict != Pass {
		t.Errorf("with /Xray ↔ /Yankee looping and no element naming either, 7.1 t6 reports %v (%s), want Pass — "+
			"veraPDF checks the element, not the dictionary", got.Verdict, got.Why)
	}
}

// deepStructureCycledAtTheTop is `reach_test.go`'s deep tree with a role map loop and the root's own first
// element standing on it, so the cycle is among the elements nib READ and the unread tail is below it.
func deepStructureCycledAtTheTop(depth int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R 100 0 R] /RoleMap << /Alpha /Delta /Delta /Alpha >> >>",
		8: "<< /Type /StructElem /S /Alpha /P 7 0 R >>",
	}
	parent := 7
	for i := 0; i < depth; i++ {
		n := 100 + i
		kid := fmt.Sprintf("%d 0 R", n+1)
		if i == depth-1 {
			kid = "[20 0 R]"
		}
		objs[n] = fmt.Sprintf("<< /Type /StructElem /S /Div /P %d 0 R /K %s >>", parent, kid)
		parent = n
	}
	objs[20] = fmt.Sprintf("<< /Type /StructElem /S /P /P %d 0 R >>", parent)
	return buildPDF(objs)
}

// TestACycleAmongTheElementsNibReadIsSettledEvenWhenTheTailIsNot — the ORDER inside the rule, which is a
// decision and not an accident.
//
// `reach_test.go` holds the other half: with no cycle among the elements nib read, an unread tail is
// `CannotCheck`, because the element past the bound may be the one on the loop. The converse does not
// follow. A cycle nib has already seen is established, and an unread tail cannot un-establish it —
// answering `CannotCheck` there would aim law 4's third verdict at something nib did settle.
func TestACycleAmongTheElementsNibReadIsSettledEvenWhenTheTailIsNot(t *testing.T) {
	if got := verdictOf(t, deepStructureCycledAtTheTop(5), "7.1 t6"); got.Verdict != Fail {
		t.Fatalf("control: five Divs down, 7.1 t6 reports %v (%s) over a tree whose top element is on a loop, want Fail", got.Verdict, got.Why)
	}
	got := verdictOf(t, deepStructureCycledAtTheTop(70), "7.1 t6")
	if got.Verdict != Fail {
		t.Errorf("seventy Divs down, 7.1 t6 reports %v (%s) — the tail is unread, but the cycle at the top is "+
			"one nib established, and it stays established", got.Verdict, got.Why)
	}
}

// selfMapRoleMap types `/Alpha` as `H1` through a chain whose last name maps to ITSELF.
//
// It is `7.1 General/7.1-t06-fail-a.pdf`'s shape (`/RoleMap << /LI /LI >>`) with this file's names: the
// element resolves to a standard type AND the map is circular, which is the pair of facts a rule reading
// only `standardType`'s verdict cannot hold at once.
const selfMapRoleMap = "/Alpha /H1 /H1 /H1 /Bravo /Figure /Charlie /Table"

// TestASelfMappedTypeIsCircularAndStillTypes — measured on that corpus file: veraPDF fails ua1 7.1-6 on the
// two `LI` elements and passes the other twelve, so a self-map is a circular mapping. The typing is
// untouched, because a conforming reader recognises `LI` before it consults the map — and the second half of
// this test is what stops the first half being bought by breaking three shipped rules.
func TestASelfMappedTypeIsCircularAndStillTypes(t *testing.T) {
	pdf := roleMapDoc(selfMapRoleMap)
	got := verdictOf(t, pdf, "7.1 t6")
	if got.Verdict != Fail {
		t.Errorf("with /H1 mapped to itself, 7.1 t6 reports %v (%s), want Fail — a self-map is a circular mapping", got.Verdict, got.Why)
	} else if !strings.Contains(got.Why, "to itself") {
		t.Errorf("the reason %q does not say the role map sends the type to itself", got.Why)
	}
	// And the element still TYPES: 7.4.2 t1 places it as the document's H1 rather than answering
	// CannotCheck over an element it could not resolve.
	if h := verdictOf(t, pdf, "7.4.2 t1"); h.Verdict != Pass {
		t.Errorf("with /H1 mapped to itself, 7.4.2 t1 reports %v (%s), want Pass — the self-map resolves, and "+
			"deriving the cycle from the typing would have broken this", h.Verdict, h.Why)
	}
}

// TestADocumentWithNoStructureElementsHasNoSubjectFor7_1t6 — veraPDF reports 0 passed and 0 failed checks
// over such a document, which is `NotApplicable` and never `Pass`: the missing structure is 7.1 t11's.
func TestADocumentWithNoStructureElementsHasNoSubjectFor7_1t6(t *testing.T) {
	got := verdictOf(t, nestedForms(1), "7.1 t6")
	if got.Verdict != NotApplicable {
		t.Errorf("over a document with no structure tree at all, 7.1 t6 reports %v (%s), want NotApplicable", got.Verdict, got.Why)
	}
}

func buildPDF(objs map[int]string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offs := map[int]int{}
	maxN := 0
	for n := range objs {
		if n > maxN {
			maxN = n
		}
	}
	for n := 1; n <= maxN; n++ {
		body, ok := objs[n]
		if !ok {
			continue
		}
		offs[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", maxN+1)
	for n := 1; n <= maxN; n++ {
		if off, ok := offs[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", maxN+1, xref)
	return b.Bytes()
}

// TestNonStandardTypesAndRemapsAgreeWithWhatVeraPDFMeasured — P03.S01's ua1 7.1 t5 and t7, each row a
// `roleMapDoc` fixture veraPDF 1.30.2 was run on before the rule was written (the slice's ledger has the
// run). Pass and fail halves both, because a rule that answered Fail unconditionally would score
// perfectly on the corpus's fail files alone.
func TestNonStandardTypesAndRemapsAgreeWithWhatVeraPDFMeasured(t *testing.T) {
	for _, tc := range []struct {
		name    string
		roleMap string
		t5, t7  Verdict
	}{
		// Alpha, Bravo and Charlie map cleanly: nothing is untyped, so t5 has no subject.
		{"every private type mapped", resolvedRoleMap, NotApplicable, Pass},
		{"a private type dead-ending at another private one", "/Alpha /Zed /Bravo /Figure /Charlie /Table", Fail, Pass},
		{"a chain through a standard type stops there", "/Alpha /P /P /Zed /Bravo /Figure /Charlie /Table", NotApplicable, Pass},
		{"a standard type remapped onto another standard type", "/Alpha /H1 /Bravo /Figure /Charlie /Table /TR /TD", NotApplicable, Fail},
		// TR is on a loop, so it is untyped — t5's subject — and it passes t5: the loop is 7.1 t6's.
		{"a loop entered from a standard type", "/Alpha /H1 /Bravo /Figure /Charlie /Table /TR /Zed /Zed /TR", Pass, Fail},
		{"a standard type remapped that no element uses", "/Alpha /H1 /Bravo /Figure /Charlie /Table /Span /Zed", NotApplicable, Pass},
		// A private type mapped to itself is untyped and on a loop: t5's subject, passing (7.1 t6 fails it).
		{"a private type mapped to itself", "/Alpha /Alpha /Bravo /Figure /Charlie /Table", Pass, Pass},
		// A STANDARD type sent to a dead end is untyped too, and passes t5 — the corpus's /Document → /Book.
		{"a standard type sent to a dead end", "/Alpha /H1 /Bravo /Figure /Charlie /Table /TR /Zed", Pass, Fail},
	} {
		pdf := roleMapDoc(tc.roleMap)
		if got := verdictOf(t, pdf, "7.1 t5"); got.Verdict != tc.t5 {
			t.Errorf("%s: 7.1 t5 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t5)
		}
		if got := verdictOf(t, pdf, "7.1 t7"); got.Verdict != tc.t7 {
			t.Errorf("%s: 7.1 t7 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t7)
		}
	}
	// A self-map is not a remap (7.1-t06-fail-a passes 7.1-7). The fixture's /TR element is the type mapped
	// to itself — `selfMapRoleMap` maps /H1, which no element's /S names, so it could not tell.
	if got := verdictOf(t, roleMapDoc("/Alpha /Figure /Bravo /Figure /Charlie /Table /TR /TR"), "7.1 t7"); got.Verdict != Pass {
		t.Errorf("/TR mapped to itself: 7.1 t7 = %v (%s), want Pass — a self-map is 7.1 t6's, not a remap", got.Verdict, got.Why)
	}
	// No structure elements: no subject for either clause.
	for _, c := range []string{"7.1 t5", "7.1 t7"} {
		if got := verdictOf(t, nestedForms(1), c); got.Verdict != NotApplicable {
			t.Errorf("no structure elements: %s = %v (%s), want NotApplicable", c, got.Verdict, got.Why)
		}
	}
}

// TestTheTypingWalkAndTheCycleWalkAreDifferentQuestions — measured on veraPDF 1.30.2: `/Alpha → /H1` with
// `/H1 → /H1` FAILS 7.1-6 on `/Alpha` and types it `/H1` (7.4.2 t1 passes). A walk that stops at a
// recognised type answers the second; only a walk of the raw map answers the first.
func TestTheTypingWalkAndTheCycleWalkAreDifferentQuestions(t *testing.T) {
	pdf := roleMapDoc(selfMapRoleMap)
	if got := verdictOf(t, pdf, "7.4.2 t1"); got.Verdict != Pass {
		t.Errorf("7.4.2 t1 = %v (%s), want Pass: /Alpha types as /H1", got.Verdict, got.Why)
	}
	if got := verdictOf(t, pdf, "7.1 t6"); got.Verdict != Fail {
		t.Errorf("7.1 t6 = %v (%s), want Fail: /Alpha's mapping reaches a self-map", got.Verdict, got.Why)
	}
	// And a chain passing THROUGH a standard type types there, not at its end.
	if got := verdictOf(t, roleMapDoc("/Alpha /H1 /H1 /Zed /Bravo /Figure /Charlie /Table"), "7.4.2 t1"); got.Verdict != Pass {
		t.Errorf("/Alpha → /H1 → /Zed: 7.4.2 t1 = %v (%s), want Pass — the element is the document's H1", got.Verdict, got.Why)
	}
}

// TestAnElementsTypeDoesNotDependOnWhichElementWasTypedFirst — found by P03.S01's review, and proved by
// running it: with `/TR → /Zed → /TR`, the walk from /TR is a loop and the walk from /Zed stops at /TR, and
// the per-document memo gave whichever element came SECOND the first one's answer. Both orders, on fresh
// documents, must type each name the same way.
func TestAnElementsTypeDoesNotDependOnWhichElementWasTypedFirst(t *testing.T) {
	roleMap := "/Alpha /H1 /Bravo /Figure /Charlie /Table /TR /Zed /Zed /TR"
	elem := func(s string) types.Dict { return types.Dict{"S": types.Name(s)} }
	type answer struct{ std, unresolved string }
	resolve := func(order ...string) map[string]answer {
		d, err := open(roleMapDoc(roleMap))
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]answer{}
		for _, s := range order {
			std, un := d.standardType(elem(s))
			out[s] = answer{std, un}
		}
		return out
	}
	trFirst, zedFirst := resolve("TR", "Zed"), resolve("Zed", "TR")
	if trFirst["TR"].unresolved == "" {
		t.Errorf("/TR → /Zed → /TR types /TR as %q; it is on a loop and must be unresolved", trFirst["TR"].std)
	}
	if zedFirst["Zed"].std != "TR" {
		t.Errorf("/Zed → /TR types /Zed as %q (%s); the walk from /Zed reaches /TR by mapping and stops there", zedFirst["Zed"].std, zedFirst["Zed"].unresolved)
	}
	for _, s := range []string{"TR", "Zed"} {
		if trFirst[s] != zedFirst[s] {
			t.Errorf("/%s types as %+v when /TR is typed first and %+v when /Zed is — the answer depends on order", s, trFirst[s], zedFirst[s])
		}
	}
}
