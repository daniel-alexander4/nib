package pdfops

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// /pending 881 — a page's content stream is held unfiltered while pdfcpu stamps several watermarks onto it
// (`contentHeldUnfiltered`). pdfcpu's own stamp, one deflate of the page a watermark, is the definition of what a
// stamp writes; these tests hold the faster path to it.

// asPdfcpuStamps runs f with the hold off: pdfcpu's own stamp, the oracle.
func asPdfcpuStamps(f func()) {
	holdContentWhileStamping = false
	defer func() { holdContentWhileStamping = true }()
	f()
}

// holdFixtures is every shape of `/Contents` the hold meets: the stream pdfcpu patches behind one filter, behind a
// chain, behind none, empty, first of several, shared by two pages, on a turned page whose box is off the origin,
// behind a filter pdfcpu reads past and cannot write again (an image filter), and holding bytes its filter does not
// decode.
func holdFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	const draw = "0.9 g 10 10 100 100 re f "
	flate := func(body string) string { return optStream(flated([]byte(body)), "/Filter /FlateDecode") }
	doc := func(page3, page5 string, objs map[int]string) []byte {
		objs[1] = "<< /Type /Catalog /Pages 2 0 R >>"
		objs[2] = "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"
		objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> " + page3 + " >>"
		objs[5] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> " + page5 + " >>"
		return assembleFixture(objs)
	}
	text, err := testpdf.Text("the page's own print", "and a second page")
	if err != nil {
		t.Fatal(err)
	}
	marked, err := StampWatermark(text, "DRAFT", WatermarkStyle{})
	if err != nil {
		t.Fatal(err)
	}
	hexed := strings.ToUpper(hex.EncodeToString([]byte(flated([]byte(draw))))) + ">"
	return map[string][]byte{
		"a scan":                   scannedPages(t, 2),
		"print under a watermark":  marked,
		"one flate stream a page":  doc("/Contents 4 0 R", "/Contents 6 0 R", map[int]string{4: flate(draw), 6: flate(draw + draw)}),
		"a chain of two filters":   doc("/Contents 4 0 R", "/Contents 6 0 R", map[int]string{4: optStream(hexed, "/Filter [/ASCIIHexDecode /FlateDecode]"), 6: flate(draw)}),
		"no filter":                doc("/Contents 4 0 R", "/Contents 6 0 R", map[int]string{4: optStream(draw, ""), 6: optStream(draw, "")}),
		"an empty flate stream":    doc("/Contents 4 0 R", "/Contents 6 0 R", map[int]string{4: flate(""), 6: flate(draw)}),
		"the first of two streams": doc("/Contents [4 0 R 6 0 R]", "/Contents 7 0 R", map[int]string{4: flate("q " + draw), 6: flate("Q " + draw), 7: "[4 0 R 6 0 R]"}),
		"one stream on two pages":  doc("/Contents 4 0 R", "/Contents 4 0 R", map[int]string{4: flate(draw)}),
		"a turned page, its box off the origin": doc("/Rotate 90 /CropBox [29.75 84.2 500 700] /Contents 4 0 R", "/Rotate 270 /Contents 6 0 R",
			map[int]string{4: flate(draw), 6: flate(draw)}),
		// pdfcpu's run-length decode of nothing leaves no content at all, which it reads as "not decoded yet".
		"an empty run-length stream":            doc("/Contents 4 0 R", "/Contents 6 0 R", map[int]string{4: optStream("\x80", "/Filter /RunLengthDecode"), 6: flate(draw)}),
		"a flate stream that does not inflate":  doc("/Contents 4 0 R", "/Contents 6 0 R", map[int]string{4: optStream("not zlib at all", "/Filter /FlateDecode"), 6: flate(draw)}),
		"a filter that cannot be written again": doc("/Contents 4 0 R", "/Contents 6 0 R", map[int]string{4: optStream("abcd", "/Filter /JPXDecode"), 6: flate(draw)}),
	}
}

// holdWords is a layer of several words a page on both pages: fitted ones, one with nothing to fit, in two paragraphs.
func holdWords() []Word {
	var words []Word
	for pg := 1; pg <= 2; pg++ {
		for i, tx := range []string{"Invoice", "2024", "Acme", "Ltd", "total", "due"} {
			x, y := 72+float64(i%3)*90, 700-float64(i/3)*24
			words = append(words, Word{Page: pg, Rect: [4]float64{x, y, x + 70, y + 12}, Text: tx, Block: 1, Para: 1 + i/3, Line: 1 + i/3})
		}
		words = append(words, Word{Page: pg, Rect: [4]float64{72, 600, 72, 600}, Text: "flat", Block: 1, Para: 3, Line: 3})
	}
	return words
}

