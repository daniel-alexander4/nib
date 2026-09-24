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

// notTreeRules are the clauses that never read the structure tree's elements, each with what it reads instead.
// Every OTHER registered clause is driven past the tree's bound below, so a rule added later is covered by
// construction — the four-clause list above had gone a whole phase without any of P03's thirty-eight tree
// rules on it (P03's phase-close review removed the `unread` guard from the containment and kid-sequence
// doors and the package stayed green).
var notTreeRules = map[string]string{
	"5 t1": "the XMP packet", "5 t2": "the XMP packet", "7.1 t8": "the XMP packet", "7.1 t9": "the XMP packet",
	"7.2 t33": "the XMP packet", "7.1 t10": "the catalog's /ViewerPreferences", "7.1 t11": "the catalog's /StructTreeRoot and /MarkInfo",
	"7.2 t2": "the catalog's /Outlines and /Lang", "7.10 t1": "optional content", "7.10 t2": "optional content",
	"6.2 t1": "page content", "7.1 t3": "page content", "7.2 t34": "page content and the parent tree",
	"7.18.4 t1": "widgets and the parent tree", "7.21.4.1 t1": "fonts", "7.21.4.2 t2": "fonts", "7.21.7 t1": "fonts",
	// P04.S03. Their subjects are annotations and form fields, not structure elements: veraPDF runs t24
	// once per annotation and t25 once per form field, and each reaches a structure element only through
	// the holder's `/StructParent` and the parent tree — the same shape as `7.18.4 t1` above. A document
	// seventy Divs deep with no annotation and no field has no subject, so NotApplicable is the honest
	// answer and CannotCheck would be a refusal over a question nothing asked. They ARE CannotCheck when
	// the PARENT tree is the thing nib could not finish, which is a different bound and its own test.
	"7.2 t24": "annotations and the parent tree", "7.2 t25": "form fields and the parent tree",
	// P04.S04. Their subject is a marked-content SEQUENCE, not a structure element: a document seventy
	// Divs deep that draws nothing has no sequence, so there is nothing to refuse over. They reach the
	// tree only to settle `isTaggedContent` and `inheritedLang` for a sequence that exists, and both of
	// those are CannotCheck when the climb runs out — a different bound, with its own test.
	"7.1 t1": "marked-content sequences", "7.1 t2": "marked-content sequences",
	// P05.S01. Their subject is an ANNOTATION — the same shape as `7.18.4 t1` and `7.2 t24` above. A
	// document seventy Divs deep with no annotation has no subject at all, so NotApplicable is the
	// honest answer; they ARE CannotCheck when the PARENT tree is what nib could not finish, which is a
	// different bound with its own row in the table test.
	"7.18.1 t1": "annotations and the parent tree", "7.18.1 t2": "annotations and the parent tree",
	// P05.S02. Same shape: the subject is an annotation of one subtype. 7.18.2 t1 and 7.18.5 t2 do not read
	// the tree AT ALL — one refuses a subtype and the other reads the annotation's own `/Contents`.
	"7.18.5 t1": "link annotations and the parent tree", "7.18.5 t2": "a link annotation's own /Contents",
	"7.18.2 t1": "annotation subtypes", "7.18.8 t1": "printer's marks and the parent tree",
	// P05.S03. 7.18.1 t3's subject is a widget; `7.18.3 t1`'s is a PAGE and it reads no structure at all —
	// only the page's `/Tabs` and whether the page carries an annotation.
	"7.18.1 t3": "widget annotations, their field's /TU and the parent tree", "7.18.3 t1": "a page's /Tabs",
	"7.2 t30": "marked-content sequences", "7.2 t31": "marked-content sequences",
	"7.2 t32": "marked-content sequences",
}

func TestEveryTreeRuleIsCannotCheckPastTheTreeBound(t *testing.T) {
	deep := deepStructure(70)
	tree := 0
	for _, clause := range Clauses() {
		if _, exempt := notTreeRules[clause]; exempt {
			continue
		}
		tree++
		if got := verdictOf(t, deep, clause); got.Verdict != CannotCheck {
			t.Errorf("seventy Divs down, %s reports %v (%s) over elements nib never read, want CannotCheck", clause, got.Verdict, got.Why)
		}
	}
	// The stimulus before the response: an exemption list that swallowed the registry would pass vacuously.
	if tree < 40 {
		t.Fatalf("only %d clauses were driven past the bound; the exemption list has swallowed the tree rules", tree)
	}
	for clause := range notTreeRules {
		if !contains(Clauses(), clause) {
			t.Errorf("notTreeRules exempts %s, which is not registered", clause)
		}
	}
}

