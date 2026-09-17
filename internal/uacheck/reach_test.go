package uacheck

import (
	"fmt"
	"strings"
	"testing"
)

// A rule never answers Pass or NotApplicable over content its reader did not reach — `/pending 496`,
// PLAN-ua-coverage law 1.
//
// Every walk in this package is bounded, because a document can make a form XObject, a structure tree or
// a number tree refer to itself. Until /pending 496 each bound simply RETURNED, and what lay past it read
// exactly like content that is not there: a heading that skipped a level under seventy `Div`s passed 7.4.2
// t1, and untagged text twelve forms down made 7.1 t3 not applicable. Each test below builds the same
// defect twice — once shallow, where the rule must SEE it (the stimulus), and once past the bound, where
// the rule must say it could not look.

// nestedForms is a one-page document whose page draws a form that draws a form, depth times, and whose
// innermost form draws untagged, non-embedded Helvetica text.
func nestedForms(depth int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	page := "/X Do"
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X 10 0 R >> >> /Contents 4 0 R >>"
	objs[4] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page)
	for i := 0; i < depth; i++ {
		n := 10 + i
		body, res := "/X Do", fmt.Sprintf("<< /XObject << /X %d 0 R >> >>", n+1)
		if i == depth-1 {
			body, res = "BT /F1 12 Tf 72 700 Td (deep) Tj ET", "<< /Font << /F1 5 0 R >> >>"
		}
		objs[n] = fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources %s /Length %d >>\nstream\n%s\nendstream", res, len(body), body)
	}
	return buildPDF(objs)
}

func TestContentPastTheFormBoundIsCannotCheckNeverAPass(t *testing.T) {
	// Stimulus: three forms down, the untagged text is walked and both rules find it.
	shallow := nestedForms(3)
	for _, clause := range []string{"7.1 t3", "7.21.4.1 t1"} {
		if got := verdictOf(t, shallow, clause); got.Verdict != Fail {
			t.Fatalf("control: untagged Helvetica three forms down reports %v for %s (%s), want Fail — the fixture does not carry the defect", got.Verdict, clause, got.Why)
		}
	}
	deep := nestedForms(12)
	for _, clause := range []string{"7.1 t3", "7.2 t34", "7.21.4.1 t1", "7.21.7 t1", "7.21.4.2 t2"} {
		got := verdictOf(t, deep, clause)
		if got.Verdict != CannotCheck {
			t.Errorf("twelve forms down, %s reports %v (%s) over text nib never walked, want CannotCheck", clause, got.Verdict, got.Why)
			continue
		}
		if !strings.Contains(got.Why, "deeper than") {
			t.Errorf("%s: the reason %q does not say the walk stopped at its depth bound", clause, got.Why)
		}
	}
}

// deepStructure is a tagged document whose root holds an H1, then a chain of depth nested `Div`s holding an
// H3 (a skipped level), a Figure with no alternate text, and a table whose one data cell names its headers.
func deepStructure(depth int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R 100 0 R] >>",
		8: "<< /Type /StructElem /S /H1 /P 7 0 R >>",
	}
	parent := 7
	for i := 0; i < depth; i++ {
		n := 100 + i
		kid := fmt.Sprintf("%d 0 R", n+1)
		if i == depth-1 {
			kid = "[20 0 R 21 0 R 22 0 R]"
		}
		objs[n] = fmt.Sprintf("<< /Type /StructElem /S /Div /P %d 0 R /K %s >>", parent, kid)
		parent = n
	}
	objs[20] = fmt.Sprintf("<< /Type /StructElem /S /H3 /P %d 0 R >>", parent)
	objs[21] = fmt.Sprintf("<< /Type /StructElem /S /Figure /P %d 0 R >>", parent)
	objs[22] = fmt.Sprintf("<< /Type /StructElem /S /Table /P %d 0 R /K 23 0 R >>", parent)
	objs[23] = "<< /Type /StructElem /S /TR /P 22 0 R /K 24 0 R >>"
	objs[24] = "<< /Type /StructElem /S /TD /P 23 0 R /A << /O /Table /Headers [(h1)] >> >>"
	return buildPDF(objs)
}

