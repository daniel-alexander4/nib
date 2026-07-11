package mdpdf

import (
	"bytes"
	"encoding/binary"
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

// tiffFixture hand-writes a little-endian uncompressed multi-page TIFF (8x8
// gray pages, IFDs chained) — image/tiff writers can't produce multi-page
// files, and faxed exhibits arrive exactly in this shape.
func tiffFixture(t *testing.T, pages int) []byte {
	t.Helper()
	var b bytes.Buffer
	w16 := func(v uint16) { binary.Write(&b, binary.LittleEndian, v) }
	w32 := func(v uint32) { binary.Write(&b, binary.LittleEndian, v) }
	const w, h = 8, 8
	const entries = 8
	ifdSize := uint32(2 + entries*12 + 4)
	dataStart := uint32(8)
	firstIFD := dataStart + uint32(pages)*w*h
	b.WriteString("II")
	w16(42)
	w32(firstIFD)
	for p := 0; p < pages; p++ {
		pix := make([]byte, w*h)
		for i := range pix {
			pix[i] = byte(40 * (p + 1))
		}
		b.Write(pix)
	}
	entry := func(tag, typ uint16, count, val uint32) { w16(tag); w16(typ); w32(count); w32(val) }
	for p := 0; p < pages; p++ {
		next := uint32(0)
		if p < pages-1 {
			next = firstIFD + uint32(p+1)*ifdSize
		}
		w16(entries)
		entry(256, 3, 1, w)                       // ImageWidth
		entry(257, 3, 1, h)                       // ImageLength
		entry(258, 3, 1, 8)                       // BitsPerSample
		entry(259, 3, 1, 1)                       // Compression: none
		entry(262, 3, 1, 1)                       // Photometric: BlackIsZero
		entry(273, 4, 1, dataStart+uint32(p)*w*h) // StripOffsets
		entry(278, 3, 1, h)                       // RowsPerStrip
		entry(279, 4, 1, w*h)                     // StripByteCounts
		w32(next)
	}
	return b.Bytes()
}

// A multi-page TIFF yields one PDF page per TIFF page — pdfcpu walks the
// whole IFD chain. This pins the behavior so a pdfcpu upgrade can't silently
// regress to first-page-only (faxed evidence is routinely multi-page TIFF).
func TestImageToPDFMultiPageTIFF(t *testing.T) {
	out, err := ImageToPDF(tiffFixture(t, 3))
	if err != nil {
		t.Fatalf("ImageToPDF: %v", err)
	}
	checkPacket(t, out, 3)

	cover := render(t, "# Appeal\n\nSee attached fax.")
	packet, skipped, err := AssemblePacket(cover, [][]byte{tiffFixture(t, 2)})
	if err != nil {
		t.Fatalf("AssemblePacket: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped = %v, want none", skipped)
	}
	checkPacket(t, packet, 3) // 1 cover + 2 TIFF pages
}

// Every emitted page — markdown text pages and image pages alike — is US
// Letter (612x792 pt): the mdpdf default for US business/medical
// correspondence. layout.go's wrap math and spec()'s paper name must agree.
func TestPagesAreUSLetter(t *testing.T) {
	for name, pdf := range map[string][]byte{
		"convert": render(t, "# Letter\n\nBody."),
		"image":   mustImageToPDF(t, pngFixture(t)),
	} {
		dims, err := api.PageDims(bytes.NewReader(pdf), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%s: PageDims: %v", name, err)
		}
		for i, d := range dims {
			if d.Width != 612 || d.Height != 792 {
				t.Errorf("%s page %d = %.0fx%.0f pt, want 612x792 (US Letter)", name, i+1, d.Width, d.Height)
			}
		}
	}
}

func mustImageToPDF(t *testing.T, img []byte) []byte {
	t.Helper()
	out, err := ImageToPDF(img)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
