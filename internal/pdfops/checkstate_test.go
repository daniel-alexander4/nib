package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// checkBoxPDF is one page with one check box "Answer" holding /V answer ("" for none) and a widget per on-state in
// states, each drawing that state and Off — the shape of an Acrobat Yes/No pair (test/ui/checkpair.test.mjs), or
// of a single box whose on-state is not Yes.
func checkBoxPDF(answer string, states ...string) []byte {
	form := func(s string) string {
		return fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 12 12] /Length %d >>\nstream\n%s\nendstream", len(s), s)
	}
	var objs []string
	kids := ""
	next := 6 // objects 1-5 are fixed below; each widget takes three (the widget, its on form, its off form)
	for i, s := range states {
		w := next + i*3
		kids += fmt.Sprintf("%d 0 R ", w)
		as := "/Off"
		if s == answer {
			as = "/" + s
		}
		objs = append(objs, fmt.Sprintf("<< /Type /Annot /Subtype /Widget /Parent 5 0 R /P 3 0 R /Rect [%d 90 %d 102] /F 4 /AS %s /AP << /N << /%s %d 0 R /Off %d 0 R >> >> >>",
			40+i*100, 52+i*100, as, s, w+1, w+2), form("0 g 2 2 8 8 re f"), form("0 G 0.5 0.5 11 11 re S"))
	}
	v := ""
	if answer != "" {
		v = "/V /" + answer
	}
	head := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Contents 4 0 R /Annots [" + kids + "] >>",
		"<< /Length 0 >>\nstream\n\nendstream",
		"<< /FT /Btn /T (Answer) " + v + " /Kids [" + kids + "] >>",
	}
	objs = append(head, objs...)
	out := "%PDF-1.7\n"
	var offsets []int
	for i, o := range objs {
		offsets = append(offsets, len(out))
		out += fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := len(out)
	out += fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offsets {
		out += fmt.Sprintf("%010d 00000 n \n", o)
	}
	out += fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return []byte(out)
}

func pairXFDF(name, value string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><xfdf xmlns="http://ns.adobe.com/xfdf/"><fields><field name="` + name + `"><value>` + value + `</value></field></fields></xfdf>`
}

// answerAndAS reads back what the file holds: the field's answer and each widget's /AS, in widget order.
func answerAndAS(t *testing.T, pdf []byte) (string, []string) {
	t.Helper()
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.EXPORTFORMFIELDS
	ctx, err := pdfread.ReadOptimized(pdf, conf)
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	fields, _ := formFields(xt)
	for _, f := range fields {
		if f.name != "Answer" {
			continue
		}
		var as []string
		for _, w := range checkWidgets(xt, f) {
			n, _ := resolved(xt, w["AS"]).(types.Name)
			as = append(as, n.Value())
		}
		return checkStatesOf(xt)["Answer"].Answer, as
	}
	t.Fatal("no field Answer")
	return "", nil
}

// TestAYesNoPairExportsWhichBoxAnswered — /pending 922. Under pdfcpu's boolean a pair answered No is "ticked",
// exactly as one answered Yes is.
func TestAYesNoPairExportsWhichBoxAnswered(t *testing.T) {
	for _, answer := range []string{"Yes", "No"} {
		pdf := checkBoxPDF(answer, "Yes", "No")
		x, err := ExportFormXFDF(pdf)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(x), "<value>"+answer+"</value>") {
			t.Errorf("answer %s: XFDF does not name it: %s", answer, x)
		}
		c, err := ExportFormCSV(pdf)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(c), "Answer,"+answer+"\n") {
			t.Errorf("answer %s: CSV does not name it: %s", answer, c)
		}
		j, err := ExportFormJSON(pdf)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(j), `"state": "`+answer+`"`) {
			t.Errorf("answer %s: JSON does not name it: %s", answer, j)
		}
	}
}

// A single box whose on-state is not Yes keeps its name; an unanswered pair exports as it did.
func TestACheckBoxExportsItsOwnOnStateName(t *testing.T) {
	x, err := ExportFormXFDF(checkBoxPDF("Ja", "Ja"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(x), "<value>Ja</value>") {
		t.Errorf("a box answered Ja is not exported as Ja: %s", x)
	}
	x, err = ExportFormXFDF(checkBoxPDF("", "Yes", "No"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(x), "<value>Off</value>") {
		t.Errorf("an unanswered pair is not Off: %s", x)
	}
	j, _ := ExportFormJSON(checkBoxPDF("", "Yes", "No"))
	if strings.Contains(string(j), `"state"`) {
		t.Errorf("an unanswered pair carries a state: %s", j)
	}
}

// TestAFillChoosesTheBoxOfAPairByName — the half of 922 that makes the export worth reading: with pdfcpu's boolean an
// edit from No to Yes did nothing, and a bare true onto an unanswered pair ticked both boxes.
func TestAFillChoosesTheBoxOfAPairByName(t *testing.T) {
	// No → Yes, by each of the three fills.
	pdf := checkBoxPDF("No", "Yes", "No")
	byXFDF, err := FillFormXFDF(pdf, []byte(pairXFDF("Answer", "Yes")))
	if err != nil {
		t.Fatalf("XFDF: %v", err)
	}
	j, _ := ExportFormJSON(pdf)
	byJSON, err := FillFormJSON(pdf, []byte(strings.Replace(string(j), `"state": "No"`, `"state": "Yes"`, 1)))
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	parts, err := FillFormCSV(pdf, []byte("Answer\nYes\n"), "")
	if err != nil {
		t.Fatalf("CSV: %v", err)
	}
	for name, out := range map[string][]byte{"XFDF": byXFDF, "JSON": byJSON, "CSV": parts[0].Data} {
		a, as := answerAndAS(t, out)
		if a != "Yes" || len(as) != 2 || as[0] != "Yes" || as[1] != "Off" {
			t.Errorf("%s: answer %q, /AS %v — want Yes with the second box Off", name, a, as)
		}
	}
	// From no answer to No: exactly one box ticked.
	out, err := FillFormXFDF(checkBoxPDF("", "Yes", "No"), []byte(pairXFDF("Answer", "No")))
	if err != nil {
		t.Fatal(err)
	}
	if a, as := answerAndAS(t, out); a != "No" || as[0] != "Off" || as[1] != "No" {
		t.Errorf("unanswered → No: answer %q, /AS %v", a, as)
	}
	// Off clears; the same answer again is nothing to do.
	out, err = FillFormXFDF(pdf, []byte(pairXFDF("Answer", "Off")))
	if err != nil {
		t.Fatal(err)
	}
	if a, as := answerAndAS(t, out); a != "" || as[0] != "Off" || as[1] != "Off" {
		t.Errorf("Off: answer %q, /AS %v", a, as)
	}
}

// A bare yes onto an unanswered pair is refused rather than ticking both; onto an answered one it keeps the answer.
func TestABareTrueCannotChooseBetweenTheBoxesOfAPair(t *testing.T) {
	_, err := FillFormCSV(checkBoxPDF("", "Yes", "No"), []byte("Answer\ntrue\n"), "")
	if err == nil || !strings.Contains(err.Error(), "several on-states") {
		t.Fatalf("a bare true onto an unanswered pair was not refused: %v", err)
	}
	out, err := FillFormJSON(checkBoxPDF("No", "Yes", "No"), []byte(`{"forms":[{"checkbox":[{"name":"Answer","value":true}]}]}`))
	if err == nil {
		if a, _ := answerAndAS(t, out); a != "No" {
			t.Errorf("a bare true changed the answer to %q", a)
		}
	}
}
