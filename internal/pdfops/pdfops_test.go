package pdfops

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"nib/internal/testpdf"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{0, 128, 255, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImagesToPDF(t *testing.T) {
	pdf, err := ImagesToPDF([][]byte{pngBytes(t, 100, 140), pngBytes(t, 100, 140)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Errorf("ImagesToPDF did not produce a PDF: %.10q", pdf)
	}
}

func TestEncrypt(t *testing.T) {
	pdf, _ := testpdf.Form()
	enc, err := Encrypt(pdf, "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enc, pdf) {
		t.Error("encrypted output is identical to input")
	}
	if !bytes.HasPrefix(enc, []byte("%PDF")) {
		t.Error("encrypted output is not a PDF")
	}
}

func TestExportFormJSON(t *testing.T) {
	pdf, _ := testpdf.Form()
	data, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture defines a "fullName" text field; it must appear in the export.
	if !bytes.Contains(data, []byte("fullName")) {
		t.Errorf("form export missing field name; got: %s", data)
	}
}

func threePagePDF(t *testing.T) []byte {
	t.Helper()
	pdf, err := ImagesToPDF([][]byte{pngBytes(t, 80, 110), pngBytes(t, 80, 110), pngBytes(t, 80, 110)})
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func TestPageOps(t *testing.T) {
	pdf := threePagePDF(t)
	if n, _ := PageCount(pdf); n != 3 {
		t.Fatalf("base page count = %d, want 3", n)
	}

	removed, err := RemovePages(pdf, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(removed); n != 2 {
		t.Errorf("after remove: count = %d, want 2", n)
	}

	reordered, err := Reorder(pdf, []string{"3", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(reordered); n != 2 {
		t.Errorf("after reorder/collect: count = %d, want 2", n)
	}

	rotated, err := Rotate(pdf, []string{"1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(rotated); n != 3 {
		t.Errorf("after rotate: count = %d, want 3", n)
	}

	merged, err := Append(pdf, threePagePDF(t))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(merged); n != 6 {
		t.Errorf("after append: count = %d, want 6", n)
	}
}

func TestRedactRemovesContent(t *testing.T) {
	// The form fixture has a "fullName" field on page 1. Redacting that page must
	// replace it with a flat image, so the field is GONE, not merely covered.
	original, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if before, _ := ExportFormJSON(original); !bytes.Contains(before, []byte("fullName")) {
		t.Fatal("fixture should contain the fullName field before redaction")
	}

	redacted, err := RedactPages(original, map[int][]byte{1: pngBytes(t, 200, 280)})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(redacted); n != 1 {
		t.Errorf("redacted page count = %d, want 1", n)
	}
	after, _ := ExportFormJSON(redacted)
	if bytes.Contains(after, []byte("fullName")) {
		t.Error("redacted page still exposes the form field — content not truly removed")
	}
}

func TestRedactKeepsOtherPagesVector(t *testing.T) {
	original := threePagePDF(t)
	redacted, err := RedactPages(original, map[int][]byte{2: pngBytes(t, 80, 110)})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(redacted); n != 3 {
		t.Errorf("redacted doc page count = %d, want 3 (only page 2 replaced)", n)
	}
}

func TestStampFields(t *testing.T) {
	// One-page PDF from a blank image, then stamp a text field and a check.
	pdf, err := ImagesToPDF([][]byte{pngBytes(t, 400, 560)})
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampFields(pdf, []Field{
		{Page: 1, Rect: [4]float64{72, 700, 300, 716}, Text: "Jane Doe"},
		{Page: 1, Rect: [4]float64{72, 650, 86, 664}, Text: "X"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("StampFields did not produce a PDF: %.10q", out)
	}
	if len(out) <= len(pdf) {
		t.Error("stamped PDF is not larger than the original (nothing added?)")
	}
	// Empty fields are skipped, returning the input unchanged.
	same, err := StampFields(pdf, []Field{{Page: 1, Text: "   "}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(same, pdf) {
		t.Error("stamping only-empty fields should return the input unchanged")
	}
}
