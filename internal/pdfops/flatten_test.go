package pdfops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// widgetForm is a one-page form (200x300). widgets are the bodies of objects 10, 11, …; extra holds further objects
// (the appearance streams, from 20), and acroExtra goes into the AcroForm dictionary.
func widgetForm(acroExtra string, extra map[int]string, widgets ...string) []byte {
	objs := map[int]string{
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var refs []string
	for i, w := range widgets {
		refs = append(refs, fmt.Sprintf("%d 0 R", 10+i))
		objs[10+i] = w
	}
	for n, o := range extra {
		objs[n] = o
	}
	objs[1] = fmt.Sprintf("<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [%s] /DA (/Helv 10 Tf 0 g) /DR << /Font << /Helv 5 0 R >> >> %s >> >>",
		strings.Join(refs, " "), acroExtra)
	objs[3] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 300] /Annots [%s] >>", strings.Join(refs, " "))
	return testpdf.Assemble(objs)
}

// apStream is an appearance stream drawing text in a 50x10 box, with (or without) fonts of its own.
func apStream(text string, withResources bool) string {
	body := fmt.Sprintf("BT /Helv 8 Tf 1 2 Td (%s) Tj ET", text)
	res := ""
	if withResources {
		res = " /Resources << /Font << /Helv 5 0 R >> >>"
	}
	return fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 50 10]%s /Length %d >>\nstream\n%s\nendstream", res, len(body), body)
}

func flatText(t *testing.T, pdf []byte) (string, []TextRun) {
	t.Helper()
	pages, err := DocumentText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	var runs []TextRun
	for _, p := range pages {
		for _, r := range p.Runs {
			b.WriteString(r.Text + " ")
			runs = append(runs, r)
		}
	}
	return b.String(), runs
}

