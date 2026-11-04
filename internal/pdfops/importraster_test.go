package pdfops

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"golang.org/x/image/bmp"
)

// pageWidths is each page's width in points, rounded: the pages of an import told apart by the width of the
// picture each came from (96 dpi, so a w-pixel picture is w*0.75 points).
func pageWidths(t *testing.T, pdf []byte) []int {
	t.Helper()
	dims, err := api.PageDims(context.Background(), bytes.NewReader(pdf), nil)
	if err != nil {
		t.Fatalf("page dimensions: %v", err)
	}
	out := make([]int, len(dims))
	for i, d := range dims {
		out[i] = int(d.Width + 0.5)
	}
	return out
}

func eq(a, b []int) bool { return fmt.Sprint(a) == fmt.Sprint(b) }

func gifOf(t *testing.T, widths ...int) []byte {
	t.Helper()
	g := &gif.GIF{}
	for _, w := range widths {
		p := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.White, color.Black})
		_ = w
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, 0)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func bmpOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := bmp.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// tiffOf hand-builds an uncompressed 8-bit grey TIFF with one page per width (height 2), little- or big-endian.
// comp overrides the Compression tag when non-zero; link=true makes the last page's next-offset point back to
// the first (a loop).
func tiffOf(widths []int, comp uint16, big, link bool) []byte {
	var bo binary.ByteOrder = binary.LittleEndian
	hdr := []byte{'I', 'I', 42, 0}
	if big {
		bo = binary.BigEndian
		hdr = []byte{'M', 'M', 0, 42}
	}
	buf := append([]byte{}, hdr...)
	buf = append(buf, 0, 0, 0, 0)
	put32 := func(at int, v uint32) { bo.PutUint32(buf[at:], v) }
	prevNext := 4
	first := 0
	const nEnt = 9
	for i, w := range widths {
		off := len(buf)
		if i == 0 {
			first = off
		}
		put32(prevNext, uint32(off))
		pix := bytes.Repeat([]byte{byte(40 + i)}, w*2)
		dataOff := off + 2 + nEnt*12 + 4
		ent := func(tag, typ uint16, v uint32) []byte {
			e := make([]byte, 12)
			bo.PutUint16(e[0:], tag)
			bo.PutUint16(e[2:], typ)
			bo.PutUint32(e[4:], 1)
			if typ == 3 {
				bo.PutUint16(e[8:], uint16(v))
			} else {
				bo.PutUint32(e[8:], v)
			}
			return e
		}
		c := uint32(1)
		if comp != 0 {
			c = uint32(comp)
		}
		b2 := make([]byte, 2)
		bo.PutUint16(b2, nEnt)
		buf = append(buf, b2...)
		for _, e := range [][]byte{
			ent(256, 4, uint32(w)), ent(257, 4, 2), ent(258, 3, 8), ent(259, 3, c), ent(262, 3, 1),
			ent(273, 4, uint32(dataOff)), ent(277, 3, 1), ent(278, 4, 2), ent(279, 4, uint32(len(pix))),
		} {
			buf = append(buf, e...)
		}
		prevNext = len(buf)
		buf = append(buf, 0, 0, 0, 0)
		buf = append(buf, pix...)
	}
	if link {
		put32(prevNext, uint32(first))
	}
	return buf
}

func TestAGIFBecomesAPagePerFrame(t *testing.T) {
	res, err := importBytes(t, gifOf(t, 4, 4, 4), "anim.gif", ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := pageWidths(t, res.PDF); !eq(got, []int{3, 3, 3}) {
		t.Fatalf("pages = %v, want three 4 px pages (3 pt at 96 dpi)", got)
	}
	if res.Name != "anim.pdf" || !res.Converted {
		t.Fatalf("name=%q converted=%v", res.Name, res.Converted)
	}
}

func TestAGIFFrameCapRefusesAtCapPlusOneAndAdmitsAtCap(t *testing.T) {
	w := make([]int, importPages)
	if res, err := importBytes(t, gifOf(t, w...), "a.gif", ImportOptions{}); err != nil {
		t.Fatalf("at the cap: %v", err)
	} else if n, _ := PageCount(res.PDF); n != importPages {
		t.Fatalf("at the cap: %d pages, want %d (the last frame must be present)", n, importPages)
	}
	_, err := importBytes(t, gifOf(t, make([]int, importPages+1)...), "a.gif", ImportOptions{})
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("cap+1: %v, want ErrTooManyPages", err)
	}
}

