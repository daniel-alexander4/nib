package pdfops

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 400`'s readers — an image is a document.

// solidPNG is a w×h PNG, optionally carrying a `pHYs` chunk declaring dpi.
func solidPNG(t *testing.T, w, h int, dpi float64) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if dpi <= 0 {
		return buf.Bytes()
	}
	// Splice a pHYs chunk in after the signature+IHDR, which is where it belongs and, more to the
	// point, before IDAT — the reader stops at IDAT, so a chunk written after it would be missed
	// and the test would silently grade the default instead of the reading.
	ppm := uint32(math.Round(dpi / 0.0254))
	body := make([]byte, 9)
	binary.BigEndian.PutUint32(body[0:4], ppm)
	binary.BigEndian.PutUint32(body[4:8], ppm)
	body[8] = 1 // metres
	chunk := make([]byte, 0, 12+len(body))
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(body)))
	chunk = append(chunk, l[:]...)
	chunk = append(chunk, []byte("pHYs")...)
	chunk = append(chunk, body...)
	// **A real CRC-32 over type+payload. Go's PNG decoder verifies it** — the first version of this
	// helper wrote four zero bytes with a comment claiming nothing checked, and the decoder answered
	// "invalid checksum" on the very test the chunk existed for.
	crc := crc32.NewIEEE()
	crc.Write(chunk[4:])
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], crc.Sum32())
	chunk = append(chunk, c[:]...)
	raw := buf.Bytes()
	ihdrEnd := 8 + 12 + 13 // signature + (len+type+CRC) + IHDR payload
	out := append([]byte{}, raw[:ihdrEnd]...)
	out = append(out, chunk...)
	return append(out, raw[ihdrEnd:]...)
}

// jpegWithOrientation is a w×h JPEG carrying an EXIF APP1 segment whose Orientation tag is n.
//
// Built by hand because the acceptance names a portrait phone photo with orientation 6, and Go's
// `image/jpeg` writes no EXIF at all — so a fixture produced by encoding alone could never carry
// the tag the rule is about.
func jpegWithOrientation(t *testing.T, w, h, n int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: uint8(y % 256), B: 60, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw := buf.Bytes()
	if n == 0 {
		return raw
	}
	// A minimal little-endian TIFF header with one IFD entry: tag 0x0112, type SHORT, count 1.
	tiff := make([]byte, 0, 26)
	tiff = append(tiff, 'I', 'I', 42, 0)
	tiff = append(tiff, 8, 0, 0, 0) // IFD0 at offset 8
	tiff = append(tiff, 1, 0)       // one entry
	tiff = append(tiff, 0x12, 0x01) // Orientation
	tiff = append(tiff, 3, 0)       // SHORT
	tiff = append(tiff, 1, 0, 0, 0) // count 1
	tiff = append(tiff, byte(n), 0, 0, 0)
	tiff = append(tiff, 0, 0, 0, 0) // next IFD: none

	body := append([]byte("Exif\x00\x00"), tiff...)
	seg := make([]byte, 0, 4+len(body))
	seg = append(seg, 0xFF, 0xE1)
	var l [2]byte
	binary.BigEndian.PutUint16(l[:], uint16(len(body)+2))
	seg = append(seg, l[:]...)
	seg = append(seg, body...)

	out := append([]byte{}, raw[:2]...) // SOI
	out = append(out, seg...)
	return append(out, raw[2:]...)
}

// pageBox reads the one page's MediaBox and its /Rotate.
func pageBox(t *testing.T, pdf []byte) (w, h float64, rotate int) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ctx.PageCount != 1 {
		t.Fatalf("the document has %d pages, want 1", ctx.PageCount)
	}
	d, _, _, err := ctx.PageDict(1, false)
	if err != nil || d == nil {
		t.Fatalf("page dict: %v", err)
	}
	box, err := ctx.PageDims()
	if err != nil || len(box) != 1 {
		t.Fatalf("page dims: %v", err)
	}
	if r := d.IntEntry("Rotate"); r != nil {
		rotate = *r
	}
	return box[0].Width, box[0].Height, rotate
}

// TestAnImageOpensAtItsPhysicalSize — the size rule, across both formats and both dpi cases.
func TestAnImageOpensAtItsPhysicalSize(t *testing.T) {
	cases := []struct {
		name         string
		make         func() []byte
		wantW, wantH float64
		why          string
	}{
		{"PNG with no pHYs falls back to 96 dpi", func() []byte { return solidPNG(t, 192, 96, 0) },
			144, 72, "a screenshot carries no density and is authored at 96"},
		{"PNG honours its pHYs chunk", func() []byte { return solidPNG(t, 300, 150, 300) },
			72, 36, "300px at 300dpi is one inch, which is 72pt"},
		{"JPEG with no JFIF density falls back to 96 dpi", func() []byte { return jpegWithOrientation(t, 192, 96, 0) },
			144, 72, "Go's encoder writes a JFIF header with density 0, which means unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := ImageToDocument(c.make(), "fixture.png")
			if err != nil {
				t.Fatalf("ImageToDocument: %v", err)
			}
			w, h, _ := pageBox(t, out)
			if math.Abs(w-c.wantW) > 1 || math.Abs(h-c.wantH) > 1 {
				t.Errorf("page is %.1f×%.1f pt, want %.0f×%.0f — %s", w, h, c.wantW, c.wantH, c.why)
			}
		})
	}
}

// TestAPhonePhotoOpensUpright — the acceptance clause, and the one pdfcpu cannot do itself.
//
// pdfcpu never reads EXIF (named search over its Go source), so without this the photo opens
// sideways. The rotation is carried as a page `/Rotate` rather than by rewriting pixels, which is
// lossless — so the assertion is on the key, and on the four orientations that are rotations.
func TestAPhonePhotoOpensUpright(t *testing.T) {
	for _, c := range []struct {
		orientation int
		wantRotate  int
		expressible bool
	}{
		{1, 0, true}, {3, 180, true}, {6, 90, true}, {8, 270, true},
		{2, 0, false}, {4, 0, false}, {5, 0, false}, {7, 0, false},
	} {
		src := jpegWithOrientation(t, 120, 240, c.orientation)
		// Stimulus asserted before the response: the fixture must really carry the tag, or this
		// grades the default path under a name that says otherwise.
		if got := jpegOrientation(src); got != c.orientation {
			t.Fatalf("setup: the fixture for orientation %d reads back as %d — the EXIF segment is "+
				"not where the reader looks", c.orientation, got)
		}
		out, err := ImageToDocument(src, "fixture.png")
		if err != nil {
			t.Fatalf("orientation %d: %v", c.orientation, err)
		}
		_, _, rotate := pageBox(t, out)
		if rotate != c.wantRotate {
			verb := "is a rotation and must be applied"
			if !c.expressible {
				verb = "is mirrored, which no rotation expresses, so the page must carry none"
			}
			t.Errorf("EXIF orientation %d produced /Rotate %d, want %d — it %s",
				c.orientation, rotate, c.wantRotate, verb)
		}
	}
}

// TestBytesThatAreNotAnImageAreRefusedByTheirCONTENT — the red proof the acceptance names.
//
// The sniff is on magic bytes and never the extension: a text file renamed `.png` must be refused
// so the caller falls through to the ordinary not-a-PDF sentence, and an image whose extension is
// wrong — ordinary, since a download is named whatever the server said — must still open.
func TestBytesThatAreNotAnImageAreRefusedByTheirCONTENT(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"a text file renamed .png", []byte("this is plainly not a PNG, whatever it is called\n")},
		{"empty", nil},
		{"a PDF", []byte("%PDF-1.7\n")},
		{"PNG magic truncated by one byte", []byte("\x89PNG\r\n\x1a")},
	} {
		if LooksLikeImage(c.data) {
			t.Errorf("%s: LooksLikeImage said yes", c.name)
		}
		if _, err := ImageToDocument(c.data, "fixture.png"); !errors.Is(err, ErrNotAnImage) {
			t.Errorf("%s: err = %v, want ErrNotAnImage — a caller distinguishes 'not an image at "+
				"all' from 'an image I could not read', and only the first falls through to the "+
				"existing refusal", c.name, err)
		}
	}
	// The positive half, without which the negative half is satisfied by a door that refuses
	// everything.
	if !LooksLikeImage(solidPNG(t, 8, 8, 0)) {
		t.Error("a real PNG was not recognised")
	}
	if !LooksLikeImage(jpegWithOrientation(t, 8, 8, 0)) {
		t.Error("a real JPEG was not recognised")
	}
}

// TestAnAbsurdlyLargeImageIsRefusedBeforeItIsDecoded — the byte cap cannot see this.
//
// A PNG of a few hundred KB can decode to gigabytes, so the refusal reads the header and never the
// image. `image.DecodeConfig` is what makes that possible; a test that built a real 200-megapixel
// image to prove it would allocate the thing the cap exists to refuse.
func TestAnAbsurdlyLargeImageIsRefusedBeforeItIsDecoded(t *testing.T) {
	// A valid PNG header declaring a 30000×30000 image (900 MP) with no matching image data. The
	// point is that the refusal arrives from the header alone.
	hacked := append([]byte{}, solidPNG(t, 4, 4, 0)...)
	binary.BigEndian.PutUint32(hacked[16:20], 30000) // IHDR width
	binary.BigEndian.PutUint32(hacked[20:24], 30000) // IHDR height
	// IHDR's own CRC must be recomputed, or the decoder refuses the checksum before it ever reports
	// a size — which would make this test pass for entirely the wrong reason.
	ih := crc32.NewIEEE()
	ih.Write(hacked[12:29]) // type + 13-byte payload
	binary.BigEndian.PutUint32(hacked[29:33], ih.Sum32())
	_, err := ImageToDocument(hacked, "fixture.png")
	if err == nil {
		t.Fatal("a 900-megapixel image was accepted")
	}
	if errors.Is(err, ErrNotAnImage) {
		t.Fatalf("refused as 'not an image' rather than as too large: %v", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("megapixel")) {
		t.Errorf("err = %v, want the refusal to name the size", err)
	}
}

// TestAnOpenedImageIsAPageOfInk — the conversion produced a real page, not an empty one.
func TestAnOpenedImageIsAPageOfInk(t *testing.T) {
	out, err := ImageToDocument(solidPNG(t, 64, 32, 0), "fixture.png")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, _, _, err := ctx.PageDict(1, false)
	if err != nil || d == nil {
		t.Fatal(err)
	}
	res, err := ctx.DereferenceDict(d["Resources"])
	if err != nil || res == nil {
		t.Fatal("the page has no /Resources, so it draws nothing")
	}
	xo, err := ctx.DereferenceDict(res["XObject"])
	if err != nil || len(xo) == 0 {
		t.Fatal("the page references no XObject — the image did not land on it")
	}
	found := false
	for _, v := range xo {
		sd, _, serr := ctx.DereferenceStreamDict(v)
		if serr != nil || sd == nil {
			continue
		}
		if st := sd.Dict.NameEntry("Subtype"); st != nil && *st == "Image" {
			found = true
			if w := sd.Dict.IntEntry("Width"); w == nil || *w != 64 {
				t.Errorf("the embedded image is %v px wide, want 64", w)
			}
		}
	}
	if !found {
		t.Error("the page's XObjects hold no /Image")
	}
	_ = types.Name("")
}

// TestAnOpenedImageIsTitledFromItsFilename — a document nib authors owes a title.
//
// `TestEveryAuthoredDocumentGetsATitle` enumerates every door that produces a PDF and refuses one
// that reaches neither `TitleFromName` nor `SetTitle`; it caught this door the moment it existed.
// The guard checks the ROUTE, so this checks the RESULT — a reader's title bar should say
// "receipt", not nothing.
func TestAnOpenedImageIsTitledFromItsFilename(t *testing.T) {
	out, err := ImageToDocument(solidPNG(t, 32, 16, 0), "receipt.png")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	md, _, err := ctx.DereferenceStreamDict(cat["Metadata"])
	if err != nil || md == nil {
		t.Fatal("the opened image carries no /Metadata, so it has no title a reader can show")
	}
	if err := md.Decode(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(md.Content, []byte("receipt")) {
		t.Errorf("the document's metadata does not mention %q — an image opened by name should not "+
			"be untitled", "receipt")
	}

	// An empty name must leave the document intact rather than cost the caller it.
	bare, err := ImageToDocument(solidPNG(t, 32, 16, 0), "")
	if err != nil {
		t.Fatalf("an unnamed image was refused: %v", err)
	}
	if !bytes.HasPrefix(bare, []byte("%PDF-")) {
		t.Error("an unnamed image did not produce a document")
	}
}
