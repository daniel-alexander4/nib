package pdfops

import (
	"archive/zip"
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// pagePNG is a w×h page whose pixel (x,y) is a function of the page number, so a page that came back as another
// page (or a flipped one) is a different picture.
func pagePNG(t *testing.T, n, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(40 * n), uint8(x * 255 / w), uint8(y * 255 / h), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestTheTIFFWritesEveryPageAndNibsOwnReaderWalksIt(t *testing.T) {
	pages := [][]byte{pagePNG(t, 1, 50, 40), pagePNG(t, 2, 33, 70), pagePNG(t, 3, 600, 500)}
	res, err := EncodePageImages(PageImagesTIFF, pages, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := walkTIFF(res.Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("a 3-page export walked as %d pages", len(got))
	}
	// The pixels survive: decode each page with the library reader at its own directory offset.
	first, err := tiff.Decode(bytes.NewReader(res.Data))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := png.Decode(bytes.NewReader(pages[0]))
	for _, p := range [][2]int{{0, 0}, {49, 39}, {10, 20}} {
		wr, wg, wb, _ := want.At(p[0], p[1]).RGBA()
		gr, gg, gb, _ := first.At(p[0], p[1]).RGBA()
		if wr != gr || wg != gg || wb != gb {
			t.Errorf("page 1 pixel %v: want %v,%v,%v got %v,%v,%v", p, wr, wg, wb, gr, gg, gb)
		}
	}
}

// An independent reader: Pillow decodes the same bytes. Skips (named) where it is absent.
func TestAnotherLibraryReadsTheTIFFPageForPage(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 absent")
	}
	if err := exec.Command(py, "-c", "import PIL").Run(); err != nil {
		t.Skip("Pillow absent")
	}
	pages := [][]byte{pagePNG(t, 1, 50, 40), pagePNG(t, 2, 33, 70), pagePNG(t, 3, 700, 900)}
	res, err := EncodePageImages(PageImagesTIFF, pages, []float64{100, 66, 700})
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "p.tiff")
	if err := os.WriteFile(f, res.Data, 0o600); err != nil {
		t.Fatal(err)
	}
	script := `import sys
from PIL import Image
im = Image.open(sys.argv[1])
out = []
for i in range(im.n_frames):
    im.seek(i)
    out.append("%dx%d:%s:%s" % (im.width, im.height, im.mode, im.getpixel((1, 1))))
print(";".join(out))
print(im.info.get("dpi"))`
	o, err := exec.Command(py, "-I", "-c", script, f).CombinedOutput()
	if err != nil {
		t.Fatalf("Pillow refused the TIFF: %v\n%s", err, o)
	}
	if !bytes.Contains(o, []byte("50x40:RGB")) || !bytes.Contains(o, []byte("33x70:RGB")) || !bytes.Contains(o, []byte("700x900:RGB")) {
		t.Errorf("Pillow read %s", o)
	}
	// Pillow leaves the last page current: 700 px over 700 pt is 72 dpi.
	if !bytes.Contains(o, []byte("(72.0, 72.0)")) {
		t.Errorf("the resolution was not carried: %s", o)
	}
}

func TestAPageWithTransparencyIsFlattenedOverWhiteInTheTIFF(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4)) // fully transparent
	var b bytes.Buffer
	png.Encode(&b, img)
	res, err := EncodePageImages(PageImagesTIFF, [][]byte{b.Bytes()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	im, err := tiff.Decode(bytes.NewReader(res.Data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, bl, _ := im.At(1, 1).RGBA()
	if r>>8 != 255 || g>>8 != 255 || bl>>8 != 255 {
		t.Errorf("a transparent pixel came out %d,%d,%d, not white", r>>8, g>>8, bl>>8)
	}
}

func TestGIFAndBMPHoldTheOnePage(t *testing.T) {
	p := pagePNG(t, 2, 64, 48)
	g, err := EncodePageImages(PageImagesGIF, [][]byte{p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gi, err := gif.Decode(bytes.NewReader(g.Data))
	if err != nil || gi.Bounds().Dx() != 64 || gi.Bounds().Dy() != 48 {
		t.Fatalf("gif: %v %v", err, gi)
	}
	bm, err := EncodePageImages(PageImagesBMP, [][]byte{p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := bmp.Decode(bytes.NewReader(bm.Data))
	if err != nil || bi.Bounds().Dx() != 64 {
		t.Fatalf("bmp: %v", err)
	}
	if _, err := EncodePageImages(PageImagesGIF, [][]byte{p, p}, nil); !errors.Is(err, ErrOnePageFormat) {
		t.Errorf("two pages as a GIF: %v", err)
	}
}

func TestTheArchivesNamePagesInOrderAndRefuseNonPictures(t *testing.T) {
	pages := [][]byte{pagePNG(t, 1, 8, 8), pagePNG(t, 2, 8, 8)}
	for _, f := range []PageImageFormat{PageImagesZIP, PageImagesCBZ} {
		res, err := EncodePageImages(f, pages, nil)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := zip.NewReader(bytes.NewReader(res.Data), int64(len(res.Data)))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != 2 || zr.File[0].Name != "page-001.png" || zr.File[1].Name != "page-002.png" {
			t.Errorf("%s names: %v", f, zr.File)
		}
	}
	if _, err := EncodePageImages(PageImagesZIP, [][]byte{[]byte("not a picture")}, nil); err == nil {
		t.Error("a non-PNG went into the archive")
	}
	// A CBZ written here is one the import door reads back as the same number of pages.
	cbz, _ := EncodePageImages(PageImagesCBZ, pages, nil)
	doc, err := CBZToDocument(t.Context(), cbz.Data, "pages.cbz")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(doc); n != 2 {
		t.Errorf("the CBZ read back as %d pages", n)
	}
}

func TestPageImagesRefuseWhatTheyCannotWrite(t *testing.T) {
	if _, err := EncodePageImages("svg", [][]byte{pagePNG(t, 1, 4, 4)}, nil); !errors.Is(err, ErrPageImageFormat) {
		t.Errorf("an unknown format: %v", err)
	}
	if _, err := EncodePageImages(PageImagesTIFF, nil, nil); !errors.Is(err, ErrNoPages) {
		t.Errorf("no pages: %v", err)
	}
	if _, ok := ParsePageImageFormat(""); ok {
		t.Error("the empty string parsed as a format")
	}
	many := make([][]byte, importPages+1)
	if _, err := EncodePageImages(PageImagesZIP, many, nil); !errors.Is(err, ErrTooManyPages) {
		t.Errorf("past the page cap: %v", err)
	}
}

// The fast row readers must say exactly what the generic reader says, for every alpha a PNG can decode to.
func TestTheRowFastPathsAgreeWithTheGenericReader(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(2, 3, 12, 6))
	nrgba := image.NewNRGBA(image.Rect(2, 3, 12, 6))
	for y := 3; y < 6; y++ {
		for x := 2; x < 12; x++ {
			a := uint8((x*37 + y*91) % 256)
			c := color.NRGBA{uint8(x * 20), uint8(y * 40), uint8(x * y), a}
			nrgba.SetNRGBA(x, y, c)
			rgba.Set(x, y, c) // Set premultiplies, so the RGBA image is a valid premultiplied one
		}
	}
	for name, img := range map[string]image.Image{"RGBA": rgba, "NRGBA": nrgba} {
		for y := 3; y < 6; y++ {
			fast := make([]byte, 30)
			fillRow(fast, img, 2, y, 10)
			for x := 0; x < 10; x++ {
				r, g, b, a := img.At(2+x, y).RGBA()
				want := [3]byte{flatten(r, a), flatten(g, a), flatten(b, a)}
				got := [3]byte{fast[3*x], fast[3*x+1], fast[3*x+2]}
				for k := range want {
					if d := int(want[k]) - int(got[k]); d < -1 || d > 1 { // rounding differs by at most one level
						t.Fatalf("%s (%d,%d) channel %d: generic %d, fast %d", name, x, y, k, want[k], got[k])
					}
				}
			}
		}
	}
}

func TestAnUndecodablePagePictureIsNamedAndAResolutionNeverBecomesGarbage(t *testing.T) {
	if _, err := EncodePageImages(PageImagesTIFF, [][]byte{[]byte("nope")}, nil); !errors.Is(err, ErrPageImageUnreadable) {
		t.Errorf("a non-PNG page: %v", err)
	}
	res, err := EncodePageImages(PageImagesTIFF, [][]byte{pagePNG(t, 1, 10, 10)}, []float64{1e-300})
	if err != nil {
		t.Fatal(err)
	}
	im, err := tiff.Decode(bytes.NewReader(res.Data))
	if err != nil || im.Bounds().Dx() != 10 {
		t.Fatalf("a nonsense width broke the file: %v", err)
	}
}

func TestAPageWithTransparencyIsWhiteInTheGIFAndTheBMPToo(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4)) // fully transparent
	var b bytes.Buffer
	png.Encode(&b, img)
	for _, f := range []PageImageFormat{PageImagesGIF, PageImagesBMP} {
		res, err := EncodePageImages(f, [][]byte{b.Bytes()}, nil)
		if err != nil {
			t.Fatal(err)
		}
		im, _, err := image.Decode(bytes.NewReader(res.Data))
		if err != nil {
			t.Fatal(err)
		}
		r, g, bl, _ := im.At(1, 1).RGBA()
		if r>>8 != 255 || g>>8 != 255 || bl>>8 != 255 {
			t.Errorf("%v: a transparent pixel came out %d,%d,%d, not white", f, r>>8, g>>8, bl>>8)
		}
	}
}
