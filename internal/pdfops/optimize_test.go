package pdfops

import (
	"bytes"
	"testing"
)

// TestOptimize: optimization returns a valid PDF with the same page count and a
// readable %PDF header — lossless structural repacking, no content change.
func TestOptimize(t *testing.T) {
	pdf := threePagePDF(t)
	out, err := Optimize(pdf)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("Optimize did not produce a PDF: %.10q", out)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("optimized PDF does not validate: %v", err)
	}
	n, err := PageCount(out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("optimized page count = %d, want 3 (lossless)", n)
	}
}
