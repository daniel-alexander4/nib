package pdfops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/testpdf"
)

// formWithFieldsOffPage is a one-page document as other software leaves it after cutting a page out of a larger
// form: the page carries one text box (`onpage`), and the form's tree still lists, on no page — `gone` (a text
// box holding what was typed), `blank` (one holding nothing), and under a container `section` a check box that is
// on and a field with two widgets that holds a value. acroForm is written into the form's own dictionary.
func formWithFieldsOffPage(offPage bool, acroForm ...string) []byte {
	fields := "6 0 R"
	objs := map[int]string{
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R] >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (onpage) /V (kept) /DA (/Helv 10 Tf 0 g) /Rect [10 10 150 30] /P 3 0 R >>",
	}
	if offPage {
		fields += " 7 0 R 8 0 R 9 0 R"
		objs[7] = "<< /Type /Annot /Subtype /Widget /FT /Tx /T (gone) /V (typed on a page that is not here) /DA (/Helv 10 Tf 0 g) /Rect [10 40 150 60] >>"
		objs[8] = "<< /Type /Annot /Subtype /Widget /FT /Tx /T (blank) /DA (/Helv 10 Tf 0 g) /Rect [10 70 150 90] >>"
		objs[9] = "<< /T (section) /Kids [10 0 R 11 0 R] >>"
		objs[10] = "<< /Type /Annot /Subtype /Widget /FT /Btn /T (agreed) /V /Yes /AS /Yes /Parent 9 0 R /Rect [10 100 20 110] >>"
		objs[11] = "<< /FT /Tx /T (twice) /V (shown in two places) /Parent 9 0 R /Kids [12 0 R 13 0 R] >>"
		objs[12] = "<< /Type /Annot /Subtype /Widget /Parent 11 0 R /Rect [10 120 150 140] >>"
		objs[13] = "<< /Type /Annot /Subtype /Widget /Parent 11 0 R /Rect [10 150 150 170] >>"
	}
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /AcroForm << " + strings.Join(acroForm, " ") + " /Fields [" + fields + "] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>"
	return testpdf.Assemble(objs)
}

// signatureOnlyForm is a document whose one field is an (unsigned) signature field on its page.
func signatureOnlyForm() []byte {
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /SigFlags 1 >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R] >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Sig /T (Signature1) /Rect [10 10 150 60] /P 3 0 R >>",
	})
}

func TestTheFieldsOfAFormAreToldByWhetherTheyAreOnAPage(t *testing.T) {
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(formWithFieldsOffPage(true)), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	fields, err := formFields(ctx.XRefTable)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]formField{}
	for _, f := range fields {
		name, _ := f.dict.StringEntry("T"), 0
		got[*name] = f
	}
	want := map[string]struct {
		onPage, value bool
		kind          string
	}{
		"onpage": {true, true, "Tx"},
		"gone":   {false, true, "Tx"},
		"blank":  {false, false, "Tx"},
		"agreed": {false, true, "Btn"},
		"twice":  {false, true, "Tx"},
	}
	if len(got) != len(want) {
		t.Fatalf("the tree's terminal fields are %d, want %d — a container or a widget kid was counted as a field: %v", len(got), len(want), got)
	}
	for name, w := range want {
		f, ok := got[name]
		if !ok || f.onPage != w.onPage || f.value != w.value || f.kind != w.kind {
			t.Errorf("%s: onPage=%v value=%v kind=%q (found %v), want %v %v %q", name, f.onPage, f.value, f.kind, ok, w.onPage, w.value, w.kind)
		}
	}
}

// TestTheScanNamesFormEntriesThatAreOnNoPage — `/pending 888`. Counts only, medium where one holds an entry,
// low where none does, and nothing at all for a form whose fields are all on its pages.
func TestTheScanNamesFormEntriesThatAreOnNoPage(t *testing.T) {
	find := func(pdf []byte) (Finding, bool) {
		rep, err := Scan(pdf)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range rep.Findings {
			if f.Kind == "offPageFields" {
				return f, true
			}
		}
		return Finding{}, false
	}
	f, ok := find(formWithFieldsOffPage(true))
	if !ok || f.Severity != "medium" || !strings.Contains(f.Detail, "4 form fields that are on no page") || !strings.Contains(f.Detail, "3 of them hold entries") {
		t.Errorf("a form with four fields on no page, three holding an entry: %+v (found %v)", f, ok)
	}
	for _, leak := range []string{"gone", "typed on a page", "shown in two places", "agreed"} {
		if strings.Contains(f.Detail, leak) {
			t.Errorf("the finding prints %q — a field's name or its value is the thing not to print", leak)
		}
	}
	if f, ok := find(formWithFieldsOffPage(false)); ok {
		t.Errorf("a form whose only field is on its page: %+v", f)
	}
	// None holding an entry: the same tree with every off-page /V taken out.
	blank := formWithFieldsOffPage(true)
	for _, v := range []string{"/V (typed on a page that is not here) ", "/V /Yes /AS /Yes ", "/V (shown in two places) "} {
		if !bytes.Contains(blank, []byte(v)) {
			t.Fatalf("setup: the fixture does not hold %q", v)
		}
		blank = bytes.Replace(blank, []byte(v), bytes.Repeat([]byte(" "), len(v)), 1)
	}
	if f, ok := find(blank); !ok || f.Severity != "low" || !strings.Contains(f.Detail, "none holds an entry") {
		t.Errorf("four fields on no page and none holding an entry: %+v (found %v)", f, ok)
	}
}