func TestABMPBecomesAPageAtItsDeclaredDensity(t *testing.T) {
	res, err := importBytes(t, bmpOf(t, 96, 48), "pic.bmp", ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := pageWidths(t, res.PDF); !eq(got, []int{72}) {
		t.Fatalf("pages = %v, want one 72 pt page (96 px at 96 dpi)", got)
	}
}

func TestATextFileThatBeginsBMIsNotAPicture(t *testing.T) {
	for _, b := range [][]byte{[]byte("BM is a company, not a bitmap, and this line is long enough"), []byte("GIF is how it is said")} {
		if LooksLikeImage(b) {
			t.Errorf("%q sniffed as an image", b[:12])
		}
	}
}

func TestAMultiPageTIFFReadsBackEveryPage(t *testing.T) {
	for _, big := range []bool{false, true} {
		res, err := importBytes(t, tiffOf([]int{40, 80, 120}, 0, big, false), "scan.tif", ImportOptions{})
		if err != nil {
			t.Fatalf("big=%v: %v", big, err)
		}
		if got := pageWidths(t, res.PDF); !eq(got, []int{30, 60, 90}) {
			t.Fatalf("big=%v: pages %v, want 30/60/90 — pages that differ must each read back", big, got)
		}
	}
}

func TestATIFFDirectoryThatLoopsIsRefusedNotOpenedShort(t *testing.T) {
	_, err := importBytes(t, tiffOf([]int{40, 80}, 0, false, true), "loop.tif", ImportOptions{})
	if !errors.Is(err, errTIFFLoop) || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("a looping chain: %v, want errTIFFLoop (a file opened as 2 of its pages says nothing of the rest)", err)
	}
}

func TestATIFFPageCapRefusesAtCapPlusOneAndAdmitsAtCap(t *testing.T) {
	w := make([]int, importPages)
	for i := range w {
		w[i] = 2
	}
	if res, err := importBytes(t, tiffOf(w, 0, false, false), "a.tif", ImportOptions{}); err != nil {
		t.Fatalf("at the cap: %v", err)
	} else if n, _ := PageCount(res.PDF); n != importPages {
		t.Fatalf("at the cap: %d pages", n)
	}
	_, err := importBytes(t, tiffOf(append(w, 2), 0, false, false), "a.tif", ImportOptions{})
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("cap+1: %v", err)
	}
}

func TestATIFFCompressionNixHasNoDecoderForIsRefusedByName(t *testing.T) {
	_, err := importBytes(t, tiffOf([]int{8, 8}, 7, false, false), "jpeg.tif", ImportOptions{})
	if !errors.Is(err, ErrTIFFCompression) {
		t.Fatalf("JPEG-in-TIFF: %v", err)
	}
}

func TestATIFFWhoseDirectoryPointsPastTheFileIsRefusedNotRead(t *testing.T) {
	b := tiffOf([]int{8, 8}, 0, false, false)
	// Point the first directory's successor far past the end.
	first := int(binary.LittleEndian.Uint32(b[4:8]))
	next := first + 2 + 9*12
	binary.LittleEndian.PutUint32(b[next:], 0x7fffff00)
	_, err := importBytes(t, b, "bad.tif", ImportOptions{})
	if err == nil {
		t.Fatal("a directory past the end of the file opened")
	}
}

// ---- the pixel caps ------------------------------------------------------------------------------------------

func pngHeaderOnly(w, h int) []byte {
	var ihdr bytes.Buffer
	binary.Write(&ihdr, binary.BigEndian, uint32(w))
	binary.Write(&ihdr, binary.BigEndian, uint32(h))
	ihdr.Write([]byte{8, 6, 0, 0, 0})
	chunk := func(typ string, body []byte) []byte {
		var c bytes.Buffer
		binary.Write(&c, binary.BigEndian, uint32(len(body)))
		c.WriteString(typ)
		c.Write(body)
		binary.Write(&c, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), body...)))
		return c.Bytes()
	}
	return append(append([]byte("\x89PNG\r\n\x1a\n"), chunk("IHDR", ihdr.Bytes())...), chunk("IEND", nil)...)
}

