package pdfops

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"nib/internal/testpdf"
)

// fitSamples is one line of words per OCR face, keyed by a language that selects it. Each face is its own entry
// because the fit mirrors where pdfcpu sets the baseline inside the form, and that depends on the face's own box.
var fitSamples = map[string][]string{
	"eng":     {"Invoice", "gyp", "ace", "Typography,", "123-45-6789", "I"},
	"rus":     {"Привет", "мир", "договор"},
	"ell":     {"Καλημέρα", "κόσμε"},
	"tha":     {"สวัสดี", "ชาวโลก"},
	"hin":     {"नमस्ते", "दुनिया"},
	"ben":     {"নমস্কার", "বিশ্ব"},
	"tam":     {"வணக்கம்", "உலகம்"},
	"tel":     {"నమస్కారం", "లోకం"},
	"kan":     {"ನಮಸ್ಕಾರ", "ಲೋಕ"},
	"mal":     {"നമസ്കാരം", "ലോകം"},
	"guj":     {"નમસ્તે", "દુનિયા"},
	"pan":     {"ਸਤਿ", "ਦੁਨੀਆ"},
	"ara":     {"مرحبا", "بالعالم"},
	"heb":     {"שלום", "עולם"},
	"chi_sim": {"你好", "世界"},
	"jpn":     {"こんにちは", "世界"},
	"kor":     {"안녕하세요", "세계"},
}

// fitWords lays a sample's words out with DISTINCT widths, and hands them over out of reading order — so a fit
// applied to the wrong word lands a width nobody asked for, on every word.
func fitWords(sample []string) []Word {
	ws := make([]Word, len(sample))
	x := 60.0
	for i, s := range sample {
		w := 31 + 17*float64(i)
		ws[i] = Word{Page: 1, Text: s, Rect: [4]float64{x, 500, x + w, 500 + 9 + float64(i%3)}}
		x += w + 12
	}
	out := make([]Word, 0, len(ws))
	for i := len(ws) - 1; i >= 0; i -= 2 {
		out = append(out, ws[i])
	}
	for i := len(ws) - 2; i >= 0; i -= 2 {
		out = append(out, ws[i])
	}
	return out
}

// hiddenRuns is the page's invisible text as the map reads it, in points from the bottom-left.
type hiddenRun struct {
	text                 string
	x0, y0, x1, y1, size float64
	short                bool
}

func hiddenRuns(t *testing.T, pdf []byte) []hiddenRun {
	t.Helper()
	pm, err := MapPage(pdf, 1)
	if err != nil {
		t.Fatalf("MapPage: %v", err)
	}
	var out []hiddenRun
	for _, tx := range pm.Text {
		if tx.Hidden {
			out = append(out, hiddenRun{tx.Text, tx.Rect[0] * pm.Width, (1 - tx.Rect[3]) * pm.Height, tx.Rect[2] * pm.Width,
				(1 - tx.Rect[1]) * pm.Height, tx.Size * pm.Height, tx.Short})
		}
	}
	return out
}

// TestAStampedWordSpansItsScannedBox — ADR-092. In every face nib stamps an OCR layer in, each word's glyph run must
// span ITS OWN box: the advance from the box's left edge to its right, and the baseline and size that put the glyphs'
// ink on the box's bottom and top. The page already carries a stamp of its own (page numbers), which the pairing has
// to step over.
func TestAStampedWordSpansItsScannedBox(t *testing.T) {
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	numbered, err := StampPageNumbers(base, PageNumberStyle{})
	if err != nil {
		t.Fatal(err)
	}
	const tol = 0.3 // points
	for lang, sample := range fitSamples {
		words := fitWords(sample)
		out, err := StampTextLayer(numbered, words, lang)
		if err != nil {
			t.Errorf("%s: %v", lang, err)
			continue
		}
		runs := hiddenRuns(t, out)
		face := ocrFace(ocrFontFor(lang))
		for _, w := range words {
			var got *hiddenRun
			for i := range runs {
				if runs[i].text == w.Text && math.Abs(runs[i].x0-w.Rect[0]) < 5 {
					got = &runs[i]
				}
			}
			if got == nil {
				t.Errorf("%s %q: no hidden run starts at its box", lang, w.Text)
				continue
			}
			if math.Abs(got.x0-w.Rect[0]) > tol || math.Abs(got.x1-w.Rect[2]) > tol {
				t.Errorf("%s %q: the run spans %.2f..%.2f, its box %.2f..%.2f", lang, w.Text, got.x0, got.x1, w.Rect[0], w.Rect[2])
			}
			_, bottom, top, ok := wordInk(ocrFontFor(lang), face, w.Text)
			if !ok {
				t.Errorf("%s %q: the face draws no ink for the sample", lang, w.Text)
				continue
			}
			em := (w.Rect[3] - w.Rect[1]) / (top - bottom)
			if math.Abs(got.size-em) > 0.02*em {
				t.Errorf("%s %q: set at %.2fpt, and its ink fills the box at %.2fpt", lang, w.Text, got.size, em)
			}
			// runBox puts a run's box descentEm below its baseline.
			if baseline, want := got.y0+descentEm*got.size, w.Rect[1]-bottom*em; math.Abs(baseline-want) > tol {
				t.Errorf("%s %q: baseline at %.2f, and its ink sits on the box's bottom from %.2f", lang, w.Text, baseline, want)
			}
			if got.short {
				t.Errorf("%s %q: a fitted word is marked as stamped short", lang, w.Text)
			}
		}
	}
}

