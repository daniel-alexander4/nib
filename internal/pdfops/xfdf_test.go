package pdfops

import (
	"encoding/xml"
	"regexp"
	"strings"
	"testing"
)

// TestFillFormXFDF fills a form from a hand-written XFDF and confirms the text and
// checkbox values land (the checkbox "Yes" on-state reads back as checked).
func TestFillFormXFDF(t *testing.T) {
	form0 := authorTestForm(t)
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/">
  <fields>
    <field name="fullName"><value>Jane Doe</value></field>
    <field name="agree"><value>Yes</value></field>
  </fields>
</xfdf>`)
	out, err := FillFormXFDF(form0, data)
	if err != nil {
		t.Fatalf("FillFormXFDF: %v", err)
	}
	if !checkboxValue(t, out, "Jane Doe") {
		t.Fatal("checkbox should be checked (agree=Yes)")
	}
}

// TestExportFormXFDFRoundTrip fills a form, exports its data as XFDF, and parses
// that XFDF back — the values must survive both directions.
func TestExportFormXFDFRoundTrip(t *testing.T) {
	form0 := authorTestForm(t)
	filled, err := FillFormJSON(form0,
		[]byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Ada Lovelace"}],"checkbox":[{"name":"agree","value":true}]}]}`))
	if err != nil {
		t.Fatalf("FillFormJSON: %v", err)
	}

	x, err := ExportFormXFDF(filled)
	if err != nil {
		t.Fatalf("ExportFormXFDF: %v", err)
	}
	xs := string(x)
	if !strings.HasPrefix(xs, xml.Header) {
		t.Errorf("missing XML header:\n%s", xs)
	}
	for _, want := range []string{`xmlns="http://ns.adobe.com/xfdf/"`, `name="fullName"`, `<value>Ada Lovelace</value>`, `name="agree"`, `<value>Yes</value>`} {
		if !strings.Contains(xs, want) {
			t.Errorf("export missing %q:\n%s", want, xs)
		}
	}

	values, err := parseXFDF(x)
	if err != nil {
		t.Fatalf("parseXFDF: %v", err)
	}
	if got := values["fullName"]; len(got) != 1 || got[0] != "Ada Lovelace" {
		t.Errorf("fullName = %q, want [Ada Lovelace]", got)
	}
	if got := values["agree"]; len(got) != 1 || got[0] != "Yes" {
		t.Errorf("agree = %q, want [Yes]", got)
	}
}

// TestParseXFDF covers the parser's three shapes: nested hierarchical names, flat
// dotted names, and multi-value fields — all reduce to fully-qualified keys.
func TestParseXFDF(t *testing.T) {
	data := []byte(`<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields>
	  <field name="address"><field name="city"><value>London</value></field></field>
	  <field name="a.b"><value>flat</value></field>
	  <field name="toppings"><value>cheese</value><value>olives</value></field>
	</fields></xfdf>`)
	values, err := parseXFDF(data)
	if err != nil {
		t.Fatalf("parseXFDF: %v", err)
	}
	if got := values["address.city"]; len(got) != 1 || got[0] != "London" {
		t.Errorf("address.city = %q, want [London]", got)
	}
	if got := values["a.b"]; len(got) != 1 || got[0] != "flat" {
		t.Errorf("a.b = %q, want [flat]", got)
	}
	if got := values["toppings"]; len(got) != 2 || got[0] != "cheese" || got[1] != "olives" {
		t.Errorf("toppings = %q, want [cheese olives]", got)
	}
}

