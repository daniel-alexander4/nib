package pdfops

import (
	"encoding/xml"
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
