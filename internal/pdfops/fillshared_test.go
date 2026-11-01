package pdfops

import (
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// twoParentsOneLeaf is a form whose two text fields share the leaf name "c1" under different parents —
// the IRS 1040's `c1_8[0]` sits under two parents — with a static XFA packet and NeedAppearances set, as a
// hybrid IRS form is.
func twoParentsOneLeaf() []byte {
	return testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R 7 0 R] /XFA [(template) 9 0 R] /DA (/Helv 10 Tf 0 g) /DR << /Font << /Helv 5 0 R >> >> >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [8 0 R 10 0 R] >>",
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /T (a) /Kids [8 0 R] >>",
		7:  "<< /T (b) /Kids [10 0 R] >>",
		8:  "<< /Type /Annot /Subtype /Widget /FT /Tx /DA (/Helv 10 Tf 0 g) /T (c1) /Parent 6 0 R /Rect [20 200 120 220] /P 3 0 R >>",
		9:  "<< /Length 24 >>\nstream\n<xdp:xdp xmlns:xdp=\"x\"/>\nendstream",
		10: "<< /Type /Annot /Subtype /Widget /FT /Tx /DA (/Helv 10 Tf 0 g) /T (c1) /Parent 7 0 R /Rect [20 100 120 120] /P 3 0 R >>",
	})
}

// TestAFillByFullPathWritesOnlyThatFieldAndLeavesNoXFAOrNeedAppearances — the two things a program that
// fills hybrid IRS forms relies on (RelaxTax handoff, N1): a leaf name two fields share is addressed by
// its full path alone, and the written form has no `/XFA` for a reader to prefer over the widgets, and a fill adds
// no `/NeedAppearances` asking it to redraw them.
func TestAFillByFullPathWritesOnlyThatFieldAndLeavesNoXFAOrNeedAppearances(t *testing.T) {
	in := twoParentsOneLeaf()
	if got, err := pdfread.ReadXFA(in); err != nil || got != pdfread.XFAStatic {
		t.Fatalf("the fixture's XFA reads %q, %v: the test reaches nothing", got, err)
	}
	out, err := FillFormJSON(in, []byte(`{"forms":[{"textfield":[{"name":"a.c1","value":"MARK-A"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	fields, err := ListFormFields(out)
	if err != nil {
		t.Fatal(err)
	}
	exp, err := ExportFormJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(exp), "MARK-A"); n != 1 {
		t.Fatalf("the value appears in %d fields of %v, want exactly one:\n%s", n, fields, exp)
	}
	at := strings.Index(string(exp), "MARK-A")
	if !strings.Contains(string(exp)[max(0, at-120):at], "a.c1") {
		t.Errorf("the value is not on a.c1:\n%s", exp)
	}
	if got, err := pdfread.ReadXFA(out); err != nil || got != "" {
		t.Errorf("the filled form still reads XFA %q (%v)", got, err)
	}
	ctx, err := pdfread.ReadForInspection(out)
	if err != nil {
		t.Fatal(err)
	}
	af, _ := ctx.DereferenceDict(ctx.RootDict["AcroForm"])
	if _, has := af["XFA"]; has {
		t.Error("the AcroForm still carries /XFA")
	}
	if v, has := af["NeedAppearances"]; has {
		t.Errorf("the AcroForm still carries /NeedAppearances %v", v)
	}
}