// TestAWordThatCannotBeFittedIsStampedAsBefore: a box with no width has nothing to span. The word is still in the
// layer, at its box's height and unstretched — and the map says so, because that is the stamp ADR-091 reads out to
// the next word.
func TestAWordThatCannotBeFittedIsStampedAsBefore(t *testing.T) {
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampTextLayer(base, []Word{
		{Page: 1, Text: "flat", Rect: [4]float64{100, 400, 100, 412}},
		{Page: 1, Text: "Invoice", Rect: [4]float64{200, 400, 280, 412}},
	}, "eng")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]hiddenRun{}
	for _, r := range hiddenRuns(t, out) {
		seen[r.text] = r
	}
	flat, ok := seen["flat"]
	if !ok {
		t.Fatal("the unfittable word is missing from the layer")
	}
	if !flat.short || math.Abs(flat.size-12) > 0.01 {
		t.Errorf("the unfittable word: short=%v at %.2fpt, want the old stamp — short, at its box's 12pt", flat.short, flat.size)
	}
	if inv := seen["Invoice"]; inv.short || math.Abs(inv.x1-280) > 0.3 {
		t.Errorf("its neighbour was not fitted: short=%v, ends at %.2f", inv.short, inv.x1)
	}
}

var popplerWord = regexp.MustCompile(`<word xMin="([\d.-]+)" yMin="[\d.-]+" xMax="([\d.-]+)" yMax="[\d.-]+">([^<]*)</word>`)

// TestAnotherProgramReadsTheWordAtItsScannedWidth is the point of ADR-092: nib's own redaction already read a short
// stamp out to the next word (ADR-091); a different program cannot. Poppler's word boxes are that program.
func TestAnotherProgramReadsTheWordAtItsScannedWidth(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	words := fitWords(fitSamples["eng"])
	out, err := StampTextLayer(base, words, "eng")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "layer.pdf")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	xml, err := exec.Command("pdftotext", "-bbox", path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	got := map[string][2]float64{}
	for _, m := range popplerWord.FindAllStringSubmatch(string(xml), -1) {
		x0, _ := strconv.ParseFloat(m[1], 64)
		x1, _ := strconv.ParseFloat(m[2], 64)
		got[m[3]] = [2]float64{x0, x1}
	}
	for _, w := range words {
		g, ok := got[w.Text]
		if !ok {
			t.Errorf("poppler does not report %q", w.Text)
			continue
		}
		if math.Abs(g[0]-w.Rect[0]) > 0.5 || math.Abs(g[1]-w.Rect[2]) > 0.5 {
			t.Errorf("poppler reads %q at %.2f..%.2f, its box is %.2f..%.2f", w.Text, g[0], g[1], w.Rect[0], w.Rect[2])
		}
	}
}

// TestTheFitKeepsATurnedPagesPlacement: on a turned page pdfcpu and stampInPlace decide which way the word runs; the
// fit is multiplied in ahead of that, so the word must stretch along the same edge it was placed on.
func TestTheFitKeepsATurnedPagesPlacement(t *testing.T) {
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	for _, deg := range []int{90, 180, 270} {
		turned, err := Rotate(base, []string{"1"}, deg)
		if err != nil {
			t.Fatalf("rotate %d: %v", deg, err)
		}
		word := Word{Page: 1, Text: "Invoice", Rect: [4]float64{100, 300, 180, 312}}
		flat := word
		flat.Rect[2] = flat.Rect[0] // unfittable: where the old stamp puts it
		was, err := StampTextLayer(turned, []Word{flat}, "eng")
		if err != nil {
			t.Fatal(err)
		}
		now, err := StampTextLayer(turned, []Word{word}, "eng")
		if err != nil {
			t.Fatal(err)
		}
		a, b := hiddenRuns(t, was), hiddenRuns(t, now)
		if len(a) != 1 || len(b) != 1 {
			t.Fatalf("turned %d: %d and %d hidden runs", deg, len(a), len(b))
		}
		oldW, oldH := a[0].x1-a[0].x0, a[0].y1-a[0].y0
		newW, newH := b[0].x1-b[0].x0, b[0].y1-b[0].y0
		if (oldW > oldH) != (newW > newH) {
			t.Errorf("turned %d: the stamp ran %.1f×%.1f and the fitted word runs %.1f×%.1f — along the other edge", deg, oldW, oldH, newW, newH)
		}
		if long := math.Max(newW, newH); math.Abs(long-80) > 0.5 {
			t.Errorf("turned %d: the fitted word is %.2f long, its box 80", deg, long)
		}
	}
}
