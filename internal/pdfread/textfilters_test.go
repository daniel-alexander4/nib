package pdfread_test

import (
	"bytes"
	"context"
	"encoding/ascii85"
	"encoding/hex"
	"image"
	_ "image/png"
	"io"
	"strconv"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// palettePicture is a one-page document drawing a 4×2 picture in a four-colour /Indexed palette (black, red, green,
// blue), its samples encoded by filter — the shape Pillow writes for every palette image it saves as a PDF.
func palettePicture(filter string, data []byte) []byte {
	content := "q 200 0 0 100 0 0 cm /Im0 Do Q"
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << /XObject << /Im0 5 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length " + strconv.Itoa(len(content)) + " >>\nstream\n" + content + "\nendstream",
		5: "<< /Type /XObject /Subtype /Image /Width 4 /Height 2 /BitsPerComponent 8 /ColorSpace [/Indexed /DeviceRGB 3 <000000FF000000FF000000FF>] " +
			filter + " /Length " + strconv.Itoa(len(data)) + " >>\nstream\n" + string(data) + "\nendstream",
	})
}

var paletteSamples = []byte{0, 1, 2, 3, 3, 2, 1, 0}

// wantPalettePixels is paletteSamples looked up in the palette, as 8-bit RGB.
func wantPalettePixels(t *testing.T, png []byte) {
	t.Helper()
	im, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("the extracted picture is not an image: %v", err)
	}
	pal := [][3]uint32{{0, 0, 0}, {255, 0, 0}, {0, 255, 0}, {0, 0, 255}}
	if b := im.Bounds(); b.Dx() != 4 || b.Dy() != 2 {
		t.Fatalf("extracted picture is %v, want 4×2", b)
	}
	for i, s := range paletteSamples {
		r, g, b, _ := im.At(i%4, i/4).RGBA()
		if got := [3]uint32{r >> 8, g >> 8, b >> 8}; got != pal[s] {
			t.Fatalf("pixel %d is %v, want palette entry %d %v", i, got, s, pal[s])
		}
	}
}

// TestAPictureWhoseFiltersEndInATextFilterIsExtracted — /pending 982. pdfcpu refuses to extract an image whose last
// filter is ASCIIHexDecode or ASCII85Decode, so a palette scan Pillow wrote came back "could not extract images from
// any page" from `nib images`, the window's image archive and the document export. Both of this package's image doors
// must give back the picture with its palette's colours.
func TestAPictureWhoseFiltersEndInATextFilterIsExtracted(t *testing.T) {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	w.Write(paletteSamples)
	w.Close()
	for _, c := range []struct{ name, filter, data string }{
		{"hex", "/Filter /ASCIIHexDecode", hex.EncodeToString(paletteSamples) + ">"},
		{"ascii85", "/Filter /ASCII85Decode", a85.String() + "~>"},
		{"hex in an array", "/Filter [/ASCIIHexDecode]", hex.EncodeToString(paletteSamples) + ">"},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := palettePicture(c.filter, []byte(c.data))
			var got [][]byte
			if err := pdfread.ExtractImages(doc, nil, func(img model.Image, _ bool, _ int) error {
				b, err := io.ReadAll(img)
				got = append(got, b)
				return err
			}); err != nil {
				t.Fatalf("ExtractImages: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("ExtractImages gave %d pictures, want 1", len(got))
			}
			wantPalettePixels(t, got[0])

			got = nil
			skipped, err := pdfread.ExtractImagesEach(context.Background(), doc, nil, 1<<20, func(img model.Image) error {
				b, err := io.ReadAll(img)
				got = append(got, b)
				return err
			})
			if err != nil || skipped != 0 || len(got) != 1 {
				t.Fatalf("ExtractImagesEach: %d pictures, %d skipped, err %v; want 1, 0, nil", len(got), skipped, err)
			}
			wantPalettePixels(t, got[0])
		})
	}
}