func TestThePixelCapRefusesOnTheHeaderAloneAndNamesItself(t *testing.T) {
	// 200 Mpx + 1: refused from the header, with the decode never begun.
	_, err := importBytes(t, pngHeaderOnly(maxImagePixels/2+1, 2), "big.png", ImportOptions{})
	if !errors.Is(err, ErrImagePixels) {
		t.Fatalf("cap+1: %v, want ErrImagePixels", err)
	}
	j, cancel := newRasterJob(t.Context())
	defer cancel()
	if err := j.charge(maxImagePixels/2, 2); err != nil {
		t.Fatalf("at the cap: %v", err)
	}
}

func TestTheImportTotalPixelCapIsARunningSum(t *testing.T) {
	j, cancel := newRasterJob(t.Context())
	defer cancel()
	per := maxImagePixels / 2
	n := 0
	for ; ; n++ {
		if err := j.charge(per, 2); err != nil {
			if !errors.Is(err, ErrImagePixels) {
				t.Fatal(err)
			}
			break
		}
		if n > 100 {
			t.Fatal("the running sum never refused")
		}
	}
	if int64(n)*int64(maxImagePixels) > importTotalPixels || int64(n+1)*int64(maxImagePixels) <= importTotalPixels {
		t.Fatalf("refused after %d pictures of %d px; the cap is %d", n, maxImagePixels, importTotalPixels)
	}
}

// ---- CBZ ----------------------------------------------------------------------------------------------------

// zipOf writes entries with archive/zip directly, which ziputil.Write would refuse for an unsafe name.
func zipNamed(t *testing.T, names []string, bodies [][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for i, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(bodies[i])
	}
	zw.Close()
	return b.Bytes()
}

