package pdfops

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"

	"nib/internal/testpdf"
)

// formToList is a two-page form with every shape `nib fields` has to tell apart: a field and its widget in one
// dictionary whose /Rect is given top corner first, a field under a container (a dotted name) whose tooltip is a
// UTF-16 string (listed after the top-level fields: the tree is walked breadth-first), a radio group whose two widgets are on different pages, a push button, and a field no page draws.
func formToList() []byte {
	return testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R 7 0 R 9 0 R 12 0 R 13 0 R] /DA (/Helv 10 Tf 0 g) /DR << /Font << /Helv 5 0 R >> >> >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R 10 0 R 13 0 R] >>",
		4:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [8 0 R 11 0 R] >>",
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /Type /Annot /Subtype /Widget /FT /Tx /DA (/Helv 10 Tf 0 g) /T (name) /TU (Line 1a) /Rect [150 30 10 10] >>",
		7:  "<< /T (parent) /Kids [8 0 R] >>",
		8:  "<< /Type /Annot /Subtype /Widget /FT /Tx /DA (/Helv 10 Tf 0 g) /T (child) /Parent 7 0 R /TU <FEFF00C9> /Rect [20 40 120 60] >>",
		9:  "<< /FT /Btn /Ff 32768 /T (choice) /Kids [10 0 R 11 0 R] >>",
		10: "<< /Type /Annot /Subtype /Widget /Parent 9 0 R /Rect [10 100 20 110] >>",
		11: "<< /Type /Annot /Subtype /Widget /Parent 9 0 R /Rect [10 120 20 130] >>",
		12: "<< /Type /Annot /Subtype /Widget /FT /Tx /DA (/Helv 10 Tf 0 g) /T (gone) /Rect [10 200 100 220] >>",
		13: "<< /Type /Annot /Subtype /Widget /FT /Btn /Ff 65536 /T (go) /Rect [200 10 260 30] >>",
	})
}

func TestTheListingTellsEachFieldByNameTypeTooltipAndWhereItIsDrawn(t *testing.T) {
	got, err := ListFormFields(formToList())
	if err != nil {
		t.Fatal(err)
	}
	want := []FormFieldInfo{
		{Name: "name", Type: "text", Fillable: true, Tooltip: "Line 1a", OnPage: true,
			Widgets: []FieldWidget{{1, [4]float64{10, 10, 150, 30}}}},
		{Name: "choice", Type: "radio", Fillable: true, Flags: 32768, OnPage: true,
			Widgets: []FieldWidget{{1, [4]float64{10, 100, 20, 110}}, {2, [4]float64{10, 120, 20, 130}}}},
		{Name: "gone", Type: "text", Fillable: true, OnPage: false, Widgets: []FieldWidget{}},
		{Name: "go", Type: "pushbutton", Fillable: false, Flags: 65536, OnPage: true,
			Widgets: []FieldWidget{{1, [4]float64{200, 10, 260, 30}}}},
		{Name: "parent.child", Type: "text", Fillable: true, Tooltip: "É", OnPage: true,
			Widgets: []FieldWidget{{2, [4]float64{20, 40, 120, 60}}}},
	}
	if !reflect.DeepEqual(got, want) {
		gb, _ := json.MarshalIndent(got, "", " ")
		wb, _ := json.MarshalIndent(want, "", " ")
		t.Fatalf("listing differs.\ngot:\n%s\nwant:\n%s", gb, wb)
	}
}

// The names are only useful if a fill takes them: every text field the listing offers as fillable and on a page is a
// name pdfcpu's own export emits for the same document.
func TestEveryFillableFieldTheListingNamesIsOneTheExportNames(t *testing.T) {
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R 7 0 R] /DA (/Helv 10 Tf 0 g) /DR << /Font << /Helv 5 0 R >> >> >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R 8 0 R] >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (top) /DA (/Helv 10 Tf 0 g) /Rect [10 10 150 30] >>",
		7: "<< /T (f1) /Kids [8 0 R] >>",
		8: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (line) /Parent 7 0 R /DA (/Helv 10 Tf 0 g) /Rect [10 40 150 60] >>",
	})
	listed, err := ListFormFields(pdf)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var fg form.FormGroup
	if err := json.Unmarshal(raw, &fg); err != nil {
		t.Fatal(err)
	}
	exported := map[string]bool{}
	for _, f := range fg.Forms {
		for _, x := range f.TextFields {
			exported[x.Name] = true
		}
	}
	n := 0
	for _, f := range listed {
		if f.Fillable && f.OnPage {
			n++
			if !exported[f.Name] {
				t.Errorf("listed %q, which the export does not name (it names %v)", f.Name, exported)
			}
		}
	}
	if n != 2 {
		t.Fatalf("expected two fillable fields on a page, listed %d", n)
	}
}

