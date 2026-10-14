package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// TestAFieldWhoseRectIsGivenTopFirstIsDrawnWhenFilled — `/pending 894`. A widget's /Rect may name its corners in
// either order; one given top first ([x1 234 x2 219], as a real form engine writes them) was filled by pdfcpu
// into an appearance whose /BBox has a negative height, and readers drew nothing. After a fill the widget's
// rectangle is the same one, lower-left first, and its appearance box has the field's height.
func TestAFieldWhoseRectIsGivenTopFirstIsDrawnWhenFilled(t *testing.T) {
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] /DR << /Font << /Helv 4 0 R >> >> /DA (/Helv 9 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [5 0 R] >>",
		4: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		5: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (from) /DA (/Helv 9 Tf 0 g) /Rect [40 234.26 140 219.28] /P 3 0 R >>",
	})
	data, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	filled, err := FillFormJSON(pdf, bytes.Replace(data, []byte(`"value": ""`), []byte(`"value": "06/2020"`), 1))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"value": ""`)) {
		t.Fatalf("setup: the export does not hold the empty field: %s", data)
	}
	ctx, err := pdfread.Validated(filled, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pd, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	annots := derefArray(ctx.XRefTable, pd["Annots"])
	if len(annots) != 1 {
		t.Fatalf("%d annotations after the fill, want 1", len(annots))
	}
	w := derefDict(ctx.XRefTable, annots[0])
	if got := w["Rect"].String(); got != "[40.00 219.28 140.00 234.26]" {
		t.Errorf("the widget's /Rect after the fill is %s, want the same rectangle lower-left first, [40.00 219.28 140.00 234.26]", got)
	}
	ap := w.DictEntry("AP")
	if ap == nil {
		t.Fatal("the filled widget has no appearance")
	}
	sd, _, err := ctx.DereferenceStreamDict(ap["N"])
	if err != nil || sd == nil {
		t.Fatalf("the filled widget's normal appearance is not a stream: %v", err)
	}
	bbox := derefArray(ctx.XRefTable, sd.Dict["BBox"])
	if len(bbox) != 4 {
		t.Fatalf("appearance /BBox %v", sd.Dict["BBox"])
	}
	h := func(o types.Object) float64 {
		switch v := resolved(ctx.XRefTable, o).(type) {
		case types.Float:
			return v.Value()
		case types.Integer:
			return float64(v.Value())
		}
		return 0
	}
	if height := h(bbox[3]) - h(bbox[1]); height < 14 {
		t.Errorf("the filled appearance's /BBox is %v, %.2f high — a reader draws nothing into a box of no positive height; want the field's 14.98", sd.Dict["BBox"], height)
	}
}