func TestStructurePastTheTreeBoundIsCannotCheckNeverAPass(t *testing.T) {
	shallow := deepStructure(5)
	// `7.1 t6` joins the list with `/pending 548`: an element past the bound may be the one standing on a
	// circular role mapping, so "no cycle among the elements nib read" is not "no cycle".
	for clause, want := range map[string]Verdict{"7.4.2 t1": Fail, "7.3 t1": Fail, "7.5 t1": Pass, "7.1 t6": Pass} {
		if got := verdictOf(t, shallow, clause); got.Verdict != want {
			t.Fatalf("control: five Divs down, %s reports %v (%s), want %v — the fixture does not carry its subject", clause, got.Verdict, got.Why, want)
		}
	}
	deep := deepStructure(70)
	for _, clause := range []string{"7.4.2 t1", "7.3 t1", "7.5 t1", "7.1 t6"} {
		got := verdictOf(t, deep, clause)
		if got.Verdict != CannotCheck {
			t.Errorf("seventy Divs down, %s reports %v (%s) over elements nib never read, want CannotCheck", clause, got.Verdict, got.Why)
			continue
		}
		if !strings.Contains(got.Why, "deeper than") {
			t.Errorf("%s: the reason %q does not say the walk stopped at its depth bound", clause, got.Why)
		}
	}
}

// deepParentTree is a document with no catalog /Lang whose one text run (MCID 0) belongs to a P declaring
// /Lang, and whose one widget belongs to a Form element — both reached through a parent tree whose /Nums
// sit depth /Kids levels down.
func deepParentTree(depth int) []byte {
	content := "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Annots [30 0 R] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] /ParentTree 200 0 R >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Lang (en) /Pg 3 0 R /K 0 >>",
		9:  "<< /Type /StructElem /S /Form /P 7 0 R /K << /Type /OBJR /Obj 30 0 R >> >>",
		30: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /StructParent 1 >>",
	}
	for i := 0; i < depth; i++ {
		objs[200+i] = fmt.Sprintf("<< /Kids [%d 0 R] /Limits [0 1] >>", 201+i)
	}
	objs[200+depth] = "<< /Nums [0 [8 0 R] 1 9 0 R] /Limits [0 1] >>"
	return buildPDF(objs)
}

func TestAParentTreePastItsBoundIsCannotCheckNeverAFalseAnswer(t *testing.T) {
	shallow := deepParentTree(3)
	for _, clause := range []string{"7.2 t34", "7.18.4 t1"} {
		if got := verdictOf(t, shallow, clause); got.Verdict != Pass {
			t.Fatalf("control: a parent tree three levels deep reports %v for %s (%s), want Pass — the fixture does not resolve", got.Verdict, clause, got.Why)
		}
	}
	deep := deepParentTree(70)
	for _, clause := range []string{"7.2 t34", "7.18.4 t1"} {
		got := verdictOf(t, deep, clause)
		if got.Verdict != CannotCheck {
			t.Errorf("a parent tree seventy levels deep reports %v for %s (%s) over keys nib never read, want CannotCheck", got.Verdict, clause, got.Why)
			continue
		}
		if !strings.Contains(got.Why, "deeper than") {
			t.Errorf("%s: the reason %q does not say the walk stopped at its depth bound", clause, got.Why)
		}
	}
}

// deepLanguage is a document with no catalog /Lang whose text belongs to a P at the bottom of depth nested
// `Div`s, and only the top Div declares /Lang.
func deepLanguage(depth int) []byte {
	content := "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Nums [0 [20 0 R]] >>",
		7: "<< /Type /StructTreeRoot /K 100 0 R /ParentTree 6 0 R >>",
	}
	parent := 7
	for i := 0; i < depth; i++ {
		n := 100 + i
		lang := ""
		if i == 0 {
			lang = " /Lang (en)"
		}
		kid := fmt.Sprintf("%d 0 R", n+1)
		if i == depth-1 {
			kid = "20 0 R"
		}
		objs[n] = fmt.Sprintf("<< /Type /StructElem /S /Div /P %d 0 R%s /K %s >>", parent, lang, kid)
		parent = n
	}
	objs[20] = fmt.Sprintf("<< /Type /StructElem /S /P /P %d 0 R /Pg 3 0 R /K 0 >>", parent)
	return buildPDF(objs)
}

func TestALanguageInheritedFromPastTheBoundIsCannotCheckNeverAFail(t *testing.T) {
	if got := verdictOf(t, deepLanguage(3), "7.2 t34"); got.Verdict != Pass {
		t.Fatalf("control: /Lang three ancestors up reports %v for 7.2 t34 (%s), want Pass", got.Verdict, got.Why)
	}
	got := verdictOf(t, deepLanguage(70), "7.2 t34")
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "deeper than") {
		t.Errorf("/Lang seventy ancestors up reports %v for 7.2 t34 (%s), want CannotCheck naming the bound", got.Verdict, got.Why)
	}
}