// plainStamps is three visible text stamps a page through the package's stamp door, with no fit after them — so the
// stream pdfcpu patched is the stream that is written.
func plainStamps(pdf []byte) ([]byte, error) {
	model.NewDefaultConfiguration()
	wms := map[int][]*model.Watermark{}
	for pg := 1; pg <= 2; pg++ {
		for i := 0; i < 3; i++ {
			wm, err := api.TextWatermark(context.Background(), fmt.Sprintf("stamp %d", i),
				fmt.Sprintf("fontname:Helvetica, points:12, scalefactor:1 abs, position:bl, offset:%d %d, rotation:0", 100+40*i, 300+20*i),
				true, false, types.POINTS, nil)
			if err != nil {
				return nil, err
			}
			wms[pg] = append(wms[pg], wm)
		}
	}
	return addWatermarks(pdf, wms)
}

// firstStreamFilters is the filters each page's first content stream is written behind.
func firstStreamFilters(t *testing.T, pdf []byte) [][]string {
	t.Helper()
	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out [][]string
	for _, pg := range pdfread.Pages(ctx) {
		_, sd, err := firstContentStream(ctx, pg.Dict)
		if err != nil || sd == nil {
			t.Fatalf("page %d: no first content stream (%v)", pg.Nr, err)
		}
		names := []string{}
		for _, f := range sd.FilterPipeline {
			names = append(names, f.Name)
		}
		out = append(out, names)
	}
	return out
}

// ownWordsOf is `ownOCRWords`' verdict on every page: ADR-101's recognition of nib's own layer.
func ownWordsOf(t *testing.T, pdf []byte) [][]ownWord {
	t.Helper()
	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out [][]ownWord
	for _, pg := range pdfread.Pages(ctx) {
		src, err := pdfread.PageContent(ctx, pg.Dict, pg.Nr)
		if err != nil {
			out = append(out, nil)
			continue
		}
		out = append(out, ownOCRWords(ctx, pg.Attrs.Resources, src, newFormWalkBudget(1)))
	}
	return out
}

// sameAsWritten fails where two outputs differ in anything a reader of the document is given: each page's content,
// its map (every run's text, box, ink and flags), the structure tree, the content digest, and which words are nib's.
func sameAsWritten(t *testing.T, held, direct []byte) {
	t.Helper()
	for pg := 1; pg <= 2; pg++ {
		if a, b := pageStream(t, held, pg), pageStream(t, direct, pg); string(a) != string(b) {
			t.Errorf("page %d draws\n%.400q\nheld, and\n%.400q\nas pdfcpu stamps it", pg, a, b)
		}
		a, aerr := MapPage(held, pg)
		b, berr := MapPage(direct, pg)
		if aerr != nil || berr != nil || !reflect.DeepEqual(a, b) {
			t.Errorf("page %d maps differently (%v, %v):\n%+v\n%+v", pg, aerr, berr, a.Text, b.Text)
		}
	}
	a, aerr := ReadStructure(held)
	b, berr := ReadStructure(direct)
	if aerr != nil || berr != nil || !reflect.DeepEqual(a, b) {
		t.Errorf("the structure trees differ (%v, %v): %d elements held, %d as pdfcpu stamps it", aerr, berr, len(a.Elements), len(b.Elements))
	}
	da, aerr := ContentDigest(held)
	db, berr := ContentDigest(direct)
	if aerr != nil || berr != nil || da != db {
		t.Errorf("the content digests differ: %s (%v) held, %s (%v) as pdfcpu stamps it", da, aerr, db, berr)
	}
	if a, b := ownWordsOf(t, held), ownWordsOf(t, direct); !reflect.DeepEqual(a, b) {
		t.Errorf("nib's own words are read differently:\n%+v\n%+v", a, b)
	}
	if a, b := firstStreamFilters(t, held), firstStreamFilters(t, direct); !reflect.DeepEqual(a, b) {
		t.Errorf("the first content streams are behind %v held, and %v as pdfcpu stamps it", a, b)
	}
}

