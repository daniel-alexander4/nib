package pdfops

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 705`: the form's own dictionary is a road `/pending 688`'s resource prune did not walk.
// `/AcroForm /DR` (default resources) was kept whole, and a producer may point it at the pages'
// shared resource dictionary — so a form or image only the redacted page drew stayed reachable, and
// pdfcpu writes by reachability. `/AcroForm /XFA` carries every page's form data outright.

const secretXFA = "SECRETXFADATA"

// formDoc is `sharedResourceDoc`'s one-shared-dictionary shape plus a text field on page 2, whose
// `/AcroForm /DR` is either that shared dictionary or a direct one naming the secret form, and an
// XFA packet carrying a secret.
func formDoc(drShared bool) []byte {
	form := fmt.Sprintf("BT /F1 12 Tf 72 700 Td (%s) Tj ET", secretText)
	xfa := fmt.Sprintf("<xdp:xdp><field name=\"a\">%s</field></xdp:xdp>", secretXFA)
	dr := "10 0 R"
	if !drShared {
		dr = "<< /XObject << /Fm1 6 0 R >> /Font << /F1 7 0 R >> >>"
	}
	objs := map[int]string{
		1:  fmt.Sprintf("<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [11 0 R] /DA (/F1 0 Tf 0 g) /DR %s /XFA 12 0 R >> >>", dr),
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Resources 10 0 R >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 8 0 R /Resources 10 0 R /Annots [11 0 R] >>",
		5:  stream("q /Fm1 Do Q q 16 0 0 1 72 72 cm /Im1 Do Q"),
		6:  fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
		7:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		8:  stream(fmt.Sprintf("BT /F1 12 Tf 72 700 Td (%s) Tj ET", publicText)),
		9:  fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 16 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>\nstream\n%s\nendstream", len(secretImage), secretImage),
		10: "<< /XObject << /Fm1 6 0 R /Im1 9 0 R >> /Font << /F1 7 0 R >> /ProcSet [/PDF /Text] >>",
		11: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (kept) /DA (/F1 12 Tf 0 g) /Rect [72 72 272 96] /P 4 0 R >>",
		12: stream(xfa),
	}
	return assembleFixture(objs)
}

// acroForm returns the output's /AcroForm, read the way nib reads it.
func acroForm(t *testing.T, pdf []byte) (*model.XRefTable, types.Dict) {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	return ctx.XRefTable, derefDict(ctx.XRefTable, root["AcroForm"])
}

// TestTheFormDictionaryCarriesNothingOnlyADroppedPageDrew — the item's property, through both subset
// doors the review measured and both shapes of `/DR`, asserted on the stimulus first.
func TestTheFormDictionaryCarriesNothingOnlyADroppedPageDrew(t *testing.T) {
	doors := map[string]func([]byte) ([]byte, error){
		"RedactPages": func(src []byte) ([]byte, error) {
			return RedactPages(src, map[int]RasterPage{1: rasterPage(t, 612, 792)})
		},
		"RemovePages": func(src []byte) ([]byte, error) { return RemovePages(src, []string{"1"}) },
	}
	for _, shared := range []bool{true, false} {
		for door, run := range doors {
			name := fmt.Sprintf("%s, /DR %s", door, map[bool]string{true: "is the pages' shared dictionary", false: "names the form"}[shared])
			t.Run(name, func(t *testing.T) {
				src := formDoc(shared)
				before := everyDecodedByte(t, src)
				for _, s := range []string{secretText, secretImage, secretXFA} {
					if !bytes.Contains(before, []byte(s)) {
						t.Fatalf("setup: the source does not carry %q, so its absence proves nothing", s)
					}
				}
				out, err := run(src)
				if err != nil {
					t.Fatal(err)
				}
				after := everyDecodedByte(t, out)
				for _, s := range []string{secretText, secretImage, secretXFA} {
					if bytes.Contains(after, []byte(s)) {
						t.Errorf("the output still carries %q, which only the dropped page (or the form's XFA) held", s)
					}
				}
				// The control: the field on the kept page survived, with the font its /DA names — so the
				// absences above are the prune's, not a form thrown away whole.
				xt, af := acroForm(t, out)
				if af == nil || len(derefArray(xt, af["Fields"])) != 1 {
					t.Fatal("the kept page's field did not survive")
				}
				if derefDict(xt, derefDict(xt, af["DR"])["Font"])["F1"] == nil {
					t.Error("/DR lost /F1, the font the kept field's /DA names — its appearance cannot be regenerated")
				}
			})
		}
	}
}
