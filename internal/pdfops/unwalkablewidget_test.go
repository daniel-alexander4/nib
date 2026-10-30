package pdfops

import (
	"bytes"
	"context"
	"fmt"
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

// formWithAParentChain is formWithAWidgetWhoseParentIsEmpty with the orphan's `/Parent` leading through depth
// named containers before it reaches an object that is not in the file.
func formWithAParentChain(depth int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R 9 0 R] >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /V (before) /DA (/Helv 10 Tf 0 g) /Rect [10 10 150 30] /P 3 0 R >>",
		9: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (orphan) /Parent 100 0 R /Rect [10 40 70 60] /P 3 0 R >>",
	}
	for i := 0; i < depth; i++ {
		objs[100+i] = fmt.Sprintf("<< /T (c%d) /Parent %d 0 R >>", i, 101+i)
	}
	return testpdf.Assemble(objs)
}

// TestTheParentWalkIsBoundedAtFiftyHops — P02 red-proof survivor 7. `unwalkableWidgets` follows a `/Parent` chain
// for 50 links: a chain that ends in nothing inside the bound is set aside and the export reads the rest, and one
// that does not end inside it is left for pdfcpu, which refuses the export. The bound is the file's to hit,
// as `formFields`' is, so both sides are held: a smaller bound loses the first row, an unbounded walk the second.
func TestTheParentWalkIsBoundedAtFiftyHops(t *testing.T) {
	for _, c := range []struct {
		depth    int
		setAside bool
	}{{3, true}, {40, true}, {60, false}} {
		_, err := ExportFormJSON(formWithAParentChain(c.depth))
		if c.setAside && err != nil {
			t.Errorf("a chain of %d containers ending in nothing: the export failed: %v", c.depth, err)
		}
		if !c.setAside && err == nil {
			t.Errorf("a chain of %d containers (past the bound) exported: it was set aside, so the bound has moved", c.depth)
		}
	}
}
