package pdfread_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// `/pending 888`, ADR-168: five of 203 real documents were refused by every operation because pdfcpu's validator
// refuses a whole document over an annotation with no `/Subtype` in a page's `/Annots`, or over a `/P` that is not
// a reference to a page. The read sets both aside for the validator and puts them back as parsed.

const stampForm = "<< /Type /XObject /Subtype /Form /BBox [0 0 50 20] /Length 12 >>\nstream\n0 0 1 1 re f\nendstream"

// A stamp as the documents hold it: an appearance, a rectangle, a name, its page — and no /Subtype.
const bareStamp = "<< /Rect [100 100 150 120] /F 4 /NM (stamp) /P 3 0 R /AP << /N 9 0 R >> >>"

// annotsDocument is a one-page document whose page holds annots (the text of its `/Annots` value), with objects
// 5-8 free for the annotations and 9 a form for an appearance.
func annotsDocument(annots string, objs map[int]string) []byte {
	all := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Annots " + annots + " >>",
		4: "<< /Length 12 >>\nstream\n0 0 1 1 re f\nendstream",
		9: stampForm,
	}
	for nr, o := range objs {
		all[nr] = o
	}
	return testpdf.Assemble(all)
}

const (
	linkAnnot = "<< /Type /Annot /Subtype /Link /Rect [10 10 60 30] /Border [0 0 0] /A << /S /URI /URI (https://example.com/) >> >>"
	textAnnot = "<< /Type /Annot /Subtype /Text /Rect [200 200 220 220] /Contents (a note) >>"
)

// annotsOf is the page's /Annots as object numbers, with "!" after one whose dictionary has no /Subtype.
func annotsOf(t *testing.T, ctx *model.Context) string {
	t.Helper()
	d, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	o, ok := d.Find("Annots")
	if !ok {
		return "(none)"
	}
	a, err := ctx.DereferenceArray(o)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range a {
		s := "direct"
		if r, ok := e.(types.IndirectRef); ok {
			s = fmt.Sprint(r.ObjectNumber.Value())
		}
		ad, err := ctx.DereferenceDict(e)
		if err != nil || ad == nil {
			t.Fatalf("an /Annots entry is not a dictionary: %v", err)
		}
		if _, has := ad.Find("Subtype"); !has {
			s += "!"
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

func TestAnAnnotationWithNoSubtypeDoesNotRefuseTheDocumentAndIsStillInIt(t *testing.T) {
	for _, c := range []struct {
		name, annots string
		objs         map[int]string
		want         string
	}{
		{"between two others", "[5 0 R 6 0 R 7 0 R]", map[int]string{5: linkAnnot, 6: bareStamp, 7: textAnnot}, "5 6! 7"},
		{"first", "[6 0 R 5 0 R]", map[int]string{5: linkAnnot, 6: bareStamp}, "6! 5"},
		{"last", "[5 0 R 6 0 R]", map[int]string{5: linkAnnot, 6: bareStamp}, "5 6!"},
		{"two together", "[5 0 R 6 0 R 8 0 R 7 0 R]", map[int]string{5: linkAnnot, 6: bareStamp, 8: bareStamp, 7: textAnnot}, "5 6! 8! 7"},
		{"the only entries", "[6 0 R 8 0 R]", map[int]string{6: bareStamp, 8: bareStamp}, "6! 8!"},
		{"the array held by reference", "8 0 R", map[int]string{5: linkAnnot, 6: bareStamp, 8: "[5 0 R 6 0 R]"}, "5 6!"},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := annotsDocument(c.annots, c.objs)
			ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("the document was refused: %v", err)
			}
			if got := annotsOf(t, ctx); got != c.want {
				t.Fatalf("after the read the page's /Annots is %q, want %q — the entry is set aside for the validator only", got, c.want)
			}
			// And through a write: what an operation hands back still holds it.
			out, err := pdfread.Write(ctx)
			if err != nil {
				t.Fatalf("the document could not be written: %v", err)
			}
			again, err := pdfread.ReadOptimized(out, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("the written document was refused: %v", err)
			}
			if got := strings.Count(annotsOf(t, again), "!"); got != strings.Count(c.want, "!") {
				t.Errorf("the written document holds %d annotation(s) with no /Subtype, want %d", got, strings.Count(c.want, "!"))
			}
			if got, want := len(strings.Fields(annotsOf(t, again))), len(strings.Fields(c.want)); got != want {
				t.Errorf("the written document's page holds %d annotation(s), want %d", got, want)
			}
		})
	}
}

// A field's widget with no /Subtype is the validator's own to repair, through the form — it is not set aside.
func TestAWidgetWithNoSubtypeIsLeftToTheValidator(t *testing.T) {
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] /DA (/Helv 0 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Annots [5 0 R] >>",
		4: "<< /Length 12 >>\nstream\n0 0 1 1 re f\nendstream",
		5: "<< /FT /Tx /T (name) /Rect [72 560 400 596] /F 4 /P 3 0 R /DA (/Helv 12 Tf 0 g) >>",
	})
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("the document was refused: %v", err)
	}
	if got := annotsOf(t, ctx); got != "5" {
		t.Errorf("the widget reads %q after validation, want \"5\" — the validator gives a field's widget its /Subtype", got)
	}
	// And the validator met it ON ITS PAGE: what it records of a page's annotations is what pdfcpu's own operations
	// read afterwards, and a widget kept from that walk would be missing from it.
	if n := len(ctx.PageAnnots[1]); n == 0 {
		t.Error("the validator recorded no annotation on the page — a field's widget is not to be set aside")
	}
}

func TestAPageEntryThatIsNotAPageDoesNotRefuseTheDocumentAndIsStillInIt(t *testing.T) {
	const widget = "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /Rect [72 560 400 596] /F 4 /P %s /DA (/Helv 12 Tf 0 g) >>"
	for _, c := range []struct {
		name, p, onPage string
		aside           bool
	}{
		{"a stream, on a page", "9 0 R", "[5 0 R]", true},
		{"a stream, on no page", "9 0 R", "[]", true},
		{"a dictionary that is not a page", "2 0 R", "[5 0 R]", true},
		{"control: its page", "3 0 R", "[5 0 R]", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := testpdf.Assemble(map[int]string{
				1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] /DA (/Helv 0 Tf 0 g) >> >>",
				2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
				3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Annots " + c.onPage + " >>",
				4: "<< /Length 12 >>\nstream\n0 0 1 1 re f\nendstream",
				5: fmt.Sprintf(widget, c.p),
				9: stampForm,
			})
			ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("the document was refused: %v", err)
			}
			d, err := ctx.DereferenceDict(*types.NewIndirectRef(5, 0))
			if err != nil || d == nil {
				t.Fatalf("the widget is gone: %v", err)
			}
			p, ok := d.Find("P")
			if !ok {
				t.Fatal("the widget has no /P after the read — it is set aside for the validator only, and put back")
			}
			if got := p.String(); got != "("+c.p+")" {
				t.Errorf("the widget's /P reads %s after the read, want (%s)", got, c.p)
			}
		})
	}
}
