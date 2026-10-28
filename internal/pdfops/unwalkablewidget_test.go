package pdfops

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// formWithAWidgetWhoseParentIsEmpty is a one-page form with a text box and a second widget whose `/Parent` names an
// object that is not in the file — the shape two of 206 real forms carry (P02.S05, D4).
func formWithAWidgetWhoseParentIsEmpty() []byte {
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R 9 0 R] >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /V (before) /DA (/Helv 10 Tf 0 g) /Rect [10 10 150 30] /P 3 0 R >>",
		9: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (orphan) /Parent 99 0 R /Rect [10 40 70 60] /P 3 0 R >>",
	})
}

// TestAWidgetWhoseParentIsEmptyDoesNotEndTheExport — pdfcpu v0.16.2 answers an export or fill "corrupt field" at a
// `/Parent` that dereferences to nothing (`form/form.go:154-156`); v0.13.0 skipped the widget. The
// widget is set aside for the walk and is on the page as it was afterwards.
func TestAWidgetWhoseParentIsEmptyDoesNotEndTheExport(t *testing.T) {
	pdf := formWithAWidgetWhoseParentIsEmpty()
	// The stimulus: without the set-aside, pdfcpu refuses this document.
	if _, err := pdfread.ExportFormJSON(pdf, "nib", nil); err == nil || !strings.Contains(err.Error(), "corrupt field") {
		t.Fatalf("stimulus: pdfcpu without the set-aside answered %v, want \"corrupt field\" — the fixture no longer reaches the refusal", err)
	}
	data, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatalf("the export failed on a widget whose parent is empty: %v", err)
	}
	if !bytes.Contains(data, []byte(`"before"`)) {
		t.Errorf("the export should hold the text box: %s", data)
	}
	filled, err := FillFormJSON(pdf, bytes.Replace(data, []byte(`"before"`), []byte(`"after"`), 1))
	if err != nil {
		t.Fatalf("the fill failed: %v", err)
	}
	if back, err := ExportFormJSON(filled); err != nil || !strings.Contains(string(back), `"after"`) {
		t.Errorf("the filled form exports as %s (%v)", back, err)
	}
	// Put back, not removed: the page still lists both widgets.
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(filled), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := ctx.XRefTable.Catalog()
	n := 0
	_ = eachPageAnnots(ctx.XRefTable, root, func(_ types.Dict, raw types.Object, _ int) { n += len(derefArray(ctx.XRefTable, raw)) })
	if n != 2 {
		t.Errorf("the filled document's page lists %d annotations, want the 2 it had", n)
	}
}
