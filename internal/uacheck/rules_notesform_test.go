package uacheck

import (
	"fmt"
	"testing"
)

// P03.S06 — every document below was run through veraPDF 1.30.2 before it was pinned; each verdict is veraPDF's.

func TestNoteIDsAgreeWithVeraPDF(t *testing.T) {
	for _, tc := range []struct {
		spec   string
		t1, t2 Verdict
	}{
		{"Document(Note!id=n1,Note!id=n2)", Pass, Pass},
		{"Document(Note,P)", Fail, Pass},
		// The set is the whole document's: a duplicate one section down still collides.
		{"Document(Note!id=a,Sect(Note!id=a))", Pass, Fail},
		// An empty ID is no ID (t1) and is still an ID to the set, so two collide (t2).
		{"Document(Note!id=,Note!id=)", Fail, Fail},
		{"Document(P)", NotApplicable, NotApplicable},
	} {
		if got := verdictOf(t, treeDoc("", tc.spec), "7.9 t1"); got.Verdict != tc.t1 {
			t.Errorf("7.9 t1 over %s = %v (%s), want %v", tc.spec, got.Verdict, got.Why, tc.t1)
		}
		if got := verdictOf(t, treeDoc("", tc.spec), "7.9 t2"); got.Verdict != tc.t2 {
			t.Errorf("7.9 t2 over %s = %v (%s), want %v", tc.spec, got.Verdict, got.Why, tc.t2)
		}
	}
}

// formChildDoc is one page, one annotation (obj 9), and a Form element (obj 21) whose /K is k and whose extra
// entries are attrs. A widget carries the field keys pdfcpu requires; a Text note carries none — pdfcpu's reader
// rewrites an annotation with field keys to /Subtype /Widget, so a "Text" field would never reach the rule.
func formChildDoc(widget bool, k, attrs string) []byte {
	c := "/Span << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (a) Tj ET EMC\n"
	annot := "<< /Type /Annot /Subtype /Text /Contents (a note) /Rect [72 600 92 620] /StructParent 1 /P 3 0 R /F 4 >>"
	form := ""
	if widget {
		annot = "<< /Type /Annot /Subtype /Widget /FT /Tx /DA (/Helv 0 Tf 0 g) /T (f) /TU (A field) /Rect [72 600 200 620] /StructParent 1 /P 3 0 R /F 4 >>"
		form = " /AcroForm << /Fields [9 0 R] >>"
	}
	return buildPDF(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /ViewerPreferences << /DisplayDocTitle true >>" + form + " >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Tabs /S /Annots [9 0 R] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [20 0 R] /ParentTree 8 0 R >>",
		8:  "<< /Nums [0 [21 0 R] 1 21 0 R] >>",
		9:  annot,
		20: "<< /Type /StructElem /S /Document /P 7 0 R /K [21 0 R] >>",
		21: fmt.Sprintf("<< /Type /StructElem /S /Form /P 20 0 R /Pg 3 0 R /K %s %s >>", k, attrs),
	})
}

func TestAFormsOneChildAgreesWithVeraPDF(t *testing.T) {
	const objr = "<< /Type /OBJR /Obj 9 0 R >>"
	for _, tc := range []struct {
		name   string
		widget bool
		k      string
		attrs  string
		want   Verdict
	}{
		{"one object reference to its widget", true, "[" + objr + "]", "", Pass},
		{"the same reference written bare, not in an array", true, objr, "", Pass},
		{"an MCID beside the widget's reference is a second child", true, "[" + objr + " 0]", "", Fail},
		{"a bare MCID alone is not an object reference", true, "[0]", "", Fail},
		{"a Role attribute excuses the rule", true, "[" + objr + " 0]", "/A << /O /PrintField /Role /tv >>", Pass},
		{"a reference to a note is not a widget", false, "[" + objr + "]", "", Fail},
	} {
		if got := verdictOf(t, formChildDoc(tc.widget, tc.k, tc.attrs), "7.18.4 t2"); got.Verdict != tc.want {
			t.Errorf("%s: 7.18.4 t2 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.want)
		}
	}
}
