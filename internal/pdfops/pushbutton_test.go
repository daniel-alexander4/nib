package pdfops

import (
	"bytes"
	"context"
	"slices"
	"sort"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// formWithAPushButton is a one-page form with a text box and a push button — a "Print" or "Reset" button, which
// holds no value. Its appearance is one stream, as a push button's is; a check box's is a dictionary of states.
func formWithAPushButton() []byte { return pushButtonForm(false) }

// pushButtonForm is formWithAPushButton with the page's /Annots written in an object of its own where indirect.
func pushButtonForm(indirect bool) []byte {
	draw := "q 0.8 g 0 0 60 20 re f Q"
	annots, extra := "[6 0 R 9 0 R 10 0 R]", ""
	if indirect {
		annots, extra = "11 0 R", "[6 0 R 9 0 R 10 0 R]"
	}
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R 7 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots " + annots + " >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /V (before) /DA (/Helv 10 Tf 0 g) /Rect [10 10 150 30] /P 3 0 R >>",
		// The button as the real forms that failed write it: the field, and its widget as a kid with no name.
		// pdfcpu takes a button with more than one widget for a radio group and reads each widget's appearance
		// as a dictionary of states; this button is drawn twice, top and bottom of the page.
		7:  "<< /FT /Btn /Ff 65536 /T (print) /Kids [9 0 R 10 0 R] >>",
		9:  "<< /Type /Annot /Subtype /Widget /Parent 7 0 R /Rect [10 40 70 60] /P 3 0 R /AP << /N 8 0 R >> >>",
		10: "<< /Type /Annot /Subtype /Widget /Parent 7 0 R /Rect [10 250 70 270] /P 3 0 R /AP << /N 8 0 R >> >>",
		8:  "<< /Type /XObject /Subtype /Form /BBox [0 0 60 20] /Length " + itoa(len(draw)) + " >>\nstream\n" + draw + "\nendstream",
	}
	if indirect {
		objs[11] = extra
	}
	return testpdf.Assemble(objs)
}

// TestAFormWithAPushButtonExportsAndFills — `/pending 888`. pdfcpu reads every button as a check box and asks
// its appearance for a dictionary of states; a push button's is a stream, and the whole export failed on it
// ("dereferenceDict: wrong type types.StreamDict" — three real forms). A push button holds nothing to export
// or fill, so it is set aside for pdfcpu's walk: the export lists the form's fields without it, a fill fills
// them, and the button is still on the page of the filled document, as it was.
func TestAFormWithAPushButtonExportsAndFills(t *testing.T) {
	pdf := formWithAPushButton()
	data, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatalf("the export failed on a form with a push button: %v", err)
	}
	if !bytes.Contains(data, []byte(`"before"`)) || bytes.Contains(data, []byte(`"print"`)) {
		t.Errorf("the export should hold the text box and not the button: %s", data)
	}
	for name, fn := range map[string]func([]byte) ([]byte, error){"csv": ExportFormCSV, "xfdf": ExportFormXFDF} {
		if out, err := fn(pdf); err != nil || !bytes.Contains(out, []byte("before")) {
			t.Errorf("%s export of a form with a push button: %v", name, err)
		}
	}
	filled, err := FillFormJSON(pdf, bytes.Replace(data, []byte(`"before"`), []byte(`"after"`), 1))
	if err != nil {
		t.Fatalf("the fill failed on a form with a push button: %v", err)
	}
	back, err := ExportFormJSON(filled)
	if err != nil || !strings.Contains(string(back), `"after"`) {
		t.Errorf("the filled form exports as %s (%v)", back, err)
	}
	// The button is still a widget on the page, with its appearance: set aside for the walk, not removed.
	fields, err := fieldsOf(t, filled)
	if err != nil {
		t.Fatal(err)
	}
	var button *formField
	for i := range fields {
		if fields[i].pushButton() {
			button = &fields[i]
		}
	}
	if button == nil || !button.onPage || len(button.dict.ArrayEntry("Kids")) != 2 {
		t.Errorf("the push button is not on the filled document's page as it was: %+v", button)
	}
}

// TestPushButtonsAsideLeavesThePagesAsItFoundThem — P02 review F13. The test above passes whether or not the button
// is set aside (v0.16.2 skips a push button itself, `form/form.go:463-471`), and nothing asked the put-back for
// the arrays it must restore: a page's /Annots written in the page or in an object it names, and the `PageAnnots`
// cache pdfcpu's fill and its choice-field export walk.
func TestPushButtonsAsideLeavesThePagesAsItFoundThem(t *testing.T) {
	for name, pdf := range map[string][]byte{"direct": pushButtonForm(false), "indirect": pushButtonForm(true)} {
		conf := model.NewDefaultConfiguration()
		conf.Cmd = model.EXPORTFORMFIELDS
		ctx, err := pdfread.ReadOptimized(pdf, conf)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		xt := ctx.XRefTable
		annots := func() []string {
			pd, _, _, err := xt.PageDict(context.Background(), 1, false)
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, a := range derefArray(xt, pd["Annots"]) {
				out = append(out, a.String())
			}
			return out
		}
		cached := func() (refs []int) {
			for _, pa := range xt.PageAnnots {
				for _, an := range pa {
					if an.IndRefs != nil {
						for _, r := range *an.IndRefs {
							refs = append(refs, r.ObjectNumber.Value())
						}
					}
				}
			}
			sort.Ints(refs)
			return refs
		}
		wasAnnots, wasCache := annots(), cached()
		if len(wasAnnots) != 3 || len(wasCache) != 3 {
			t.Fatalf("%s: setup: the page holds %v and the cache %v, want three each", name, wasAnnots, wasCache)
		}
		putBack := pushButtonsAside(xt)
		if len(annots()) != 1 || len(cached()) != 1 {
			t.Errorf("%s: the button's widgets were not set aside: page %v, cache %v", name, annots(), cached())
		}
		putBack()
		if !slices.Equal(annots(), wasAnnots) || !slices.Equal(cached(), wasCache) {
			t.Errorf("%s: put back as page %v / cache %v, want %v / %v", name, annots(), cached(), wasAnnots, wasCache)
		}
	}
}
