package pdfops

import (
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestAuthorForm authors a text field + checkbox onto an existing (content-bearing)
// PDF and proves they come back as real, readable AcroForm fields with the
// original page content preserved.
func TestAuthorForm(t *testing.T) {
	base, err := testpdf.Text("hello world") // a one-page PDF with known body text
	if err != nil {
		t.Fatal(err)
	}
	out, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 600, 300, 620}, Kind: "text", Name: "full_name"},
		{Page: 1, Rect: [4]float64{100, 560, 112, 572}, Kind: "check", Name: "agree"},
	})
	if err != nil {
		t.Fatalf("AuthorForm: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("authored PDF does not validate: %v", err)
	}

	// The fields are real AcroForm fields (readable via the export path).
	js, err := ExportFormJSON(out)
	if err != nil {
		t.Fatalf("ExportFormJSON: %v", err)
	}
	if !strings.Contains(string(js), "full_name") || !strings.Contains(string(js), "agree") {
		t.Errorf("authored fields missing from form export: %s", js)
	}

	// Page count unchanged — fields land on the existing page, not a new one.
	if n, _ := PageCount(out); n != 1 {
		t.Errorf("page count = %d, want 1 (fields must not add a page)", n)
	}
}

// TestAuthorFormDropdown authors a combobox (dropdown) with options and proves it
// round-trips as a real choice field; an option-less dropdown is rejected.
func TestAuthorFormDropdown(t *testing.T) {
	base, err := testpdf.Text("pick one")
	if err != nil {
		t.Fatal(err)
	}
	out, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 600, 260, 618}, Kind: "dropdown", Name: "color", Options: []string{"Red", "Green", "Blue"}},
	})
	if err != nil {
		t.Fatalf("AuthorForm dropdown: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("authored PDF does not validate: %v", err)
	}
	js, _ := ExportFormJSON(out)
	if s := string(js); !strings.Contains(s, "color") || !strings.Contains(s, "Green") {
		t.Errorf("dropdown field/options missing from form export: %s", s)
	}

	// An option-less dropdown is rejected (pdfcpu requires ≥1 option).
	if _, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 600, 260, 618}, Kind: "dropdown", Name: "empty"},
	}); err == nil {
		t.Error("expected an error for a dropdown with no options")
	}
}

// TestAuthorFormRadio authors a radio button group (≥2 values) and proves it
// round-trips as a real choice field; a group with fewer than two values is
// rejected (pdfcpu requires ≥2).
func TestAuthorFormRadio(t *testing.T) {
	base, err := testpdf.Text("survey")
	if err != nil {
		t.Fatal(err)
	}
	out, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 600, 220, 614}, Kind: "radio", Name: "plan", Options: []string{"Basic", "Pro", "Enterprise"}},
	})
	if err != nil {
		t.Fatalf("AuthorForm radio: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("authored PDF does not validate: %v", err)
	}
	js, _ := ExportFormJSON(out)
	s := string(js)
	for _, want := range []string{"plan", "Basic", "Pro", "Enterprise"} {
		if !strings.Contains(s, want) {
			t.Errorf("radio field/value %q missing from form export: %s", want, s)
		}
	}

	// A radio group with fewer than two options is rejected.
	if _, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{100, 600, 220, 614}, Kind: "radio", Name: "lonely", Options: []string{"only"}},
	}); err == nil {
		t.Error("expected an error for a radio group with <2 options")
	}
}
