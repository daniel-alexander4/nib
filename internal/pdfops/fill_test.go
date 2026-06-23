package pdfops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
)

// authorTestForm builds a one-page PDF carrying a text field "fullName" and a
// checkbox "agree" — a fillable form to exercise the fill paths against.
func authorTestForm(t *testing.T) []byte {
	t.Helper()
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 400, 600)})
	if err != nil {
		t.Fatal(err)
	}
	f, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{50, 500, 250, 520}, Kind: "text", Name: "fullName"},
		{Page: 1, Rect: [4]float64{50, 460, 70, 480}, Kind: "check", Name: "agree"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFillFormJSON(t *testing.T) {
	out, err := FillFormJSON(authorTestForm(t),
		[]byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Jane Doe"}],"checkbox":[{"name":"agree","value":true}]}]}`))
	if err != nil {
		t.Fatalf("FillFormJSON: %v", err)
	}
	j, err := ExportFormJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), "Jane Doe") {
		t.Fatalf("filled value missing; export=\n%s", j)
	}
}

func TestFillFormCSV(t *testing.T) {
	form0 := authorTestForm(t)
	csv := "fullName,agree,label\nJane Doe,true,jane\nJohn Roe,no,john\n"
	parts, err := FillFormCSV(form0, []byte(csv), "label")
	if err != nil {
		t.Fatalf("FillFormCSV: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(parts))
	}
	if parts[0].Name != "jane" || parts[1].Name != "john" {
		t.Fatalf("output names = %q, %q; want jane, john", parts[0].Name, parts[1].Name)
	}
	// Row 0: text filled, checkbox coerced true.
	cb0 := checkboxValue(t, parts[0].Data, "Jane Doe")
	if !cb0 {
		t.Fatal("row 0 checkbox should be checked (agree=true)")
	}
	// Row 1: checkbox coerced false ("no").
	if checkboxValue(t, parts[1].Data, "John Roe") {
		t.Fatal("row 1 checkbox should be unchecked (agree=no)")
	}
}

// checkboxValue re-exports a filled PDF, asserts wantText is present, and returns
// the "agree" checkbox's boolean value.
func checkboxValue(t *testing.T, pdf []byte, wantText string) bool {
	t.Helper()
	j, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), wantText) {
		t.Fatalf("text %q missing; export=\n%s", wantText, j)
	}
	var fg form.FormGroup
	if err := json.Unmarshal(j, &fg); err != nil {
		t.Fatal(err)
	}
	if len(fg.Forms) == 0 || len(fg.Forms[0].CheckBoxes) == 0 {
		t.Fatalf("no checkbox in export=\n%s", j)
	}
	return fg.Forms[0].CheckBoxes[0].Value
}

func TestFillFormCSVErrors(t *testing.T) {
	form0 := authorTestForm(t)
	if _, err := FillFormCSV(form0, []byte("fullName\n"), ""); err == nil {
		t.Error("header-only CSV (no data rows) should error")
	}
	if _, err := FillFormCSV(form0, []byte("fullName,agree\nx,y\n"), "missing"); err == nil {
		t.Error("--name-col naming a non-existent column should error")
	}
}

func TestTruthy(t *testing.T) {
	for _, s := range []string{"true", "T", "1", "Yes", "x", "checked", "ON"} {
		if !truthy(s) {
			t.Errorf("truthy(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "false", "no", "0", "off", "maybe"} {
		if truthy(s) {
			t.Errorf("truthy(%q) = true, want false", s)
		}
	}
}
