package pdfops

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 908` — the pictures in a document are shrunk and nothing else changes. Documents are hand-built so each
// picture's dictionary is exactly what a case needs; the output is read back through the same door the pass uses.

type pic struct {
	w, h  int
	extra string // dictionary entries beside the size
	data  []byte
}

// photo is a deterministic, noisy gradient: more than 256 colours, and large once encoded.
func photo(w, h int) *image.RGBA {
	r := rand.New(rand.NewSource(7))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := func() int { return r.Intn(40) - 20 }
			img.Set(x, y, color.RGBA{
				uint8(clamp8(x*255/w + n())), uint8(clamp8(y*255/h + n())), uint8(clamp8(128 + (x+y)*60/(w+h) + n())), 255,
			})
		}
	}
	return img
}

// zeros is a zlib stream of n zero bytes.
func zeros(n int) []byte {
	var b bytes.Buffer
	zw := zlib.NewWriter(&b)
	zw.Write(make([]byte, n))
	zw.Close()
	return b.Bytes()
}

func clamp8(v int) int { return min(255, max(0, v)) }

func jpegOf(t *testing.T, img image.Image, q int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: q}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func flateRGB(t *testing.T, img *image.RGBA) []byte {
	t.Helper()
	raw := make([]byte, 0, img.Bounds().Dx()*img.Bounds().Dy()*3)
	for i := 0; i < len(img.Pix); i += 4 {
		raw = append(raw, img.Pix[i], img.Pix[i+1], img.Pix[i+2])
	}
	var b bytes.Buffer
	zw := zlib.NewWriter(&b)
	zw.Write(raw)
	zw.Close()
	return b.Bytes()
}

// docWithPictures draws every picture on one page under a line of text.
func docWithPictures(pics ...pic) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var xo, content strings.Builder
	content.WriteString("BT /F1 18 Tf 40 760 Td (the words stay) Tj ET\n")
	for i, p := range pics {
		nr := 10 + i
		fmt.Fprintf(&xo, "/Im%d %d 0 R ", i, nr)
		fmt.Fprintf(&content, "q 200 0 0 150 %d 500 cm /Im%d Do Q\n", 20+i*5, i)
		objs[nr] = fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d %s /Length %d >>\nstream\n%s\nendstream",
			p.w, p.h, p.extra, len(p.data), p.data)
	}
	objs[3] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> /XObject << %s>> >> >>", xo.String())
	objs[4] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", content.Len(), content.String())
	return testpdf.Assemble(objs)
}

type picInfo struct {
	w, h    int
	filter  string
	rawLen  int
	space   string
	meanRGB [3]float64
}

// picturesIn reads every image stream of pdf back.
func picturesIn(t *testing.T, pdf []byte) []picInfo {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	var nrs []int
	for nr, e := range ctx.Table {
		if e == nil {
			continue
		}
		if sd, ok := e.Object.(types.StreamDict); ok {
			if st := sd.Dict.NameEntry("Subtype"); st != nil && *st == "Image" {
				nrs = append(nrs, nr)
			}
		}
	}
	sort.Ints(nrs)
	var out []picInfo
	for _, nr := range nrs {
		sd := ctx.Table[nr].Object.(types.StreamDict)
		in := picInfo{w: intEntry(sd.Dict, "Width"), h: intEntry(sd.Dict, "Height"), rawLen: len(sd.Raw), space: fmt.Sprint(sd.Dict["ColorSpace"])}
		if len(sd.FilterPipeline) == 1 {
			in.filter = sd.FilterPipeline[0].Name
		}
		switch in.filter {
		case "DCTDecode":
			if img, err := jpeg.Decode(bytes.NewReader(sd.Raw)); err == nil {
				in.meanRGB = meanOf(img)
			}
		case "FlateDecode":
			if err := sd.Decode(); err == nil && len(sd.Content) == in.w*in.h*3 {
				in.meanRGB = meanOf(samplesToImage(sd.Content, in.w, in.h, 3))
			}
		}
		out = append(out, in)
	}
	return out
}

func meanOf(img image.Image) [3]float64 {
	var sum [3]float64
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			r, g, bl, _ := img.At(x, y).RGBA()
			sum[0] += float64(r >> 8)
			sum[1] += float64(g >> 8)
			sum[2] += float64(bl >> 8)
			n++
		}
	}
	for i := range sum {
		sum[i] /= float64(n)
	}
	return sum
}

const rgb = "/ColorSpace /DeviceRGB /BitsPerComponent 8"

func TestShrinkPicturesMakesAJPEGSmallerAndLeavesTheRest(t *testing.T) {
	img := photo(2400, 1800)
	doc := docWithPictures(pic{2400, 1800, rgb + " /Filter /DCTDecode", jpegOf(t, img, 95)})
	before := picturesIn(t, doc)
	if len(before) != 1 || before[0].filter != "DCTDecode" || before[0].w != 2400 {
		t.Fatalf("setup: %+v", before)
	}

	out, rep, err := ShrinkPictures(doc, 3) // 1400 px, quality 70
	if err != nil {
		t.Fatal(err)
	}
	after := picturesIn(t, out)
	if len(after) != 1 {
		t.Fatalf("the picture count changed: %+v", after)
	}
	a := after[0]
	if rep.Shrunk != 1 || a.filter != "DCTDecode" {
		t.Fatalf("report %+v, picture %+v; want one picture shrunk and still a JPEG", rep, a)
	}
	if max(a.w, a.h) != 1400 || a.w*3 != a.h*4 {
		t.Errorf("picture is %d×%d, want the longest side at 1400 and the 4:3 shape kept", a.w, a.h)
	}
	if a.rawLen*3 > before[0].rawLen {
		t.Errorf("picture is %d bytes of %d: not a third or less", a.rawLen, before[0].rawLen)
	}
	if len(out)*3 > len(doc) {
		t.Errorf("document is %d bytes of %d: the saving did not reach the file", len(out), len(doc))
	}
	// Colours: the entry that names them is untouched, and the average colour moved by a few levels at most.
	if a.space != before[0].space {
		t.Errorf("colour space %q became %q", before[0].space, a.space)
	}
	for c := 0; c < 3; c++ {
		if d := math.Abs(a.meanRGB[c] - before[0].meanRGB[c]); d > 4 {
			t.Errorf("channel %d mean moved by %.1f levels", c, d)
		}
	}
	// Text is not a picture.
	if !strings.Contains(pageText(t, out, 1), "the words stay") {
		t.Error("the page's text did not survive")
	}
}

func TestShrinkPicturesReencodesAFlatePhotographAndKeepsAFlatColourGraphicExact(t *testing.T) {
	img := photo(1600, 1200)
	var flat bytes.Buffer
	zw := zlib.NewWriter(&flat)
	zw.Write(bytes.Repeat([]byte{200, 30, 30, 20, 20, 200}, 1000*800/2)) // two colours
	zw.Close()
	doc := docWithPictures(
		pic{1600, 1200, rgb + " /Filter /FlateDecode", flateRGB(t, img)},
		pic{1000, 800, rgb + " /Filter /FlateDecode", flat.Bytes()},
	)
	out, rep, err := ShrinkPictures(doc, 2)
	if err != nil {
		t.Fatal(err)
	}
	after := picturesIn(t, out)
	if len(after) != 2 {
		t.Fatalf("pictures: %+v", after)
	}
	var photoOut, graphicOut picInfo
	for _, p := range after {
		if p.w == 1000 {
			graphicOut = p
		} else {
			photoOut = p
		}
	}
	// Rung 2 allows 1800 px, so this 1600 px photograph keeps its size and changes its ENCODING: lossless Flate
	// samples become a JPEG, and that is where the saving is.
	was := picturesIn(t, doc)
	var flateLen int
	for _, p := range was {
		if p.w == 1600 {
			flateLen = p.rawLen
		}
	}
	if photoOut.filter != "DCTDecode" || photoOut.w != 1600 || photoOut.rawLen*3 > flateLen {
		t.Errorf("the photograph is %+v (was %d bytes of Flate), want a JPEG a third the size or less", photoOut, flateLen)
	}
	if graphicOut.filter != "FlateDecode" || graphicOut.w != 1000 {
		t.Errorf("the two-colour graphic is %+v, want it left exactly as it was", graphicOut)
	}
	if rep.Shrunk != 1 || rep.Skipped != 1 {
		t.Errorf("report %+v, want one shrunk and one skipped", rep)
	}
}

func TestShrinkPicturesLeavesWhatItCannotKeepHonest(t *testing.T) {
	img := photo(1200, 900)
	data := jpegOf(t, img, 95)
	// Each picture is eligible but for ONE thing; the control, last, is eligible outright. A skip that fired for
	// every picture would pass the first five and fail the control.
	doc := docWithPictures(
		pic{1200, 900, rgb + " /Filter /DCTDecode /Decode [1 0 1 0 1 0]", data},
		pic{1200, 900, rgb + " /Filter /DCTDecode /Mask [0 10 0 10 0 10]", data},
		pic{1200, 900, rgb + " /Filter /DCTDecode /SMaskInData 1", data},
		pic{1200, 900, "/ColorSpace /DeviceCMYK /BitsPerComponent 8 /Filter /DCTDecode", data},
		pic{1200, 900, "/ImageMask true /BitsPerComponent 1 /Filter /FlateDecode", zeros(150 * 900)},
		pic{1200, 900, rgb + " /Filter /DCTDecode", data},
	)
	out, rep, err := ShrinkPictures(doc, 4)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Images != 6 || rep.Shrunk != 1 || rep.Skipped != 5 {
		t.Fatalf("report %+v, want 6 pictures: 1 shrunk and 5 skipped", rep)
	}
	want := map[string]int{reasonDecode: 1, reasonMask: 2, reasonColour: 1, reasonStencil: 1}
	for why, n := range want {
		if rep.Left[why] != n {
			t.Errorf("reason %q counted %d, want %d (all: %v)", why, rep.Left[why], n, rep.Left)
		}
	}
	shrunk := 0
	for _, p := range picturesIn(t, out) {
		if p.w < 1200 {
			shrunk++
		}
	}
	if shrunk != 1 {
		t.Errorf("%d pictures changed size, want exactly the control", shrunk)
	}
}

func TestShrinkPicturesKeepsAPictureThatWouldNotGetSmaller(t *testing.T) {
	small := photo(120, 90)
	doc := docWithPictures(pic{120, 90, rgb + " /Filter /DCTDecode", jpegOf(t, small, 30)})
	_, rep, err := ShrinkPictures(doc, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Kept != 1 || rep.Shrunk != 0 {
		t.Errorf("report %+v, want a small low-quality JPEG kept as it is", rep)
	}
}

func TestShrinkToSizeFindsTheGentlestRungThatFits(t *testing.T) {
	doc := docWithPictures(pic{2400, 1800, rgb + " /Filter /DCTDecode", jpegOf(t, photo(2400, 1800), 95)})
	sizes := make([]int, len(ShrinkLadder))
	for i := range ShrinkLadder {
		out, _, err := ShrinkPictures(doc, i)
		if err != nil {
			t.Fatal(err)
		}
		sizes[i] = len(out)
		if i > 0 && sizes[i] > sizes[i-1] {
			t.Fatalf("rung %d is %d bytes, larger than rung %d (%d): bisection needs a monotone ladder", i, sizes[i], i-1, sizes[i-1])
		}
	}
	// A target between rung 2 and rung 1 must land on rung 2, the gentlest that fits.
	target := (sizes[1] + sizes[2]) / 2
	out, rep, err := ShrinkToSize(doc, target)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Met || rep.Level != 2 || len(out) > target {
		t.Errorf("report %+v, %d bytes for a target of %d; want rung 2 and the target met", rep, len(out), target)
	}
	// A target above everything is met by the gentlest rung; one below everything is not met, and the hardest is returned.
	if _, rep, _ := ShrinkToSize(doc, len(doc)*2); !rep.Met || rep.Level != 0 {
		t.Errorf("a generous target = %+v, want rung 0 and met", rep)
	}
	out, rep, err = ShrinkToSize(doc, 1000)
	if err != nil || rep.Met || rep.Level != len(ShrinkLadder)-1 || len(out) != sizes[len(sizes)-1] {
		t.Errorf("an impossible target = %+v (%d bytes, %v); want the hardest rung and not met", rep, len(out), err)
	}
	if _, _, err := ShrinkToSize(doc, 0); err == nil {
		t.Error("a zero target was accepted")
	}
	if _, _, err := ShrinkPictures(doc, 99); err == nil {
		t.Error("a rung that does not exist was accepted")
	}
}

// A document with nothing this can shrink comes back as the bytes that went in, not as the larger file Optimize's
// rewrite makes of it (measured: +2 % on one real document).
func TestShrinkPicturesNeverAnswersWithALargerFile(t *testing.T) {
	var flat bytes.Buffer
	zw := zlib.NewWriter(&flat)
	zw.Write(bytes.Repeat([]byte{200, 30, 30, 20, 20, 200}, 500*400/2))
	zw.Close()
	doc := docWithPictures(pic{500, 400, rgb + " /Filter /FlateDecode", flat.Bytes()})
	out, rep, err := ShrinkPictures(doc, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, doc) || rep.Shrunk != 0 || rep.After != len(doc) {
		t.Errorf("a document with only a flat graphic came back changed: %d -> %d bytes, %+v", len(doc), len(out), rep)
	}
	if rep.Left[reasonGraphic] != 1 {
		t.Errorf("the reason was not recorded: %v", rep.Left)
	}
}

// A document that needs a password to open is refused as Optimize refuses it, not read wrongly.
func TestShrinkPicturesRefusesADocumentItCannotOpen(t *testing.T) {
	doc := docWithPictures(pic{800, 600, rgb + " /Filter /DCTDecode", jpegOf(t, photo(800, 600), 90)})
	locked, err := Encrypt(doc, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ShrinkPictures(locked, 3); err == nil {
		t.Error("a password-protected document was shrunk without its password")
	}
}

// A Flate picture that declares a small size and inflates past it is a decompression bomb: it is inflated only as far
// as the dictionary declares, left as it is, and the pass does not allocate what the bomb would have cost.
func TestShrinkPicturesDoesNotInflateAPictureBeyondItsDeclaredSize(t *testing.T) {
	var bomb bytes.Buffer
	zw := zlib.NewWriter(&bomb)
	zw.Write(make([]byte, 300<<20)) // 300 MiB of zeros, a few hundred KB compressed
	zw.Close()
	doc := docWithPictures(pic{100, 100, rgb + " /Filter /FlateDecode", bomb.Bytes()})
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	out, rep, err := ShrinkPictures(doc, 3)
	runtime.ReadMemStats(&m1)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Shrunk != 0 || rep.Left[reasonFormat] != 1 || !bytes.Equal(out, doc) {
		t.Errorf("the bomb was not left alone: %+v", rep)
	}
	if grew := (m1.TotalAlloc - m0.TotalAlloc) >> 20; grew > 100 {
		t.Errorf("the pass allocated %d MiB for a picture that declares 30 KB: it inflated the bomb", grew)
	}
}