// TestADocumentWithNoFormFieldsSaysSo — a signature field is not form data, and a document whose only field is
// one has nothing to export: ErrNoFormFields from every export and from the fills that read the form first,
// never pdfcpu's "no form fields affected" and never a bare failure.
func TestADocumentWithNoFormFieldsSaysSo(t *testing.T) {
	sig := signatureOnlyForm()
	for name, fn := range map[string]func([]byte) ([]byte, error){
		"json": ExportFormJSON, "csv": ExportFormCSV, "xfdf": ExportFormXFDF,
		"fill xfdf": func(b []byte) ([]byte, error) {
			return FillFormXFDF(b, []byte(`<?xml version="1.0"?><xfdf xmlns="http://ns.adobe.com/xfdf/"><fields><field name="a"><value>b</value></field></fields></xfdf>`))
		},
		"fill csv": func(b []byte) ([]byte, error) { _, err := FillFormCSV(b, []byte("a\nb\n"), ""); return nil, err },
	} {
		if _, err := fn(sig); !errors.Is(err, ErrNoFormFields) {
			t.Errorf("%s over a signature-only document: %v, want ErrNoFormFields", name, err)
		}
	}
	// Control: a form with a text box on its page exports, so the answer above is about the document.
	if out, err := ExportFormJSON(formWithFieldsOffPage(false)); err != nil || !bytes.Contains(out, []byte("onpage")) {
		t.Fatalf("control: a one-field form does not export: %v", err)
	}
}

// fieldsOf is formFields over pdf, read without validation.
func fieldsOf(t *testing.T, pdf []byte) ([]formField, error) {
	t.Helper()
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	return formFields(ctx.XRefTable)
}

// TestRemovingFormEntriesOnNoPageLeavesTheFormOnThePagesAsItWas — `/pending 888`. After the removal the scan
// finds none, the tree holds exactly the field on the page with the value it had, and the calculation order
// names nothing that is gone. A form with nothing to remove comes back with its fields as they were.
func TestRemovingFormEntriesOnNoPageLeavesTheFormOnThePagesAsItWas(t *testing.T) {
	// The calculation order names one field that stays and one that goes.
	pdf := formWithFieldsOffPage(true, "/CO [6 0 R 7 0 R]")
	before := exportedValues(t, pdf)
	out, err := RemoveOffPageFields(pdf)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := fieldsOf(t, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || !fields[0].onPage || !fields[0].value {
		t.Fatalf("after the removal the form's fields are %+v, want the one on the page, holding its value", fields)
	}
	// What was removed is not in the file at all — asked of every object the file holds, read through the
	// parser: a search of the bytes says nothing about a file whose objects are in compressed streams.
	ctx0, err := api.ReadContext(context.Background(), bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	objects := 0
	for nr := range ctx0.XRefTable.Table {
		e, found := ctx0.XRefTable.Find(nr)
		if !found || e.Free || e.Object == nil {
			continue
		}
		objects++
		text := e.Object.PDFString()
		for _, leak := range []string{"typed on a page that is not here", "shown in two places", "(gone)", "(section)", "(agreed)"} {
			if strings.Contains(text, leak) {
				t.Errorf("object %d of the result still holds %q", nr, leak)
			}
		}
	}
	if objects < 5 {
		t.Fatalf("setup: only %d objects of the result were read, so nothing was searched", objects)
	}
	if after := exportedValues(t, out); after != before || !strings.Contains(before, "onpage=kept") {
		t.Errorf("the form on the page does not export as it did:\nbefore %s\nafter  %s", before, after)
	}
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := ctx.XRefTable.Catalog()
	af := derefDict(ctx.XRefTable, root["AcroForm"])
	if co := derefArray(ctx.XRefTable, af["CO"]); len(co) != 1 {
		t.Errorf("the calculation order names %d fields after the removal, want the one that is still there", len(co))
	}
	if n := len(derefArray(ctx.XRefTable, af["Fields"])); n != 1 {
		t.Errorf("the form lists %d top-level fields, want 1 — a container emptied by the removal was left behind", n)
	}
	rep, err := Scan(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		if f.Kind == "offPageFields" {
			t.Errorf("the scan still reports %q", f.Detail)
		}
	}
	// Nothing to remove: the fields are as they were.
	plain := formWithFieldsOffPage(false)
	kept, err := RemoveOffPageFields(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := fieldsOf(t, kept); len(got) != 1 || !got[0].onPage {
		t.Errorf("a form with nothing off its pages came back as %+v", got)
	}
}

// exportedValues is pdf's form as pdfcpu exports it, reduced to "name=value" for its text boxes, sorted — the
// part of the export that is about the form and not about the file (the export's header names object numbers).
func exportedValues(t *testing.T, pdf []byte) string {
	t.Helper()
	data, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var fg form.FormGroup
	if err := json.Unmarshal(data, &fg); err != nil || len(fg.Forms) == 0 {
		t.Fatalf("the export does not parse: %v", err)
	}
	var out []string
	for _, f := range fg.Forms[0].TextFields {
		out = append(out, f.Name+"="+f.Value)
	}
	sort.Strings(out)
	return strings.Join(out, ";")
}
