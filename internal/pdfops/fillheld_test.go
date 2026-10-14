package pdfops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"

	"nib/internal/testpdf"
)

// `/pending 888`: every fill but a hand-written JSON one lays the supplied values over the form's own export, so a
// field the data never names arrives at the option check holding what the document holds. A real form had a
// choice field whose own value was none of its 80 options, and every fill of it was refused over that field.

// strayChoiceForm is a form with a text field and a choice field ("colour": red or green) that HOLDS "blue".
func strayChoiceForm() []byte {
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R 6 0 R] /DA (/Helv 0 Tf 0 g) /DR << /Font << /Helv 7 0 R >> >> >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Annots [5 0 R 6 0 R] /Resources << /Font << /Helv 7 0 R >> >> >>",
		4: "<< /Length 12 >>\nstream\n0 0 1 1 re f\nendstream",
		5: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /V () /Rect [72 700 300 720] /F 4 /P 3 0 R /DA (/Helv 12 Tf 0 g) >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Ch /Ff 131072 /T (colour) /V (blue) /Opt [(red) (green)] /Rect [72 660 300 680] /F 4 /P 3 0 R /DA (/Helv 12 Tf 0 g) >>",
		7: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	})
}

func xfdfOf(fields map[string]string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><xfdf xmlns="http://ns.adobe.com/xfdf/" xml:space="preserve"><fields>`)
	for name, v := range fields {
		b.WriteString(`<field name="` + name + `"><value>` + v + `</value></field>`)
	}
	b.WriteString(`</fields></xfdf>`)
	return []byte(b.String())
}

func TestAFillIsNotRefusedOverAValueItDoesNotChange(t *testing.T) {
	pdf := strayChoiceForm()
	// Stimulus: the document does hold a value its field does not offer, and the export reads it.
	before, err := ExportFormXFDF(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "blue") {
		t.Fatalf("setup: the export does not read the choice field's stray value:\n%s", before)
	}

	// By XFDF, naming only the text field.
	out, err := FillFormXFDF(pdf, xfdfOf(map[string]string{"name": "Ada"}))
	if err != nil {
		t.Fatalf("a fill of the text field alone was refused: %v", err)
	}
	after, err := ExportFormXFDF(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "Ada") {
		t.Errorf("the text field was not filled:\n%s", after)
	}
	if !strings.Contains(string(after), "blue") {
		t.Errorf("the choice field no longer holds what it held:\n%s", after)
	}

	// By JSON: the export with only the text field's value changed.
	j, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), `"blue"`) {
		t.Fatalf("setup: the JSON export does not carry the stray value:\n%s", j)
	}
	var fg form.FormGroup
	if err := json.Unmarshal(j, &fg); err != nil {
		t.Fatal(err)
	}
	fg.Forms[0].TextFields[0].Value = "Ada"
	edited, err := json.Marshal(fg)
	if err != nil {
		t.Fatal(err)
	}
	out, err = FillFormJSON(pdf, edited)
	if err != nil {
		t.Fatalf("the document's own export with the text field changed was refused as a fill: %v", err)
	}
	if after, _ := ExportFormXFDF(out); !strings.Contains(string(after), "Ada") || !strings.Contains(string(after), "blue") {
		t.Errorf("after the JSON fill the form does not read the new text and the old choice:\n%s", after)
	}

	// A value the fill CHANGES to something the field does not offer is still refused, by name.
	_, err = FillFormXFDF(pdf, xfdfOf(map[string]string{"colour": "purple"}))
	if err == nil || !strings.Contains(err.Error(), "colour") || !strings.Contains(err.Error(), "purple") {
		t.Errorf("changing the choice to a value it does not offer: err = %v, want a refusal naming the field and the value", err)
	}
	if _, err := FillFormJSON(pdf, []byte(strings.Replace(string(edited), `"blue"`, `"purple"`, 1))); err == nil {
		t.Error("a JSON fill changing the choice to a value it does not offer was accepted")
	}
	// And one it does offer is taken.
	out, err = FillFormXFDF(pdf, xfdfOf(map[string]string{"colour": "green"}))
	if err != nil {
		t.Fatalf("changing the choice to a value it offers was refused: %v", err)
	}
	if after, _ := ExportFormXFDF(out); !strings.Contains(string(after), "green") || strings.Contains(string(after), "blue") {
		t.Errorf("the choice field was not changed to the offered value:\n%s", after)
	}
}