func TestACBZOpensInNaturalNameOrderAndIgnoresWhatIsNotAPicture(t *testing.T) {
	cbz := zipNamed(t,
		[]string{"page10.png", "page2.png", "ComicInfo.xml", "page1.png", "__MACOSX/page1.png", ".hidden.png", "thumbs/"},
		[][]byte{pngBytes(t, 40, 8), pngBytes(t, 20, 8), []byte("<ComicInfo/>"), pngBytes(t, 10, 8), pngBytes(t, 99, 8), pngBytes(t, 98, 8), nil})
	res, err := importBytes(t, cbz, "book.cbz", ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := pageWidths(t, res.PDF); !eq(got, []int{8, 15, 30}) {
		t.Fatalf("pages %v, want page1, page2, page10 (8, 15, 30 pt)", got)
	}
	if res.Name != "book.pdf" {
		t.Fatalf("name %q", res.Name)
	}
}

func TestACBZIsKnownByItsBytesNotItsNames(t *testing.T) {
	cbz := zipNamed(t, []string{"a.txt", "b.jpg"}, [][]byte{pngBytes(t, 10, 8), []byte("not an image at all")})
	res, err := importBytes(t, cbz, "x.cbz", ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(res.PDF); n != 1 {
		t.Fatalf("pages %d: a PNG named .txt is a page, a text file named .jpg is not", n)
	}
}

func TestACBZWithNoPicturesIsRefusedByName(t *testing.T) {
	_, err := importBytes(t, zipNamed(t, []string{"a.txt"}, [][]byte{[]byte("hi")}), "x.cbz", ImportOptions{})
	if !errors.Is(err, ErrNoPages) {
		t.Fatalf("%v", err)
	}
}

func TestACBZWithAnUnsafeOrRepeatedNameIsRefusedWhole(t *testing.T) {
	p := pngBytes(t, 4, 4)
	for _, names := range [][]string{{"../evil.png", "ok.png"}, {"/abs.png", "ok.png"}, {"a\\b.png", "ok.png"}, {"p.png", "p.png"}} {
		_, err := importBytes(t, zipNamed(t, names, [][]byte{p, p}), "x.cbz", ImportOptions{})
		if !errors.Is(err, ErrArchiveRefused) {
			t.Errorf("%q: %v, want ErrArchiveRefused", names, err)
		}
	}
}

func TestACBZEntryCapRefusesAtCapPlusOneAndAdmitsAtCap(t *testing.T) {
	mk := func(n int) []byte {
		names := make([]string, n)
		bodies := make([][]byte, n)
		for i := range names {
			names[i] = fmt.Sprintf("f%04d.txt", i)
			bodies[i] = []byte("x")
		}
		names[0], bodies[0] = "page.png", pngBytes(t, 4, 4)
		return zipNamed(t, names, bodies)
	}
	if _, err := importBytes(t, mk(importArchiveEntries), "x.cbz", ImportOptions{}); err != nil {
		t.Fatalf("at the cap: %v", err)
	}
	if _, err := importBytes(t, mk(importArchiveEntries+1), "x.cbz", ImportOptions{}); !errors.Is(err, ErrArchiveRefused) {
		t.Fatalf("cap+1: %v", err)
	}
}

func TestACBZPageCapRefusesAtCapPlusOneAndAdmitsAtCap(t *testing.T) {
	mk := func(n int) []byte {
		p := pngBytes(t, 2, 2)
		names := make([]string, n)
		bodies := make([][]byte, n)
		for i := range names {
			names[i] = fmt.Sprintf("p%04d.png", i)
			bodies[i] = p
		}
		return zipNamed(t, names, bodies)
	}
	res, err := importBytes(t, mk(importPages), "x.cbz", ImportOptions{})
	if err != nil {
		t.Fatalf("at the cap: %v", err)
	}
	if n, _ := PageCount(res.PDF); n != importPages {
		t.Fatalf("at the cap: %d pages, want %d", n, importPages)
	}
	if _, err := importBytes(t, mk(importPages+1), "x.cbz", ImportOptions{}); !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("cap+1: %v", err)
	}
}

func TestACBZPictureCapIsTheNamedConstant(t *testing.T) {
	// Stored, so the ratio is 1 and only the picture cap can refuse.
	mk := func(n int) []byte {
		body := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, n-8)...)
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: "big.png", Method: zip.Store})
		w.Write(body)
		zw.Close()
		return b.Bytes()
	}
	// At the cap the archive is read and the "picture" is then refused as unreadable, not as too large.
	_, err := importBytes(t, mk(importCBZEntryBytes), "x.cbz", ImportOptions{})
	if err != nil && strings.Contains(err.Error(), "holds more than") {
		t.Fatalf("at the cap: %v", err)
	}
	_, err = importBytes(t, mk(importCBZEntryBytes+1), "x.cbz", ImportOptions{})
	if !errors.Is(err, ErrArchiveRefused) || !strings.Contains(err.Error(), "holds more than") {
		t.Fatalf("one byte past the cap: %v", err)
	}
}

func TestThePNGEncoderRoundTripsATransparentFrame(t *testing.T) {
	// The GIF path hands a transparent canvas to the PNG encoder; a PNG with alpha must reach a page.
	im := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	j, cancel := newRasterJob(t.Context())
	defer cancel()
	if err := j.emitImage(im, 0); err != nil {
		t.Fatal(err)
	}
}

