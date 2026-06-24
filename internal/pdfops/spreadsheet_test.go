package pdfops

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestGridToCSV(t *testing.T) {
	got, err := GridToCSV([][]string{{"Name", "Total"}, {"Ada, L.", "1,000"}})
	if err != nil {
		t.Fatal(err)
	}
	// encoding/csv quotes cells containing commas.
	want := "Name,Total\n\"Ada, L.\",\"1,000\"\n"
	if string(got) != want {
		t.Errorf("CSV = %q, want %q", got, want)
	}
}

// TestGridToXLSX confirms the output is a valid zip whose worksheet carries the
// cell text (XML-escaped), i.e. a structurally sound .xlsx Excel/LibreOffice open.
func TestGridToXLSX(t *testing.T) {
	got, err := GridToXLSX([][]string{{"a & b", "B1"}, {"", "x<y"}})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(got), int64(len(got)))
	if err != nil {
		t.Fatalf("output is not a zip: %v", err)
	}
	want := map[string]bool{
		"[Content_Types].xml": true, "_rels/.rels": true, "xl/workbook.xml": true,
		"xl/_rels/workbook.xml.rels": true, "xl/worksheets/sheet1.xml": true,
	}
	var sheet string
	for _, f := range zr.File {
		delete(want, f.Name)
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			sheet = string(b)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing OOXML parts: %v", want)
	}
	for _, w := range []string{`r="A1"`, `a &amp; b`, `r="B2"`, `x&lt;y`} {
		if !strings.Contains(sheet, w) {
			t.Errorf("sheet missing %q:\n%s", w, sheet)
		}
	}
	// Empty cell A2 is omitted (only B2 present in row 2).
	if strings.Contains(sheet, `r="A2"`) {
		t.Errorf("empty cell A2 should be omitted:\n%s", sheet)
	}
}

func TestColName(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 51: "AZ", 52: "BA"} {
		if got := colName(i); got != want {
			t.Errorf("colName(%d) = %q, want %q", i, got, want)
		}
	}
}
