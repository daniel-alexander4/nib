package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// repeatedFieldForm is a two-page form with two fields that are each shown on both pages — one field, two
// widgets — whose producer also wrote a copy of the value on every widget, as some do.
func repeatedFieldForm() []byte {
	return testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R 9 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [7 0 R 10 0 R] >>",
		4:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [8 0 R 11 0 R] >>",
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /FT /Tx /T (buyer) /V (old buyer) /DA (/Helv 10 Tf 0 g) /Kids [7 0 R 8 0 R] >>",
		7:  "<< /Type /Annot /Subtype /Widget /Parent 6 0 R /V (old buyer) /Rect [10 10 150 30] /P 3 0 R >>",
		8:  "<< /Type /Annot /Subtype /Widget /Parent 6 0 R /V (old buyer) /Rect [10 10 150 30] /P 4 0 R >>",
		9:  "<< /FT /Tx /T (seller) /V (the seller) /DA (/Helv 10 Tf 0 g) /Kids [10 0 R 11 0 R] >>",
		10: "<< /Type /Annot /Subtype /Widget /Parent 9 0 R /V (the seller) /Rect [10 40 150 60] /P 3 0 R >>",
		11: "<< /Type /Annot /Subtype /Widget /Parent 9 0 R /V (the seller) /Rect [10 40 150 60] /P 4 0 R >>",
	})
}

// TestAFillOfAFieldShownInSeveralPlacesLeavesNoWidgetHoldingTheOldValue — `/pending 888`. pdfcpu sets the
// field's value and redraws each widget; a copy of the old value left on a widget is what pdf.js then reads.
// After the fill the field holds the new value, none of its widgets holds any other, each widget has been
// drawn — and a repeated field the fill did not change is exactly as the file wrote it.
func TestAFillOfAFieldShownInSeveralPlacesLeavesNoWidgetHoldingTheOldValue(t *testing.T) {
	pdf := repeatedFieldForm()
	data, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the export sees both fields and the value about to be replaced.
	if !bytes.Contains(data, []byte(`"old buyer"`)) || !bytes.Contains(data, []byte(`"the seller"`)) {
		t.Fatalf("setup: the export does not hold both fields' values: %s", data)
	}
	filled, err := FillFormJSON(pdf, bytes.Replace(data, []byte(`"old buyer"`), []byte(`"new buyer"`), 1))
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadContext(bytes.NewReader(filled), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	fields, err := formFields(ctx.XRefTable)
	if err != nil {
		t.Fatal(err)
	}
	text := func(o types.Object) string {
		s, _ := types.StringOrHexLiteral(resolved(ctx.XRefTable, o))
		if s == nil {
			return ""
		}
		return *s
	}
	seen := 0
	for _, f := range fields {
		name := text(f.dict["T"])
		kids := derefArray(ctx.XRefTable, f.dict["Kids"])
		if len(kids) != 2 {
			t.Fatalf("%s: %d widgets in the output, want 2", name, len(kids))
		}
		for i, k := range kids {
			kd := derefDict(ctx.XRefTable, k)
			own, has := kd.Find("V")
			switch name {
			case "buyer":
				if has {
					t.Errorf("buyer, widget %d: still holds a value of its own (%q) after the field was filled — a reader that takes the nearest /V shows it", i+1, text(own))
				}
				if derefDict(ctx.XRefTable, kd["AP"]) == nil {
					t.Errorf("buyer, widget %d: was not drawn", i+1)
				}
			case "seller":
				if !has || text(own) != "the seller" {
					t.Errorf("seller, widget %d: a field the fill did not change lost or changed its widget's value (%q, present %v)", i+1, text(own), has)
				}
			}
		}
		switch name {
		case "buyer":
			seen++
			if got := text(f.dict["V"]); got != "new buyer" {
				t.Errorf("buyer holds %q, want the new value", got)
			}
		case "seller":
			seen++
			if got := text(f.dict["V"]); got != "the seller" {
				t.Errorf("seller holds %q, and was not filled", got)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("the output's form has %d of the two fields", seen)
	}
	// And the export of the result reads the new value, once.
	back, err := ExportFormJSON(filled)
	if err != nil || strings.Count(string(back), `"new buyer"`) != 1 || strings.Contains(string(back), `"old buyer"`) {
		t.Errorf("the filled form exports as: %s (%v)", back, err)
	}
}
