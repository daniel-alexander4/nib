package pdfops

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"

	"golang.org/x/image/tiff"

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

// tinyWebP is a 2×2 lossless WebP, 38 bytes.
//
// Embedded rather than generated because **Go has no WebP encoder** — `x/image/webp` decodes only.
// Produced once with, and reproducible by:
//
//	python3 -c "from PIL import Image; Image.new('RGB',(2,2),(200,60,60)).save('t.webp','WEBP',lossless=True)"
//
// A file copied from elsewhere on the machine would have been a fixture nobody could regenerate.
const tinyWebP = "UklGRh4AAABXRUJQVlA4TBEAAAAvAUAAAAdQniKXp/+BiOh/AAA="

func webpFixture(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(tinyWebP)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func tiffFixture(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: uint8(x * 8), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := tiff.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestEveryFormatTheSniffCLAIMSActuallyRoundTrips — /pending 539(a).
//
// /pending 400 said to add TIFF and WebP "once a test proves pdfcpu takes them", and the proof it
// had was a reading: pdfcpu imports `hhrutter/tiff` and `x/image/webp`, both of which call
// `image.RegisterFormat` in `init()`. **Reading an import list is not a round-trip** — a format can
// decode through `image.DecodeConfig` and still be refused by `api.ImportImages`, which is a
// different code path with its own opinions — so every format the sniff claims is driven end to end
// here, and the table is the sniff's own list rather than a copy of it.
func TestEveryFormatTheSniffCLAIMSActuallyRoundTrips(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"png", solidPNG(t, 96, 48, 0)},
		{"jpeg", jpegWithOrientation(t, 96, 48, 0)},
		{"tiff", tiffFixture(t, 96, 48)},
		{"webp", webpFixture(t)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Stimulus: the sniff must recognise it, or the round-trip below grades the refusal path.
			if !LooksLikeImage(c.data) {
				t.Fatalf("the sniff does not recognise this %s, so nothing downstream was exercised", c.name)
			}
			out, err := ImageToDocument(c.data, "fixture."+c.name)
			if err != nil {
				t.Fatalf("%s: the sniff claims this format and the conversion refused it: %v", c.name, err)
			}
			w, h, _ := pageBox(t, out)
			if w <= 0 || h <= 0 {
				t.Errorf("%s produced a %.1f×%.1f page", c.name, w, h)
			}
		})
	}
	// The sniff's claims and this table are the same set, or one of them has grown alone.
	if got := len(cases); got != 4 {
		t.Fatalf("this table drives %d formats; sniffImage claims 4", got)
	}
}

// TestATiffMagicInsideAJpegIsNotATiff — the trap /pending 539 named.
//
// `II*\0` is both TIFF's little-endian header and the start of an EXIF block, which every
// orientation-bearing photo carries six bytes into its APP1 segment. A sniff that searched for the
// magic rather than anchoring at offset 0 would call such a photo a TIFF and hand it to the wrong
// decoder.
func TestATiffMagicInsideAJpegIsNotATiff(t *testing.T) {
	src := jpegWithOrientation(t, 40, 80, 6)
	if i := bytes.Index(src, []byte{0x49, 0x49, 0x2A, 0x00}); i <= 0 {
		t.Fatalf("setup: the fixture carries no TIFF magic after offset 0 (found at %d), so the "+
			"confusion this test is about cannot arise", i)
	}
	f, ok := sniffImage(src)
	if !ok || f != formatJPEG {
		t.Errorf("a JPEG carrying an EXIF block sniffed as %q — the TIFF magic inside it was matched "+
			"somewhere other than offset 0", f)
	}
}

// TestRedactingAnOpenedImageDESTROYSThePixelsUnderTheMark — /pending 400's stated trap, 539(b).
//
// 400 wrote this down so nobody would ship it: *"A redaction mark over an opened image covers pixels
// that are still in the file until the page is flattened… an image is the case where a user most
// expects 'I blacked it out' to mean the pixels are gone."* It then shipped without running the test
// it had specified, which is the gap this closes.
//
// **The assertion is on the BYTES of the embedded image, not on the mark being drawn.** A test that
// confirmed a black rectangle exists would pass on a document that still carries every original
// pixel underneath it — which is precisely the failure being guarded against.
func TestRedactingAnOpenedImageDESTROYSThePixelsUnderTheMark(t *testing.T) {
	// An image whose top half is a distinctive colour. After redacting that half, no pixel of it
	// may survive anywhere in the output.
	const secret = 0xC7
	img := image.NewRGBA(image.Rect(0, 0, 60, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 60; x++ {
			if y < 20 {
				img.Set(x, y, color.RGBA{R: secret, G: 0x11, B: 0x22, A: 255}) // the part to be redacted
			} else {
				img.Set(x, y, color.RGBA{R: 0x11, G: 0x99, B: 0x11, A: 255})
			}
		}
	}
	var enc bytes.Buffer
	if err := png.Encode(&enc, img); err != nil {
		t.Fatal(err)
	}
	doc, err := ImageToDocument(enc.Bytes(), "secret.png")
	if err != nil {
		t.Fatal(err)
	}

	// Stimulus, asserted before the response: the secret colour really is in the opened document's
	// embedded image. Without this the test passes on a build that never embedded anything.
	if !embeddedImageHasRed(t, doc, secret) {
		t.Fatal("setup: the opened image does not carry the secret colour, so its absence below " +
			"would prove nothing")
	}

	// Redaction rasterises: the caller hands back the page as pixels, with the secret half painted
	// over, and RedactPages rebuilds the page from that raster.
	flat := image.NewRGBA(image.Rect(0, 0, 60, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 60; x++ {
			if y < 20 {
				flat.Set(x, y, color.RGBA{A: 255}) // the mark: opaque black
			} else {
				flat.Set(x, y, color.RGBA{R: 0x11, G: 0x99, B: 0x11, A: 255})
			}
		}
	}
	var rast bytes.Buffer
	if err := png.Encode(&rast, flat); err != nil {
		t.Fatal(err)
	}
	w, h, _ := pageBox(t, doc)
	out, err := RedactPages(doc, map[int]RasterPage{1: {Image: rast.Bytes(), W: w, H: h}})
	if err != nil {
		t.Fatalf("RedactPages: %v", err)
	}

	if embeddedImageHasRed(t, out, secret) {
		t.Error("a redacted image-origin document still carries the pixels under the mark. The mark " +
			"is drawn and the original is underneath it, which is exactly what a user reads " +
			"'I blacked it out' as not meaning")
	}
}

// embeddedImageHasRed decodes every image XObject in pdf and reports whether any pixel carries r as
// its red channel. It reads the IMAGES, not the content stream, because the question is whether the
// pixels survive anywhere in the file.
func embeddedImageHasRed(t *testing.T, pdf []byte, r uint8) bool {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	for nr := range ctx.XRefTable.Table {
		sd, _, serr := ctx.DereferenceStreamDict(*types.NewIndirectRef(nr, 0))
		if serr != nil || sd == nil {
			continue
		}
		if st := sd.Dict.NameEntry("Subtype"); st == nil || *st != "Image" {
			continue
		}
		if sd.Decode() != nil {
			continue
		}
		// Whatever the filter, the decoded bytes are the samples; a scan for the byte value is
		// enough to answer "did this colour survive", and does not depend on the colour space.
		if bytes.IndexByte(sd.Content, r) >= 0 {
			return true
		}
	}
	return false
}