// TestExportFormXFDFHierarchical confirms a dotted field name is serialized as
// nested <field> elements (the spec-correct, portable shape).
func TestExportFormXFDFHierarchical(t *testing.T) {
	var root xfdfField
	insertField(&root, []string{"address", "city"}, []string{"London"})
	insertField(&root, []string{"address", "zip"}, []string{"EC1A"})
	body, err := xml.MarshalIndent(xfdfDoc{Xmlns: xfdfNS, Fields: root.Fields}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	xs := string(body)
	if !strings.Contains(xs, `<field name="address">`) || !strings.Contains(xs, `<field name="city">`) {
		t.Errorf("expected nested address/city fields:\n%s", xs)
	}
	// One <address> parent holds both children (not two separate address blocks).
	if n := strings.Count(xs, `name="address"`); n != 1 {
		t.Errorf("address appears %d times, want 1 (merged parent):\n%s", n, xs)
	}
}

func TestFillFormXFDFErrors(t *testing.T) {
	form0 := authorTestForm(t)
	if _, err := FillFormXFDF(form0, []byte(`<xfdf><fields></fields></xfdf>`)); err == nil {
		t.Error("XFDF with no field values should error")
	}
	if _, err := FillFormXFDF(form0, []byte(`<xfdf><fields><field`)); err == nil {
		t.Error("malformed XFDF should error")
	}
}

// A field carrying BOTH a value and qualified children keeps its own value.
//
// flattenField returned early as soon as it saw children, so the parent's value was
// dropped silently — and XFDF permits exactly that shape. Silent is the operative word:
// the import reported success and the field simply did not fill.
func TestFlattenFieldKeepsAParentsOwnValue(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/">
  <fields>
    <field name="address">
      <value>1 High Street</value>
      <field name="city"><value>Ipswich</value></field>
    </field>
  </fields>
</xfdf>`)
	got, err := parseXFDF(xml)
	if err != nil {
		t.Fatal(err)
	}
	// The child, which always worked — the stimulus that says the parse ran at all.
	if v := got["address.city"]; len(v) == 0 || v[0] != "Ipswich" {
		t.Fatalf("setup: the qualified child did not parse; got %#v", got)
	}
	if v := got["address"]; len(v) == 0 || v[0] != "1 High Street" {
		t.Errorf("the parent field's own value was dropped because it also has children; got %#v", got)
	}
}

// TestAnXFDFCheckboxValueIsAnExportNameNotABoolean — /pending 9.
//
// XFDF is how Acrobat and Foxit hand form data to another program, and a checkbox's `<value>`
// there is its EXPORT-VALUE NAME — whatever name the form chose for its on-state. `Off` is the
// one name PDF reserves for unchecked. Nib read that value with `truthy`, a CSV cell reader that
// accepts eight English words, so a form whose on-state was anything else (a German `Ja`, a
// numbered choice, a form's own label) came back UNCHECKED and the fill reported success.
//
// The value string only decides checked-ness: pdfcpu resolves the actual on-state name from the
// widget's appearance dictionary when it writes, so this test does not need a form with an exotic
// on-state to exercise the defect — it needs a value the CSV rule rejects, which is the class.
func TestAnXFDFCheckboxValueIsAnExportNameNotABoolean(t *testing.T) {
	form := authorTestForm(t)

	// SETUP: the CSV rule really does reject this value, or the row proves nothing about which
	// rule the XFDF path uses.
	if truthy("Ja") {
		t.Fatal("setup: the CSV reader already accepts \"Ja\", so this value cannot show the split")
	}

	filled, err := FillFormXFDF(form, []byte(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields>`+
			// The text field is not decoration. With only the checkbox in the document, an
			// unchecked result changes nothing and pdfcpu refuses the whole fill with "no form
			// fields affected" — so the defect would surface as an ERROR. Its real shape is
			// SILENT: any real export carries other fields, they apply, the fill reports
			// success, and the box is quietly wrong. This makes the row demonstrate that.
			`<field name="fullName"><value>Ada Lovelace</value></field>`+
			`<field name="agree"><value>Ja</value></field>`+
			`</fields></xfdf>`))
	if err != nil {
		t.Fatalf("FillFormXFDF: %v", err)
	}
	out, err := ExportFormXFDF(filled)
	if err != nil {
		t.Fatalf("ExportFormXFDF: %v", err)
	}
	// SETUP: the rest of the fill landed, so a wrong checkbox below is a wrong ANSWER rather
	// than a refused operation — which is the defect's real shape.
	if !regexp.MustCompile(`(?s)<field name="fullName">\s*<value>Ada Lovelace</value>`).Match(out) {
		t.Fatalf("setup: the text field did not fill, so this row is not exercising a silent "+
			"checkbox failure. export=%s", out)
	}
	agreeOn := regexp.MustCompile(`(?s)<field name="agree">\s*<value>Yes</value>`)
	if !agreeOn.Match(out) {
		t.Errorf("a checkbox given its export-value name came back unchecked — the XFDF path is "+
			"reading a form-data name with the CSV cell reader. export=%s", out)
	}
}

// TestACSVCheckboxCellIsStillABoolean — the control, and it is why the rule is a parameter
// rather than a widened shared predicate.
//
// Under the XFDF rule anything but empty and `Off` means checked. Applied to CSV that would make
// a human's "no" mean YES — a silent, confident wrong answer on a mail merge. The two callers
// pass their own rule; this row fails the day someone collapses them.
func TestACSVCheckboxCellIsStillABoolean(t *testing.T) {
	for _, no := range []string{"no", "No", "false", "0", ""} {
		if truthy(no) {
			t.Errorf("the CSV reader treats %q as checked", no)
		}
		if no != "" && !xfdfChecked(no) {
			t.Errorf("the XFDF reader treats the export name %q as unchecked — only Off and empty are", no)
		}
	}
	if xfdfChecked("Off") || xfdfChecked("off") || xfdfChecked("  ") {
		t.Error("Off is the one name PDF reserves for the unchecked state")
	}
}
