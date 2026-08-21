package pdfops

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
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

// TestGridToODS confirms a valid ODF package: mimetype is the first entry and
// stored uncompressed (the ODF gotcha), and content.xml carries the escaped cells.
func TestGridToODS(t *testing.T) {
	got, err := GridToODS([][]string{{"a & b", "B1"}, {"", "x<y"}})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(got), int64(len(got)))
	if err != nil {
		t.Fatalf("output is not a zip: %v", err)
	}
	if len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		t.Fatalf("first entry must be mimetype, got %v", zr.File)
	}
	if zr.File[0].Method != zip.Store {
		t.Error("mimetype must be stored uncompressed")
	}
	// The mimetype's local header must carry its real size (no data-descriptor
	// flag) so a strict ODF reader can sniff it — Create streams with the bit set,
	// which LibreOffice rejects; CreateRaw clears it. Guard at the byte level.
	if got[0] != 'P' || got[1] != 'K' {
		t.Fatal("not a zip")
	}
	if flags := uint16(got[6]) | uint16(got[7])<<8; flags&0x08 != 0 {
		t.Errorf("mimetype local header has the data-descriptor flag set (0x%04x) — LibreOffice will reject it", flags)
	}
	if usize := uint32(got[22]) | uint32(got[23])<<8 | uint32(got[24])<<16 | uint32(got[25])<<24; usize == 0 {
		t.Error("mimetype local header uncompressed size is 0 (data-descriptor mode) — must be in the header")
	}
	want := map[string]bool{"mimetype": true, "META-INF/manifest.xml": true, "content.xml": true}
	var content string
	for _, f := range zr.File {
		delete(want, f.Name)
		if f.Name == "content.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			content = string(b)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing ODF parts: %v", want)
	}
	for _, w := range []string{"a &amp; b", "x&lt;y", `table:name="Sheet1"`} {
		if !strings.Contains(content, w) {
			t.Errorf("content.xml missing %q:\n%s", w, content)
		}
	}
}

func TestColName(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 51: "AZ", 52: "BA"} {
		if got := colName(i); got != want {
			t.Errorf("colName(%d) = %q, want %q", i, got, want)
		}
	}
}

// TestGridToCSVNeutralizesFormulas pins the one output format here that carries no
// type information. The grid is table text extracted from an untrusted PDF, so a
// cell beginning = + - @ becomes a live formula when the file is opened in Excel or
// LibreOffice. XLSX and ODS are inert by construction (t="inlineStr",
// office:value-type="string"); CSV was not.
func TestGridToCSVNeutralizesFormulas(t *testing.T) {
	out, err := GridToCSV([][]string{
		{"=HYPERLINK(\"http://evil\",\"click\")", "+1+1", "-2+3", "@SUM(A1)"},
		{"\t=cmd|' /c calc'!A0", "ordinary", "-", "12.50"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)

	// Every formula trigger must be quoted out. Asserted per cell rather than by
	// scanning for "=" anywhere: a value may legitimately contain one.
	for _, dangerous := range []string{"=HYPERLINK", "+1+1", "-2+3", "@SUM(A1)", "=cmd|"} {
		idx := strings.Index(got, dangerous)
		if idx < 1 {
			t.Fatalf("%q not found in output (or at position 0): %q", dangerous, got)
		}
		if got[idx-1] != '\'' {
			t.Errorf("cell %q is not neutralized — it would execute on open; output: %q", dangerous, got)
		}
	}

	// The positive control: ordinary values must survive untouched, or a guard that
	// prefixed everything would pass every assertion above.
	if !strings.Contains(got, "ordinary") || strings.Contains(got, "'ordinary") {
		t.Errorf("an ordinary cell was altered: %q", got)
	}
	if !strings.Contains(got, "12.50") || strings.Contains(got, "'12.50") {
		t.Errorf("a numeric cell was altered: %q", got)
	}
}

// TestFormCSVIsAsGuardedAsTableCSV.
//
// `csvSafe` exists with the argument written out — *"the grid is table text extracted from
// an UNTRUSTED PDF… a CSV cell beginning `= + - @` becomes a live formula the moment the
// file is opened in Excel or LibreOffice"* — and was applied by `GridToCSV` and tested with
// `=HYPERLINK` and `=cmd|' /c calc'!A0`. `ExportFormCSV` wrote field names and values raw.
// Those come from the AcroForm of an arbitrary opened PDF, which is exactly as untrusted as
// table text, and they go out as a download from both the GUI and `nib`.
//
// Asserted through csvSafe's own behaviour on the four lead characters rather than by
// building a hostile PDF, because the finding is that one emitter skipped a guard the other
// applies — a property of the call sites.
func TestFormCSVIsAsGuardedAsTableCSV(t *testing.T) {
	for _, c := range []string{
		"=HYPERLINK(\"http://evil\",\"click\")",
		"+1+1",
		"-1+1",
		"@SUM(A1)",
		"=cmd|' /c calc'!A0",
	} {
		got := csvSafe(c)
		if got == c {
			t.Errorf("csvSafe(%q) returned it unchanged — this cell is a live formula in "+
				"Excel and LibreOffice", c)
		}
	}
	// The control: ordinary values must survive untouched, or every exported form is
	// mangled to close a hole in a few of them.
	for _, c := range []string{"Jane Smith", "1 High Street", "2026-08-20", "", "12345"} {
		if got := csvSafe(c); got != c {
			t.Errorf("csvSafe(%q) = %q — an ordinary field value was altered", c, got)
		}
	}

	// And the call sites, which is where the defect actually was.
	src, err := os.ReadFile("pdfops.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func ExportFormCSV")
	if start < 0 {
		t.Fatal("ExportFormCSV is gone — this guard is looking for a function that does not exist")
	}
	end := strings.Index(body[start:], "\ncw.Flush()")
	fn := body[start:]
	if end > 0 {
		fn = body[start : start+end]
	}
	var writes, guarded int
	for _, line := range strings.Split(fn, "\n") {
		if !strings.Contains(line, "cw.Write(") {
			continue
		}
		writes++
		if strings.Contains(line, "csvSafe(") {
			guarded++
		} else if !strings.Contains(line, `"field", "value"`) {
			t.Errorf("ExportFormCSV writes an unguarded cell: %s", strings.TrimSpace(line))
		}
	}
	// The floor: this function writes a row per field kind, and zero means the scan
	// stopped matching and every assertion above ran over nothing.
	if writes < 5 {
		t.Fatalf("found %d cw.Write call(s) in ExportFormCSV; the matcher has gone blind", writes)
	}
	t.Logf("%d write sites, %d guarded", writes, guarded)
}
