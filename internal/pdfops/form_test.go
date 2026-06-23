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

// TestAuthorFormRadioVertical authors a vertical radio group in a tall box and
// confirms (a) it round-trips as a real choice field and (b) the buttons render
// anchored at the box's TOP and stacking downward — i.e. their ink clusters in
// the upper half of the box, not the lower. This pins pdfcpu's vertical-anchor
// convention (orientation:"vert" anchors at the top edge), the one geometry the
// horizontal-only v1 left unverified.
func TestAuthorFormRadioVertical(t *testing.T) {
	base, err := testpdf.Text("survey")
	if err != nil {
		t.Fatal(err)
	}
	// A deliberately tall, narrow box (height ≫ width) so three small buttons
	// stacked from the top leave clear slack at the bottom — the discriminator.
	const x0, y0, x1, y1 = 100.0, 560.0, 116.0, 700.0
	out, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{x0, y0, x1, y1}, Kind: "radio", Name: "plan",
			Options: []string{"Basic", "Pro", "Enterprise"}, Orientation: "vert"},
	})
	if err != nil {
		t.Fatalf("AuthorForm vertical radio: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("authored PDF does not validate: %v", err)
	}
	js, _ := ExportFormJSON(out)
	for _, want := range []string{"plan", "Basic", "Pro", "Enterprise"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("radio field/value %q missing from form export: %s", want, js)
		}
	}

	// Geometry check (needs poppler). testpdf pages are A4 portrait (842pt tall);
	// poppler renders at 36 DPI → 0.5 px/pt, top-left origin.
	skipNoPoppler(t)
	img := renderPDF(t, out, "vradio")[0]
	const scale = 0.5
	const pageH = 842.0
	// Box in image rows (top-left origin): box top (y1) is the smaller row.
	topRow := int((pageH - y1) * scale)
	botRow := int((pageH - y0) * scale)
	midRow := (topRow + botRow) / 2
	// Scan the button column band (a little padding around [x0,x1]) for ink.
	loCol, hiCol := int(x0*scale)-2, int(x1*scale)+2
	var sumRow, n int
	for row := topRow; row <= botRow; row++ {
		for col := loCol; col <= hiCol; col++ {
			r, g, b, _ := img.At(col, row).RGBA()
			if r+g+b < 384*257 { // dark (RGBA is 16-bit; 384 of 765 in 8-bit terms)
				sumRow += row
				n++
			}
		}
	}
	if n == 0 {
		t.Fatal("found no radio-button ink inside the box — vertical group did not render where expected")
	}
	if meanRow := sumRow / n; meanRow > midRow {
		t.Errorf("vertical radio ink centred at row %d, below box mid %d — buttons did not anchor at the top edge", meanRow, midRow)
	}
}
