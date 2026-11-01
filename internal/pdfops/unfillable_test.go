package pdfops

import (
	"nib/internal/testpdf"
	"reflect"
	"slices"
	"testing"
)

// TestUnfillableFieldsNamesWhatTheFillSetsAside — /pending 919 F14. A fill silently drops a value given to a field
// whose widget's `/Parent` leads nowhere (`unwalkableWidgets`), so the names it sets aside are reported: the
// widget's own `/T` for a bare one, the dotted partials from the break down for one under containers, nothing for a
// chain past the walk's bound (pdfcpu refuses that document rather than skipping a field) and nothing for a form
// with no such widget.
func TestUnfillableFieldsNamesWhatTheFillSetsAside(t *testing.T) {
	for _, c := range []struct {
		name string
		pdf  []byte
		want []string
	}{
		{"a bare orphan", formWithAWidgetWhoseParentIsEmpty(), []string{"orphan"}},
		{"under three containers", formWithAParentChain(3), []string{"c2.c1.c0.orphan"}},
		{"a chain past the bound", formWithAParentChain(60), nil},
	} {
		got, err := UnfillableFields(c.pdf)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: UnfillableFields = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestUnfillableFieldsIsEmptyForAFormWithNoSuchWidget is the other side: a form whose every widget reaches its
// field reports nothing, so the stderr line of `nib fill` appears only when it is true.
func TestUnfillableFieldsIsEmptyForAFormWithNoSuchWidget(t *testing.T) {
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R] >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /V (before) /DA (/Helv 10 Tf 0 g) /Rect [10 10 150 30] /P 3 0 R >>",
	})
	got, err := UnfillableFields(pdf)
	if err != nil || len(got) != 0 {
		t.Errorf("UnfillableFields = %q, %v; want none", got, err)
	}
}

// TestJSONFieldNamesAreWhatTheFillIsAskedToSet — /pending 919. A record names its fields by id or name in any of the
// six kinds; only forms[0] is read; sorted, each once; unreadable names none.
func TestJSONFieldNamesAreWhatTheFillIsAskedToSet(t *testing.T) {
	rec := `{"forms":[{"textfield":[{"id":"1","name":"b"},{"id":"2","name":"a"}],"checkbox":[{"name":"c"}],` +
		`"radiobuttongroup":[{"name":"r"}],"combobox":[{"name":"m"}],"listbox":[{"name":"l"}],"datefield":[{"name":"d"}]},` +
		`{"textfield":[{"name":"second"}]}]}`
	got := JSONFieldNames([]byte(rec))
	want := []string{"1", "2", "a", "b", "c", "d", "l", "m", "r"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("names = %q, want %q (forms[1] is not read)", got, want)
	}
	for _, bad := range []string{"", "not json", `{"forms":[]}`} {
		if n := JSONFieldNames([]byte(bad)); len(n) != 0 {
			t.Errorf("%q named %q, want none", bad, n)
		}
	}
}
