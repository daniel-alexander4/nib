package pdfops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestStampTextLayerSearchable proves the OCR text layer is (a) EXTRACTABLE —
// pdftotext recovers the words — and (b) UNICODE-CORRECT: an accented name, smart
// quotes, an em-dash, and an ellipsis all survive. These are all > U+00FF; the
// core-font (Helvetica) path would have truncated each to a space, so this is the
// regression that pins the Roboto-user-font choice.
func TestStampTextLayerSearchable(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	base, err := testpdf.Text("scan") // a one-page PDF standing in for the scanned image
	if err != nil {
		t.Fatal(err)
	}
	words := []Word{
		{Page: 1, Rect: [4]float64{50, 120, 150, 132}, Text: "Hello"},
		{Page: 1, Rect: [4]float64{50, 100, 150, 112}, Text: "café"},
		{Page: 1, Rect: [4]float64{50, 80, 250, 92}, Text: "“smart”"},
		{Page: 1, Rect: [4]float64{50, 60, 200, 72}, Text: "em—dash"},
		{Page: 1, Rect: [4]float64{50, 40, 200, 52}, Text: "more…"},
	}
	out, err := StampTextLayer(base, words)
	if err != nil {
		t.Fatalf("StampTextLayer: %v", err)
	}
	txt := pdfToText(t, out)
	for _, want := range []string{"Hello", "café", "“smart”", "em—dash", "more…"} {
		if !strings.Contains(txt, want) {
			t.Errorf("extracted text missing %q (Unicode lost?)\n--- pdftotext gave ---\n%s", want, txt)
		}
	}
}

// TestStampTextLayerNoWords is a no-op pass-through (nothing to OCR on a page).
func TestStampTextLayerNoWords(t *testing.T) {
	base, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampTextLayer(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(base) {
		t.Errorf("no-words output changed: got %d bytes, want %d (pass-through)", len(out), len(base))
	}
}

func pdfToText(t *testing.T, pdf []byte) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "in.pdf")
	if err := os.WriteFile(src, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("pdftotext", "-enc", "UTF-8", src, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	return string(out)
}
