package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// `/pending 888` (5): the window can take out form fields that are on no page (ADR-166) and the command line
// could not — `nib sanitize` was one fixed pair of scrubs.
func TestSanitizeRemovesFormFieldsOnNoPageOnlyWhenAsked(t *testing.T) {
	// Two text fields in the form's list; only the first has its widget on the page. The second holds an entry.
	doc := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R 6 0 R] /DA (/Helv 0 Tf 0 g) /DR << /Font << /Helv 7 0 R >> >> >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Annots [5 0 R] >>",
		4: "<< /Length 12 >>\nstream\n0 0 1 1 re f\nendstream",
		5: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (shown) /V (kept) /Rect [72 700 300 720] /F 4 /P 3 0 R /DA (/Helv 12 Tf 0 g) >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (hidden) /V (typed where no page shows it) /Rect [72 660 300 680] /F 4 /DA (/Helv 12 Tf 0 g) >>",
		7: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	})
	offPage := func(pdf []byte) bool {
		t.Helper()
		rep, err := pdfops.Scan(pdf)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range rep.Findings {
			if f.Kind == "offPageFields" {
				return true
			}
		}
		return false
	}
	if !offPage(doc) {
		t.Fatal("setup: the scan finds no form field on no page in the fixture")
	}
	dir := t.TempDir()
	in := writeFile(t, dir, "in.pdf", string(doc))

	plain := filepath.Join(dir, "plain.pdf")
	if code := cmdSanitize([]string{in, "-o", plain}); code != 0 {
		t.Fatalf("sanitize exit = %d, want 0", code)
	}
	if !offPage(readPDF(t, plain)) {
		t.Error("sanitize without --fields removed a form field: a form may keep one with no box on purpose")
	}

	asked := filepath.Join(dir, "asked.pdf")
	if code := cmdSanitize([]string{"--fields", in, "-o", asked}); code != 0 {
		t.Fatalf("sanitize --fields exit = %d, want 0", code)
	}
	got := readPDF(t, asked)
	if offPage(got) {
		t.Error("sanitize --fields left a form field that is on no page")
	}
	// And the field that IS on a page is as it was.
	x, err := pdfops.ExportFormXFDF(got)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(x); !strings.Contains(s, "shown") || !strings.Contains(s, "kept") || strings.Contains(s, "hidden") {
		t.Errorf("after --fields the form reads:\n%s\nwant the field on the page with its value, and not the other", s)
	}
}
