package cli

import (
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// /pending 889 part 2: the command line's door for the Web Capture removal. A record Nib could not read whole
// refuses every other rewrite, so `nib sanitize` alone fails on it, and --web-capture removes it FIRST.
func TestSanitizeRemovesAWebCaptureRecordOnlyWhenAsked(t *testing.T) {
	w := testpdf.WholeWebCapture
	w.Refused = true
	dir := t.TempDir()
	in := writeFile(t, dir, "in.pdf", string(testpdf.WebCaptured(w)))
	has := func(pdf []byte) bool {
		t.Helper()
		rep, err := pdfops.Scan(pdf)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range rep.Findings {
			if f.Kind == "webCapture" {
				return true
			}
		}
		return false
	}
	if code := cmdSanitize([]string{in, "-o", filepath.Join(dir, "plain.pdf")}); code == 0 {
		t.Error("sanitize without --web-capture succeeded over a record Nib could not read whole — the write door should refuse it")
	}
	asked := filepath.Join(dir, "asked.pdf")
	if code := cmdSanitize([]string{"--web-capture", in, "-o", asked}); code != 0 {
		t.Fatalf("sanitize --web-capture exit = %d, want 0", code)
	}
	if has(readPDF(t, asked)) {
		t.Error("sanitize --web-capture left the record")
	}
}
