// SPDX-License-Identifier: AGPL-3.0-or-later

package mdpdf

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"slices"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func pngFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func jpegFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 8, 8))
	img.SetGray(3, 3, color.Gray{Y: 128})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// checkPacket asserts out is a valid PDF with the expected page count.
func checkPacket(t *testing.T, out []byte, wantPages int) {
	t.Helper()
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("packet is not a PDF")
	}
	if err := api.Validate(bytes.NewReader(out), model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("invalid packet: %v", err)
	}
	if n := pageCount(t, out); n != wantPages {
		t.Fatalf("got %d pages, want %d", n, wantPages)
	}
}

func TestAssemblePacket(t *testing.T) {
	cover := render(t, "# Appeal\n\nDear Sir or Madam, please reconsider.")
	pdfA := render(t, "# Exhibit A\n\nLab results.")
	pdfB := render(t, "# Exhibit B\n\nReferral letter.")
	garbage := []byte("not a pdf")

	tests := []struct {
		name      string
		exhibits  [][]byte
		wantPages int
		wantSkip  []int
	}{
		{"cover only", nil, 1, nil},
		{"two pdfs", [][]byte{pdfA, pdfB}, 3, nil},
		{"pdf image garbage", [][]byte{pdfA, pngFixture(t), garbage}, 3, []int{2}},
		{"jpeg exhibit", [][]byte{jpegFixture(t)}, 2, nil},
		{"all garbage", [][]byte{garbage, []byte{}}, 1, []int{0, 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, skipped, err := AssemblePacket(cover, tc.exhibits)
			if err != nil {
				t.Fatalf("AssemblePacket: %v", err)
			}
			if !slices.Equal(skipped, tc.wantSkip) {
				t.Fatalf("skipped = %v, want %v", skipped, tc.wantSkip)
			}
			checkPacket(t, out, tc.wantPages)
		})
	}
}

func TestAssemblePacketBadCover(t *testing.T) {
	for _, cover := range [][]byte{[]byte("not a pdf"), nil} {
		if _, _, err := AssemblePacket(cover, nil); err == nil {
			t.Fatalf("AssemblePacket(%q, nil): want error, got nil", cover)
		}
	}
}

func TestImageToPDF(t *testing.T) {
	out, err := ImageToPDF(pngFixture(t))
	if err != nil {
		t.Fatalf("ImageToPDF: %v", err)
	}
	checkPacket(t, out, 1)

	if _, err := ImageToPDF([]byte("not an image")); err == nil {
		t.Fatal("ImageToPDF(garbage): want error, got nil")
	}
}