// A TIFF whose first directory is a reduced-resolution thumbnail and whose second is the page: the walk drops the
// thumbnail, and the decoder — which reads the FIRST directory — would have opened it instead of the page.
func TestATIFFWithAThumbnailFirstOpensThePageNotTheThumbnail(t *testing.T) {
	b := tiffOf([]int{8, 40}, 0, false, false)
	// The first directory's fifth entry (PhotometricInterpretation, tag 262) becomes NewSubfileType = reduced (tag 254),
	// moved to the front so the tags stay in ascending order and the decoder reads the thumbnail without complaint.
	d := 8 + 2
	ent := append([]byte{}, b[d+4*12:d+5*12]...)
	binary.LittleEndian.PutUint16(ent[0:], 254)
	binary.LittleEndian.PutUint16(ent[2:], 4)
	binary.LittleEndian.PutUint32(ent[4:], 1)
	binary.LittleEndian.PutUint32(ent[8:], 1)
	copy(b[d+12:d+5*12], b[d:d+4*12])
	copy(b[d:], ent)
	res, err := importBytes(t, b, "thumb.tif", ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := pageWidths(t, res.PDF); !eq(got, []int{30}) {
		t.Fatalf("pages = %v, want the one 40 px page (30 pt), not the 8 px thumbnail (6 pt)", got)
	}
}

// TestAGIFsFramesAndScreensAreOneBudget — the frames' own pixels and the screen-sized pages they become are
// charged to the SAME counter, so importTotalPixels is the sum and not two caps of the same size.
func TestAGIFsFramesAndScreensAreOneBudget(t *testing.T) {
	g := gifOf(t, 4) // one 4x4 frame: 16 px of page, 16 px of frame
	j, cancel := newRasterJob(t.Context())
	defer cancel()
	j.pixels = importTotalPixels - 20 // room for the page (16) but not the page and the frame (32)
	if err := j.addGIF(g); !errors.Is(err, ErrImagePixels) {
		t.Fatalf("page and frame together past the one cap: %v, want ErrImagePixels", err)
	}
	j2, cancel2 := newRasterJob(t.Context())
	defer cancel2()
	j2.pixels = importTotalPixels - 32
	if err := j2.addGIF(g); err != nil {
		t.Fatalf("exactly at the cap: %v", err)
	}
}

// TestADensityNobodyMeantIsTheDefaultDensity — a declared density outside minImportDPI..maxImportDPI is read as
// unknown, so a 96 px picture is 72 pt (the 96 dpi default) rather than a 7 km or a 0.07 pt page.
func TestADensityNobodyMeantIsTheDefaultDensity(t *testing.T) {
	withPPM := func(ppm uint32) []byte {
		b := bmpOf(t, 96, 48)
		binary.LittleEndian.PutUint32(b[38:42], ppm)
		return b
	}
	for name, c := range map[string]struct {
		ppm  uint32
		want int
	}{
		"in range (192 dpi)":     {7559, 36},
		"1 dpi":                  {39, 72},
		"100000 dpi":             {3937008, 72},
		"just under the minimum": {391, 72},
		"just over the maximum":  {94495, 72},
	} {
		res, err := importBytes(t, withPPM(c.ppm), "d.bmp", ImportOptions{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := pageWidths(t, res.PDF); !eq(got, []int{c.want}) {
			t.Errorf("%s: width %v, want %d", name, got, c.want)
		}
	}
}

func TestACBZMemberThatIsNotAPictureNeverRefusesTheArchiveForItsSize(t *testing.T) {
	// A stored text member one byte over the picture cap; the picture beside it is the only page.
	big := bytes.Repeat([]byte("x"), importCBZEntryBytes+1)
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "ComicInfo.xml", Method: zip.Store})
	w.Write(big)
	w, _ = zw.Create("p1.png")
	w.Write(pngBytes(t, 8, 8))
	zw.Close()
	res, err := importBytes(t, b.Bytes(), "x.cbz", ImportOptions{})
	if err != nil {
		t.Fatalf("an over-cap non-picture refused the archive: %v", err)
	}
	if n, _ := PageCount(res.PDF); n != 1 {
		t.Fatalf("%d pages, want 1", n)
	}
}

func TestTheLabelNoteIsTheOneSentenceForARefusedLabel(t *testing.T) {
	if got := (&ImportResult{}).LabelNote(); got != "" {
		t.Fatalf("no refusal: %q", got)
	}
	var nilRes *ImportResult
	if got := nilRes.LabelNote(); got != "" {
		t.Fatalf("nil result: %q", got)
	}
	got := (&ImportResult{LabelRefused: ErrUAUntagged}).LabelNote()
	if !strings.HasPrefix(got, "Not labelled PDF/UA: ") || !strings.Contains(got, ErrUAUntagged.Error()) {
		t.Fatalf("refused: %q", got)
	}
}