// TestStampingWithTheContentHeldWritesWhatPdfcpusOwnStampWrites: for every shape of `/Contents`, an OCR layer —
// stamped, fitted and described — and a set of plain stamps come out as pdfcpu's own stamp writes them.
func TestStampingWithTheContentHeldWritesWhatPdfcpusOwnStampWrites(t *testing.T) {
	log.SetOutput(io.Discard) // the layer logs each word with nothing to fit; the fixtures have one a page on purpose
	defer log.SetOutput(os.Stderr)
	sawTagged, sawRefused, sawOwn, sawReversed := false, false, false, false
	hebrew := func() []Word {
		var words []Word
		for pg := 1; pg <= 2; pg++ {
			for i, tx := range []string{"שלום", "ירושלים", "עולם"} {
				x := 72 + float64(i)*120
				words = append(words, Word{Page: pg, Rect: [4]float64{x, 700, x + 90, 714}, Text: tx, Block: 1, Para: 1, Line: 1})
			}
		}
		return words
	}
	for name, base := range holdFixtures(t) {
		for op, stamp := range map[string]func() ([]byte, bool, error){
			"an OCR layer": func() ([]byte, bool, error) { return TagOCRLayer(base, holdWords(), "eng") },
			// A right-to-left word is set in reverse and marked as that, after the stamp (ADR-097).
			"a Hebrew OCR layer": func() ([]byte, bool, error) { return TagOCRLayer(base, hebrew(), "heb") },
			"plain stamps":       func() ([]byte, bool, error) { out, err := plainStamps(base); return out, false, err },
		} {
			t.Run(name+", "+op, func(t *testing.T) {
				held, tagged, err := stamp()
				var direct []byte
				var dTagged bool
				var derr error
				asPdfcpuStamps(func() { direct, dTagged, derr = stamp() })
				if (err == nil) != (derr == nil) || tagged != dTagged {
					t.Fatalf("held: tagged %v, %v; as pdfcpu stamps it: tagged %v, %v", tagged, err, dTagged, derr)
				}
				if err != nil {
					// v0.16 wraps the refusal in where it happened ("page 1, watermark 0: update contents: …"); the REASON is the tail.
					if !strings.HasSuffix(fmt.Sprint(derr), fmt.Sprint(err)) {
						t.Errorf("refused with %q held, and %q as pdfcpu stamps it", err, derr)
					}
					sawRefused = true
					return
				}
				sameAsWritten(t, held, direct)
				sawTagged = sawTagged || tagged
				if op == "a Hebrew OCR layer" {
					for _, tx := range hiddenOn(t, held, 1) {
						sawReversed = sawReversed || tx.Reversed
					}
				}
				if op == "an OCR layer" {
					for _, page := range ownWordsOf(t, held) {
						sawOwn = sawOwn || len(page) == len(holdWords())/2
					}
				}
			})
		}
	}
	// The stimulus: the comparison saw a described layer, a refusal, a layer `ownOCRWords` recognises whole, and a
	// word the map reads as set in reverse.
	if !sawTagged || !sawRefused || !sawOwn || !sawReversed {
		t.Errorf("the fixtures gave a described layer: %v, a refused stamp: %v, a recognised layer: %v, a reversed word: %v — want all four",
			sawTagged, sawRefused, sawOwn, sawReversed)
	}
}

// TestAHeldContentStreamIsWrittenBehindItsOwnFilters: after plain stamps, each fixture's first content stream is
// behind exactly the filters it came with — one, a chain in its order, or none — and decodes to the page's drawing
// with the stamps after it.
func TestAHeldContentStreamIsWrittenBehindItsOwnFilters(t *testing.T) {
	fixtures := holdFixtures(t)
	for name, want := range map[string][]string{
		"one flate stream a page":  {"FlateDecode"},
		"a chain of two filters":   {"ASCIIHexDecode", "FlateDecode"},
		"no filter":                {},
		"an empty flate stream":    {"FlateDecode"},
		"the first of two streams": {"FlateDecode"},
	} {
		was := firstStreamFilters(t, fixtures[name])
		if !reflect.DeepEqual(was[0], want) {
			t.Fatalf("%s: the fixture's first stream is behind %v, want %v", name, was[0], want)
		}
		out, err := plainStamps(fixtures[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if now := firstStreamFilters(t, out); !reflect.DeepEqual(now[0], want) {
			t.Errorf("%s: the stamped page's first stream is behind %v; it came behind %v", name, now[0], want)
		}
		content := string(pageStream(t, out, 1))
		if n := strings.Count(content, "/Artifact <</Subtype /Watermark"); n != 3 {
			t.Errorf("%s: the stamped page draws %d stamps, want 3", name, n)
		}
		if name != "an empty flate stream" && !strings.Contains(content, "10 10 100 100 re f") {
			t.Errorf("%s: the stamped page no longer draws its own content: %.200q", name, content)
		}
	}
}

// TestAPageOfManyWordsIsNotDeflatedOncePerWord is the cost the hold removes, read as what the stamp allocates: every
// deflate of the growing page allocates its output, so pdfcpu's own stamp allocates more for each word already on
// the page. Measured at 600 words on one scanned page: 775 MB held against 1,245 MB (0.62).
func TestAPageOfManyWordsIsNotDeflatedOncePerWord(t *testing.T) {
	base := scannedPages(t, 1)
	var words []Word
	for i := 0; i < 600; i++ {
		x, y := 30+float64(i%12)*46, 20+float64(i/12)*15
		words = append(words, Word{Page: 1, Rect: [4]float64{x, y, x + 40, y + 9}, Text: fmt.Sprintf("w%dord", i%97)})
	}
	stamp := func() {
		if _, err := StampTextLayer(base, words, "eng"); err != nil {
			t.Fatal(err)
		}
	}
	stamp() // the fonts and the face are read once, outside both measurements
	held := allocated(stamp)
	var direct uint64
	asPdfcpuStamps(func() { direct = allocated(stamp) })
	if held*4 > direct*3 {
		t.Errorf("stamping 600 words on one page allocated %d MB with the content held and %d MB as pdfcpu stamps it — "+
			"the page is still deflated once a word", held>>20, direct>>20)
	}
}