// deepParentTree is a document with no catalog /Lang whose one text run (MCID 0) belongs to a P declaring
// /Lang, and whose one widget belongs to a Form element — both reached through a parent tree whose /Nums
// sit depth /Kids levels down.
func deepParentTree(depth int) []byte {
	content := "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Annots [30 0 R 31 0 R 32 0 R 33 0 R] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R 9 0 R 10 0 R 11 0 R 12 0 R] /ParentTree 200 0 R >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Lang (en) /Pg 3 0 R /K 0 >>",
		// P04.S03: the Form element declares its own `/Lang` and the widget carries `/Contents`, so
		// 7.2 t24 is a SUBJECT here and passes while the tree resolves — which is what makes its
		// CannotCheck past the bound a change of answer rather than the same answer twice.
		9:  "<< /Type /StructElem /S /Form /P 7 0 R /Lang (en) /K << /Type /OBJR /Obj 30 0 R >> >>",
		30: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /StructParent 1 /Contents (a note) >>",
		// P05.S01: a NON-widget annotation, because 7.18.1 t1 and t2 exclude a widget — with only object
		// 30 on the page the two clauses had no subject here and could not have been on the list below.
		// It names key 2, an element whose tag IS `Annot`, so both clauses PASS while the tree resolves.
		// **No `/Contents`**: with one, 7.18.1 t2 answers from the annotation itself and never consults the
		// tree, so it would report a definite Pass past the bound rather than a refusal — a Pass that is
		// correct, and therefore a row that would prove nothing. Without it, t2 must reach element 10's
		// `/Alt` through the parent tree, which is the read the bound stops.
		31: "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /F 4 /StructParent 2 >>",
		// Its own `/Lang`, for the same reason element 9 carries one: without it 7.2 t24 fails the control.
		10: "<< /Type /StructElem /S /Annot /P 7 0 R /Lang (en) /Alt (a described note) /K << /Type /OBJR /Obj 31 0 R >> >>",
		// P05.S02: a Link and a PrinterMark, because 7.18.5 t1 and 7.18.8 t1 read the parent tree and both
		// were exempted from the tree-bound guard on the promise of a row here. The link names key 3 (a Link
		// element, so it PASSES while the tree resolves) and the mark names key 4 — in the tree, so it FAILS
		// while the tree resolves, which makes its CannotCheck past the bound a change of answer rather than
		// the same answer twice.
		32: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /F 4 /StructParent 3 /Contents (a link) >>",
		33: "<< /Type /Annot /Subtype /PrinterMark /Rect [0 0 10 10] /F 4 /StructParent 4 /Contents (a mark) /AP << /N 34 0 R >> >>",
		34: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream",
		11: "<< /Type /StructElem /S /Link /P 7 0 R /Lang (en) /K << /Type /OBJR /Obj 32 0 R >> >>",
		12: "<< /Type /StructElem /S /P /P 7 0 R /Lang (en) /K << /Type /OBJR /Obj 33 0 R >> >>",
	}
	for i := 0; i < depth; i++ {
		objs[200+i] = fmt.Sprintf("<< /Kids [%d 0 R] /Limits [0 4] >>", 201+i)
	}
	objs[200+depth] = "<< /Nums [0 [8 0 R] 1 9 0 R 2 10 0 R 3 11 0 R 4 12 0 R] /Limits [0 4] >>"
	return buildPDF(objs)
}

func TestAParentTreePastItsBoundIsCannotCheckNeverAFalseAnswer(t *testing.T) {
	shallow := deepParentTree(3)
	for _, clause := range []string{"7.2 t34", "7.18.4 t1", "7.2 t24", "7.18.1 t1", "7.18.1 t2", "7.18.5 t1"} {
		if got := verdictOf(t, shallow, clause); got.Verdict != Pass {
			t.Fatalf("control: a parent tree three levels deep reports %v for %s (%s), want Pass — the fixture does not resolve", got.Verdict, clause, got.Why)
		}
	}
	deep := deepParentTree(70)
	for _, clause := range []string{"7.2 t34", "7.18.4 t1", "7.2 t24", "7.18.1 t1", "7.18.1 t2", "7.18.5 t1"} {
		got := verdictOf(t, deep, clause)
		if got.Verdict != CannotCheck {
			t.Errorf("a parent tree seventy levels deep reports %v for %s (%s) over keys nib never read, want CannotCheck", got.Verdict, clause, got.Why)
			continue
		}
		if !strings.Contains(got.Why, "deeper than") {
			t.Errorf("%s: the reason %q does not say the walk stopped at its depth bound", clause, got.Why)
		}
	}
	// **`7.18.8 t1` cannot ride the loop above**, because its control is a FAIL rather than a Pass: the
	// fixture's printer's mark IS in the tree, which is what that clause refuses. So it is asserted in both
	// directions here — definite while the tree resolves, a refusal once it does not. Without this, mutating
	// its `unread` branch to `continue` left the whole package green (measured) and the clause reported
	// "none of the document's printer's marks is in the structure tree" about a tree nib never finished.
	// `7.18.1 t3` is the same shape as 7.18.8 t1 here: the fixture's widget carries neither a field `/TU` nor
	// an element `/Alt`, so it is a definite FAIL while the tree resolves and must become a refusal once the
	// walk stops — a Pass there would be "this widget is described" over keys nib never read.
	for _, clause := range []string{"7.18.8 t1", "7.18.1 t3"} {
		if got := verdictOf(t, shallow, clause); got.Verdict != Fail {
			t.Errorf("control: a parent tree three levels deep reports %v for %s (%s), want Fail", got.Verdict, clause, got.Why)
		}
		if got := verdictOf(t, deep, clause); got.Verdict != CannotCheck {
			t.Errorf("a parent tree seventy levels deep reports %v for %s (%s), want CannotCheck — a tree nib "+
				"never read is not a document with nothing in it", got.Verdict, clause, got.Why)
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
	// The wording is `parentLang`'s since P04.S04 — t34 climbs through `inheritedLangOf` now, and the
	// climb that said "deeper than" was the second implementation this slice deleted (`/pending 635`).
	got := verdictOf(t, deepLanguage(70), "7.2 t34")
	if got.Verdict != CannotCheck || got.Why != langClimbTooFar {
		t.Errorf("/Lang seventy ancestors up reports %v for 7.2 t34 (%s), want CannotCheck naming the bound", got.Verdict, got.Why)
	}
}