func TestADocumentWithNoFormListsNoFields(t *testing.T) {
	got, err := ListFormFields(testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
	}))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want none and no error", got, err)
	}
}

func TestEachKindOfFieldHasItsOwnWord(t *testing.T) {
	for _, c := range []struct {
		f    formField
		want string
	}{
		{formField{kind: "Tx"}, "text"},
		{formField{kind: "Btn"}, "checkbox"},
		{formField{kind: "Btn", flags: 1 << 15}, "radio"},
		{formField{kind: "Btn", flags: 1 << 16}, "pushbutton"},
		{formField{kind: "Ch"}, "listbox"},
		{formField{kind: "Ch", flags: 1 << 17}, "combobox"},
		{formField{kind: "Sig"}, "signature"},
		{formField{}, "unknown"},
	} {
		if got := fieldType(c.f); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.f, got, c.want)
		}
	}
}

// xfaFormToLabel is a one-page form whose fields are named as an XFA form's are (`a[0]`, `a[1]`: same-named
// siblings), with an XFA packet giving labels three ways and a datasets packet holding a decoy. asArray says whether
// the packet is the array of name/stream pairs or one XDP stream.
func xfaFormToLabel(asArray bool) []byte {
	tpl := `<template><subform name="form1"><field name="a"><assist><speak> First   a
</speak></assist></field><field name="a"><caption><value><text>Second a</text></value></caption></field>` +
		`<field name="b"><assist><toolTip>The b</toolTip></assist></field><field name="c"/></subform></template>`
	data := `<datasets><field name="c">decoy</field></datasets>`
	xfa := "12 0 R"
	objs := map[int]string{
		2:  "<< /Type /Pages /Kids [4 0 R] /Count 1 /MediaBox [0 0 612 792] >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Resources << >> /Annots [11 0 R 13 0 R 14 0 R 15 0 R] >>",
		5:  stream("q Q"),
		7:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		10: "<< /T (form1[0]) /Kids [11 0 R 13 0 R 14 0 R 15 0 R] >>",
	}
	for i, n := range []struct {
		obj  int
		name string
	}{{11, "a[0]"}, {13, "a[1]"}, {14, "b[0]"}, {15, "c[0]"}} {
		objs[n.obj] = fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Tx /T (%s) /Parent 10 0 R /DA (/F1 12 Tf 0 g) /Rect [72 %d 272 %d] /P 4 0 R >>", n.name, 72+30*i, 96+30*i)
	}
	if asArray {
		objs[12] = "[(preamble) 16 0 R (template) 17 0 R (datasets) 18 0 R]"
		objs[16], objs[17], objs[18] = stream("<xdp:xdp>"), stream(tpl), stream(data)
	} else {
		objs[12] = stream("<xdp:xdp>" + tpl + data + "</xdp:xdp>")
	}
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [10 0 R] /DA (/F1 0 Tf 0 g) /DR << /Font << /F1 7 0 R >> >> /XFA " + xfa + " >> >>"
	return assembleFixture(objs)
}

// `/pending 924`: an IRS form has no /TU; the printed line label is in its XFA template. A label is keyed by the
// field's indexed XFA path (the AcroForm name), taken from `speak`, else `toolTip`, else the caption, and only from
// inside `<template>`.
func TestAnXFAFormsFieldsAreLabelledFromItsTemplate(t *testing.T) {
	for _, asArray := range []bool{false, true} {
		got, err := ListFormFields(xfaFormToLabel(asArray))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{
			"form1[0].a[0]": "First a",  // speak, white space folded
			"form1[0].a[1]": "Second a", // same name, its own caption: the INDEX tells them apart
			"form1[0].b[0]": "The b",    // toolTip
			"form1[0].c[0]": "",         // the template names no label; the datasets decoy is not one
		}
		if len(got) != len(want) {
			t.Fatalf("array=%v: %d fields, want %d: %+v", asArray, len(got), len(want), got)
		}
		for _, f := range got {
			if w, ok := want[f.Name]; !ok || f.Label != w {
				t.Errorf("array=%v: %s label %q, want %q", asArray, f.Name, f.Label, w)
			}
		}
	}
}

func TestAFormWithNoXFAHasNoLabels(t *testing.T) {
	got, err := ListFormFields(formToList())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got {
		if f.Label != "" {
			t.Errorf("%s carries label %q without an XFA packet", f.Name, f.Label)
		}
	}
}