// TestAFilledFormIsFlattenedIntoPageContentAndItsTextStaysText — the point of the verb (RelaxTax Group B1): the web
// Flatten rasterises, and loses the text layer. Here a filled value is read back as TEXT afterwards, no field is left,
// and the file is the form's size and not a picture's.
func TestAFilledFormIsFlattenedIntoPageContentAndItsTextStaysText(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	filled, err := FillFormJSON(form, []byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Ada Lovelace"}],"checkbox":[{"name":"agree","value":true}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := flatText(t, filled)
	out, res, err := FlattenForm(filled, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Flattened < 2 || res.Hidden != 0 {
		t.Errorf("result %+v, want the text field and the ticked box flattened", res)
	}
	after, _ := flatText(t, out)
	if !strings.Contains(after, "Ada Lovelace") {
		t.Errorf("the filled value is not text after the flatten (before %q): %q", before, after)
	}
	if fields, err := ListFormFields(out); err == nil && len(fields) != 0 {
		t.Errorf("%d field(s) survive the flatten: %v", len(fields), fields)
	}
	if len(out) > 2*len(filled)+4096 {
		t.Errorf("the flattened file is %d bytes against the filled %d: not the vector reuse it should be", len(out), len(filled))
	}
	if _, _, err := FlattenForm(out, false); !errors.Is(err, ErrNoFormFields) {
		t.Errorf("flattening a flattened form = %v, want ErrNoFormFields", err)
	}
}

func TestFlattenPlacesTheAppearanceOnTheWidgetRectangle(t *testing.T) {
	pdf := widgetForm("", map[int]string{20: apStream("Hi", true)},
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (a) /V (Hi) /Rect [100 100 200 120] /AP << /N 20 0 R >> >>")
	out, res, err := FlattenForm(pdf, false)
	if err != nil || res.Flattened != 1 {
		t.Fatalf("res %+v err %v", res, err)
	}
	_, runs := flatText(t, out)
	var found bool
	for _, r := range runs {
		if strings.Contains(r.Text, "Hi") {
			found = true
			// The 50x10 box is scaled 2x2 onto 100..200 by 100..120 (180..200 from the top of the 300pt page).
			if r.Rect[0] < 100-0.5 || r.Rect[2] > 200+0.5 || r.Rect[1] < 180-0.5 || r.Rect[3] > 200+0.5 {
				t.Errorf("text drawn at %v, outside the widget's box 100,180 – 200,200", r.Rect)
			}
			if r.Size < 15.5 || r.Size > 16.5 {
				t.Errorf("text is %v pt, want the 8 pt of the appearance scaled 2x to the box (16)", r.Size)
			}
		}
	}
	if !found {
		t.Errorf("the appearance's text is not on the page: %+v", runs)
	}
}

func TestFlattenReadsTheStateAButtonIsIn(t *testing.T) {
	extra := map[int]string{20: apStream("ON", true), 21: apStream("OFF", true)}
	on := "<< /Type /Annot /Subtype /Widget /FT /Btn /T (a) /V /Yes /AS /Yes /Rect [10 10 60 20] /AP << /N << /Yes 20 0 R /Off 21 0 R >> >> >>"
	// /AS /Gone names a state /N does not have: nothing is drawn, and with no value nothing is lost.
	odd := "<< /Type /Annot /Subtype /Widget /FT /Btn /T (b) /AS /Gone /Rect [10 40 60 50] /AP << /N << /Yes 20 0 R >> >> >>"
	out, res, err := FlattenForm(widgetForm("", extra, on, odd), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Flattened != 1 || res.Blank != 1 {
		t.Errorf("result %+v, want one box drawn and one blank", res)
	}
	txt, _ := flatText(t, out)
	if !strings.Contains(txt, "ON") || strings.Contains(txt, "OFF") {
		t.Errorf("page text %q, want the Yes state and not the Off one", txt)
	}
}

func TestFlattenGivesAnAppearanceTheFormsFontsWhenItHasNone(t *testing.T) {
	pdf := widgetForm("", map[int]string{20: apStream("Dr", false)},
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (a) /V (Dr) /Rect [10 10 60 20] /AP << /N 20 0 R >> >>")
	out, _, err := FlattenForm(pdf, false)
	if err != nil {
		t.Fatal(err)
	}
	// The AcroForm is gone, and with it the /DR the appearance named /Helv through: the text must still set.
	if txt, _ := flatText(t, out); !strings.Contains(txt, "Dr") {
		t.Errorf("the appearance drew nothing once the form's default resources were gone: %q", txt)
	}
	ctx, err := pdfread.ReadForInspection(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := ctx.RootDict["AcroForm"]; has {
		t.Error("the AcroForm survives")
	}
}

func TestFlattenRefusesAValueNothingDraws(t *testing.T) {
	pdf := widgetForm("", nil,
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (ssn) /V (SECRET-9) /Rect [10 10 60 20] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (empty) /Rect [10 40 60 50] >>")
	_, _, err := FlattenForm(pdf, false)
	var miss *FlattenMissingAppearance
	if !errors.As(err, &miss) || len(miss.Fields) != 1 || miss.Fields[0] != "ssn" {
		t.Fatalf("err = %v, want a refusal naming exactly the field with a value", err)
	}
	if strings.Contains(err.Error(), "SECRET-9") {
		t.Errorf("the refusal repeats the value: %q", err)
	}
}

func TestFlattenDoesNotDrawAWidgetAReaderDoesNotShow(t *testing.T) {
	pdf := widgetForm("", map[int]string{20: apStream("SEEN", true), 21: apStream("HID", true)},
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (a) /V (x) /Rect [10 10 60 20] /AP << /N 20 0 R >> >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (b) /V (x) /F 2 /Rect [10 40 60 50] /AP << /N 21 0 R >> >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (c) /V (x) /F 32 /Rect [10 70 60 80] /AP << /N 21 0 R >> >>")
	out, res, err := FlattenForm(pdf, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Flattened != 1 || res.Hidden != 2 {
		t.Errorf("result %+v, want one drawn and two hidden", res)
	}
	if txt, _ := flatText(t, out); !strings.Contains(txt, "SEEN") || strings.Contains(txt, "HID") {
		t.Errorf("page text %q", txt)
	}
}

func TestFlattenLeavesAnythingThatIsNotAWidget(t *testing.T) {
	pdf := widgetForm("", map[int]string{20: apStream("W", true), 21: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /Border [0 0 0] >>"},
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (a) /V (W) /Rect [10 10 60 20] /AP << /N 20 0 R >> >>")
	// Add the link to the page's annotations by hand: the helper names only widgets.
	pdf = []byte(strings.Replace(string(pdf), "/Annots [10 0 R]", "/Annots [10 0 R 21 0 R]", 1))
	out, _, err := FlattenForm(pdf, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.ReadForInspection(out)
	if err != nil {
		t.Fatal(err)
	}
	pd, _, _, _ := ctx.PageDict(context.Background(), 1, false)
	annots, _ := ctx.DereferenceArray(pd["Annots"])
	if len(annots) != 1 {
		t.Fatalf("%d annotations left, want the link alone", len(annots))
	}
	if d, _ := ctx.DereferenceDict(annots[0]); d == nil || d["Subtype"] != types.Name("Link") {
		t.Errorf("the surviving annotation is %v, want the link", d)
	}
}

// TestFlattenOfATaggedFormLeavesATreeThatNamesNoRemovedWidget — a tagged form's widgets are each named by a structure
// element through an OBJR, and a removed annotation an OBJR still names is what a checker reads as a broken tree.
func TestFlattenOfATaggedFormLeavesATreeThatNamesNoRemovedWidget(t *testing.T) {
	base, err := testpdf.Text("")
	if err != nil {
		t.Fatal(err)
	}
	form, tagged, err := AuthorTaggedForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 600, 220, 620}, Kind: "text", Name: "who", Label: "Who"},
		{Page: 1, Rect: [4]float64{100, 560, 112, 572}, Kind: "check", Name: "ok", Label: "Ok"},
	})
	if err != nil || !tagged {
		t.Fatalf("the fixture is not a tagged form (tagged %v, err %v)", tagged, err)
	}
	filled, err := FillFormJSON(form, []byte(`{"forms":[{"textfield":[{"name":"who","value":"Grace"}],"checkbox":[{"name":"ok","value":true}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if st := inspectTags(filled); !st.tree {
		t.Fatalf("the filled fixture has no structure tree: %+v", st)
	}
	out, res, err := FlattenForm(filled, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Flattened < 2 {
		t.Errorf("result %+v", res)
	}
	ctx, err := pdfread.ReadForInspection(out)
	if err != nil {
		t.Fatal(err)
	}
	for nr, e := range ctx.Table {
		if e == nil || e.Free {
			continue
		}
		if d, ok := e.Object.(types.Dict); ok && d["Type"] == types.Name("OBJR") {
			if tgt := derefDict(ctx.XRefTable, d["Obj"]); tgt != nil && nameVal(tgt, "Subtype") == "Widget" {
				t.Errorf("object %d: an OBJR still names a widget", nr)
			}
		}
	}
	if txt, _ := flatText(t, out); !strings.Contains(txt, "Grace") {
		t.Errorf("the value is not text after the flatten: %q", txt)
	}
}

// TestFlattenBalancesTheGraphicsStateItAddsTo — what the page draws is wrapped in q … Q, so the overlay's own Q has
// a q of its own and never pops one the page's content opened (or underflows a balanced page).
func TestFlattenBalancesTheGraphicsStateItAddsTo(t *testing.T) {
	pdf := widgetForm("", map[int]string{20: apStream("Hi", true)},
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (a) /V (Hi) /Rect [10 10 60 20] /AP << /N 20 0 R >> >>")
	out, _, err := FlattenForm(pdf, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.ReadForInspection(out)
	if err != nil {
		t.Fatal(err)
	}
	pd, _, _, _ := ctx.PageDict(context.Background(), 1, false)
	content, err := pdfread.PageContent(ctx, pd, 1)
	if err != nil {
		t.Fatal(err)
	}
	depth := 0
	for _, f := range strings.Fields(string(content)) {
		switch f {
		case "q":
			depth++
		case "Q":
			depth--
			if depth < 0 {
				t.Fatalf("a Q with no q before it in:\n%s", content)
			}
		}
	}
	if depth != 0 {
		t.Errorf("q/Q unbalanced by %d in:\n%s", depth, content)
	}
}
