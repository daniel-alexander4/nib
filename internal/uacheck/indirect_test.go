package uacheck

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Indirect values, the single-element /K, and a verdict outside the enum — `/pending 496`.
//
// PDF lets almost any value be an indirect object (`/DisplayDocTitle 9 0 R` with `9 0 obj true`). A reader
// that casts the dictionary entry straight to `types.Boolean` reads that as the wrong type, and depending on
// the rule that is a false Fail (the clause's value unread) or a false Pass (a Figure whose `/S` was not
// read is not a Figure, so its missing alternate text is never asked about).

// indirectDoc is a tagged one-page document with each value the rules read stored as an indirect object.
func indirectDoc() []byte {
	content := "/P /MC0 BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"
	return buildPDF(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /ViewerPreferences << /DisplayDocTitle 40 0 R >> /OCProperties << /OCGs [50 0 R] /D << /Name 41 0 R /ON [50 0 R] >> >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 42 0 R /Annots [30 0 R] /Resources << /Font << /F1 5 0 R >> /Properties << /MC0 << /MCID 43 0 R >> >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /Nums [0 [8 0 R] 44 0 R 9 0 R] >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 9 0 R 10 0 R 11 0 R] /ParentTree 6 0 R /RoleMap << /Deep 45 0 R >> >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Lang (en) /Pg 3 0 R /K 0 >>",
		9:  "<< /Type /StructElem /S /Form /P 7 0 R /K << /Type /OBJR /Obj 30 0 R >> >>",
		10: "<< /Type /StructElem /S /H1 /P 7 0 R >>",
		11: "<< /Type /StructElem /S /Deep /P 7 0 R >>",
		30: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /StructParent 47 0 R >>",
		40: "true",
		41: "(layer)",
		42: "0",
		43: "0",
		44: "1",
		45: "/H3",
		47: "1",
		50: "<< /Type /OCG /Name (layer) >>",
	})
}

func TestTheRulesReadIndirectValues(t *testing.T) {
	pdf := indirectDoc()
	for _, c := range []struct {
		clause, why string
		want        Verdict
	}{
		{"7.1 t10", "/DisplayDocTitle 40 0 R → true", Pass},
		{"7.10 t1", "/Name 41 0 R → (layer)", Pass},
		{"7.1 t3", "/Properties /MC0 << /MCID 43 0 R >>", Pass},
		{"7.2 t34", "/StructParents 42 0 R resolves MCID 0 to a P with /Lang", Pass},
		{"7.18.4 t1", "/StructParent 47 0 R and the /Nums key 44 0 R", Pass},
		{"7.4.2 t1", "/RoleMap /Deep 45 0 R → /H3, after an H1", Fail},
	} {
		if got := verdictOf(t, pdf, c.clause); got.Verdict != c.want {
			t.Errorf("%s (%s) reports %v (%s), want %v", c.clause, c.why, got.Verdict, got.Why, c.want)
		}
	}
}

// TestAnIndirectStructureTypeIsStillAFigure — the false-Pass direction, in its own tree so no other element
// can supply the Figure.
func TestAnIndirectStructureTypeIsStillAFigure(t *testing.T) {
	pdf := buildPDF(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		7:  "<< /Type /StructTreeRoot /K [12 0 R] >>",
		12: "<< /Type /StructElem /S 46 0 R /P 7 0 R >>",
		46: "/Figure",
	})
	if got := verdictOf(t, pdf, "7.3 t1"); got.Verdict != Fail {
		t.Errorf("a Figure named through /S 46 0 R with no /Alt reports %v for 7.3 t1 (%s), want Fail", got.Verdict, got.Why)
	}
}

// TestASingleElementKIsAStructureRoot — `/K` may be one element rather than an array of them.
func TestASingleElementKIsAStructureRoot(t *testing.T) {
	for name, k := range map[string]string{"indirect": "10 0 R", "inline": "<< /Type /StructElem /S /P >>"} {
		pdf := buildPDF(map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
			7:  "<< /Type /StructTreeRoot /K " + k + " >>",
			10: "<< /Type /StructElem /S /P /P 7 0 R >>",
		})
		if got := verdictOf(t, pdf, "7.1 t11"); got.Verdict != Pass {
			t.Errorf("%s: a root whose /K is one element reports %v for 7.1 t11 (%s), want Pass", name, got.Verdict, got.Why)
		}
	}
}

// TestAVerdictOutsideTheEnumIsNeverSilence — an out-of-range verdict is neither a failure nor unresolved to
// a caller asking only those two questions, so the door's refusal list came back empty and `nib ua` printed
// "every clause nib checks passes" over a report `Conformant()` called non-conformant.
func TestAVerdictOutsideTheEnumIsNeverSilence(t *testing.T) {
	rogue := Rule{Clause: "9.9 t9", Summary: "x", Check: func(*Document) Result { return Result{Verdict: Verdict(42), Why: "rogue"} }}
	if got := runOne(rogue, &Document{}); got.Verdict != NotRun || !strings.Contains(got.Why, "42") {
		t.Errorf("a rule returning Verdict(42) is reported as %v (%q), want NotRun naming the value", got.Verdict, got.Why)
	}
	rep := Report{Results: []Result{{Clause: "9.9 t9", Verdict: Verdict(42)}}}
	if rep.Conformant() {
		t.Fatal("stimulus: a report holding Verdict(42) is conformant — the enum's conformance test changed")
	}
	if len(Refusals(rep)) == 0 {
		t.Error("a non-conformant report produced no refusal, so every surface reading the refusal list says every clause passes")
	}
}

// TestTheRulesReadTypedValuesOnlyThroughTheDoor — ADR-009: the reader that resolves an indirect value is
// written once in document.go, and this asserts routing through it. A new direct read in a rule file is the
// defect these tests found six copies of.
func TestTheRulesReadTypedValuesOnlyThroughTheDoor(t *testing.T) {
	direct := regexp.MustCompile(`\.(NameEntry|IntEntry|BooleanEntry|StringEntry|StringLiteralEntry|Int64Entry|NumberEntry)\(|\.\(types\.(Integer|Name|Boolean|StringLiteral|HexLiteral|Float)\)`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "document.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for i, line := range strings.Split(string(src), "\n") {
			if direct.MatchString(line) {
				t.Errorf("%s:%d reads a typed value directly, which misreads an indirect one — use the Document door: %s", name, i+1, strings.TrimSpace(line))
			}
		}
	}
	if checked < 10 {
		t.Fatalf("the guard read %d source file(s) — it is not looking at the package", checked)
	}
}
