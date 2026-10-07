package pdfops

import (
	"bytes"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"nib/internal/testpdf"

	pdffont "github.com/pdfcpu/pdfcpu/pkg/font"
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
	// ink0, ink1 is the run's own ink up the page, where the map gives one (ADR-093); inked says it did.
	ink0, ink1 float64
	inked      bool
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
			r := hiddenRun{text: tx.Text, x0: tx.Rect[0] * pm.Width, y0: (1 - tx.Rect[3]) * pm.Height, x1: tx.Rect[2] * pm.Width,
				y1: (1 - tx.Rect[1]) * pm.Height, size: tx.Size * pm.Height, short: tx.Short}
			if len(tx.Ink) == 2 {
				r.ink0, r.ink1, r.inked = (1-tx.Ink[1])*pm.Height, (1-tx.Ink[0])*pm.Height, true
			}
			out = append(out, r)
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
			// The map gives a fitted word its own ink beside the line's reach, and the fit put that ink on the scanned
			// box's bottom and top — so the ink read back IS the box handed in (ADR-093; /pending 851 parts 1 and 2:
			// the line's reach stood over the line above and short of a descender).
			if !got.inked || math.Abs(got.ink0-w.Rect[1]) > tol || math.Abs(got.ink1-w.Rect[3]) > tol {
				t.Errorf("%s %q: the map has its ink %.2f..%.2f up the page (given: %v), its scanned box %.2f..%.2f", lang, w.Text, got.ink0, got.ink1, got.inked, w.Rect[1], w.Rect[3])
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
	// A short stamp has no ink of its own on the map: it was put on no box, and ADR-091's reading starts from its reach.
	if flat.inked {
		t.Errorf("the unfittable word was given ink %.2f..%.2f, as though it had been fitted", flat.ink0, flat.ink1)
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

// TestOnlyAnUprightRunInAnOCRFaceHasItsInkMeasured: the ink is measured from nib's own copy of the face, so it is
// the ink of nothing else — another font's hidden text has only the line's reach — and a height about the baseline
// means nothing for a run that does not read across.
func TestOnlyAnUprightRunInAnOCRFaceHasItsInkMeasured(t *testing.T) {
	run := func(font, text string, rotated bool) textRun {
		return textRun{text: text, baseFont: font, size: 10, x: 50, y: 100, width: 40, rotated: rotated}
	}
	for _, font := range []string{ocrFont, "ABCDEF+" + ocrFont} {
		// "Invoice" in Roboto at 10pt: no descender, a capital's height.
		bottom, top, ok := fittedInk(run(font, "Invoice", false))
		if !ok || math.Abs(bottom-100) > 0.2 || math.Abs(top-107.2) > 0.2 {
			t.Errorf("%s: ink %.2f..%.2f (ok=%v), want 100..107.2", font, bottom, top, ok)
		}
	}
	// A face named in the table, not only the default one — and a descender below the baseline.
	if bottom, top, ok := fittedInk(run("NotoSansArabic-Regular", "مرحبا", false)); !ok || !(top > 100 && bottom < 100) {
		t.Errorf("an Arabic word in nib's Arabic face: ink %.2f..%.2f (ok=%v), want ink either side of the baseline at 100", bottom, top, ok)
	}
	if _, _, ok := fittedInk(run("Helvetica", "Invoice", false)); ok {
		t.Error("a run in a face nib does not stamp was measured by Roboto's ink")
	}
	if _, kept := ocrFaces.Load("Helvetica"); kept {
		t.Error("a font name from a document was remembered as a face: the table is what bounds that cache")
	}
	if _, _, ok := fittedInk(run(ocrFont, "Invoice", true)); ok {
		t.Error("a turned run was given a height about its baseline")
	}
}

// TestAPageWhoseContentCannotBeDecodedRefusesTheLayerAndNeverLosesItQuietly settles /pending 851 part 5 by running
// it. pdfcpu has a branch that skips a content stream behind a filter it does not implement — JBIG2 and JPX, which are
// image filters — with no stamp and no error (`patchFirstContentStreamForWatermark`). Driven through nib, that is not
// what answers. Where the stream pdfcpu must patch cannot be decoded, the whole stamp is REFUSED. Where a readable
// stream comes first, the words are stamped, the page cannot be read back to pair them, and they are COUNTED — the
// one input found that makes ADR-092's `unpaired` count fire — while every other page is fitted as usual.
func TestAPageWhoseContentCannotBeDecodedRefusesTheLayerAndNeverLosesItQuietly(t *testing.T) {
	doc := func(contents string) []byte {
		return assembleFixture(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents " + contents + " >>",
			4: "<< /Length 4 /Filter /JPXDecode >>\nstream\nabcd\nendstream",
			5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 6 0 R >>",
			6: "<< /Length 0 >>\nstream\n\nendstream",
		})
	}
	words := []Word{
		{Page: 1, Text: "lost", Rect: [4]float64{100, 400, 140, 412}},
		{Page: 2, Text: "Invoice", Rect: [4]float64{200, 400, 280, 412}},
	}
	for _, contents := range []string{"4 0 R", "[4 0 R]"} {
		if out, err := StampTextLayer(doc(contents), words, "eng"); err == nil {
			t.Errorf("/Contents %s: the layer was written (%d bytes) over a page whose content cannot be read", contents, len(out))
		}
	}
	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)
	out, err := StampTextLayer(doc("[6 0 R 4 0 R]"), words, "eng")
	if err != nil {
		t.Fatalf("a readable stream comes first, and the layer was refused: %v", err)
	}
	if !strings.Contains(logged.String(), "0 word(s) with nothing to fit and 1 on a page that could not be paired") {
		t.Errorf("the word on the page that cannot be read back was not counted; logged %q", logged.String())
	}
	pm, err := MapPage(out, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(pm.Text) != 1 || !pm.Text[0].Hidden || pm.Text[0].Short || math.Abs(pm.Text[0].Rect[2]*pm.Width-280) > 0.3 {
		t.Errorf("the readable page's word was not written and fitted: %+v", pm.Text)
	}
}

// TestOnlyAHiddenWordInsideAFormIsGivenItsInk: the ink is what the FIT put on a box, and the fit is written only for
// a hidden word drawn through a form. The same face set as print, or hidden straight on the page, was fitted to
// nothing — its glyphs' ink says where the glyphs are, not where a scanned word is.
func TestOnlyAHiddenWordInsideAFormIsGivenItsInk(t *testing.T) {
	sp := displaySpace{box: [4]float64{0, 0, 612, 792}, w: 612, h: 792}
	run := func(tr int, inForm bool) textRun {
		r := textRun{text: "I", baseFont: ocrFont, size: 10, x: 50, y: 100, width: 3, inForm: inForm,
			glyphs: []runGlyph{{text: "I", advance: 3}}}
		r.state.tr, r.state.ctm = tr, runMatrix{2, 0, 0, 1, 0, 0} // wider than tall: a fitted word, not a short one
		return r
	}
	if m, ok := mapText(sp, run(3, true)); !ok || len(m.Ink) != 2 || len(m.Cuts) != 2 {
		t.Fatalf("a fitted word has no ink on the map: %+v (ok=%v)", m, ok)
	}
	if m, _ := mapText(sp, run(0, true)); len(m.Ink) != 0 {
		t.Errorf("print inside a form was given ink %v", m.Ink)
	}
	if m, _ := mapText(sp, run(3, false)); len(m.Ink) != 0 {
		t.Errorf("hidden text set straight on the page was given ink %v", m.Ink)
	}
}

// TestARightToLeftWordIsSetWhereItsLettersAre — ADR-097. A Hebrew or Arabic word's first letter is the RIGHTMOST of
// the scanned word, so the stamp sets the word last letter first and marks it `/ReversedChars`; the map hands the
// word back in reading order with its glyph boundaries running from the right. Set in reading order (as it was) the
// first letter's boundaries were the box's LEFT end, and every other reader had the word backwards.
func TestARightToLeftWordIsSetWhereItsLettersAre(t *testing.T) {
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	for lang, sample := range map[string][]string{"heb": {"שלום", "ירושלים", "שָׁלוֹם"}, "ara": {"مرحبا", "بالعالم"}} {
		words := fitWords(sample)
		for _, stamp := range []struct {
			name string
			do   func() ([]byte, error)
		}{
			{"stamped", func() ([]byte, error) { return StampTextLayer(base, words, lang) }},
			{"tagged", func() ([]byte, error) { out, _, err := TagOCRLayer(base, words, lang); return out, err }},
		} {
			out, err := stamp.do()
			if err != nil {
				t.Fatalf("%s %s: %v", lang, stamp.name, err)
			}
			pm, err := MapPage(out, 1)
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, tx := range pm.Text {
				if !tx.Hidden {
					continue
				}
				var w *Word
				for i := range words {
					if words[i].Text == tx.Text {
						w = &words[i]
					}
				}
				if w == nil {
					t.Errorf("%s %s: the map reads %q, which is no word in reading order (%q)", lang, stamp.name, tx.Text, sample)
					continue
				}
				found++
				if !tx.Reversed || tx.Short {
					t.Errorf("%s %s %q: reversed %v short %v, want a fitted word marked as set in reverse", lang, stamp.name, tx.Text, tx.Reversed, tx.Short)
				}
				if strings.Join(tx.Chars, "") != w.Text || len(tx.Cuts) != len(tx.Chars)+1 {
					t.Errorf("%s %s %q: chars %q with %d cuts", lang, stamp.name, w.Text, tx.Chars, len(tx.Cuts))
					continue
				}
				// The boundaries run from the box's right edge to its left, and each letter is as wide as its own
				// advance: the first letter of the word is at the right, where the scan has it.
				if math.Abs(tx.Cuts[0]*pm.Width-w.Rect[2]) > 0.05 || math.Abs(tx.Cuts[len(tx.Cuts)-1]*pm.Width-w.Rect[0]) > 0.05 {
					t.Errorf("%s %s %q: cuts run %.2f..%.2f, the box is %.2f..%.2f from its right", lang, stamp.name, w.Text,
						tx.Cuts[0]*pm.Width, tx.Cuts[len(tx.Cuts)-1]*pm.Width, w.Rect[2], w.Rect[0])
				}
				face, total := ocrFontFor(lang), 0.0
				for _, r := range w.Text {
					total += float64(pdffont.CharWidth(face, r))
				}
				for i, c := range tx.Chars {
					want := float64(pdffont.CharWidth(face, []rune(c)[0])) / total * (w.Rect[2] - w.Rect[0])
					if got := (tx.Cuts[i] - tx.Cuts[i+1]) * pm.Width; math.Abs(got-want) > 0.05 {
						t.Errorf("%s %s %q: letter %d (%q) is %.2fpt wide leftward from its cut, want %.2f", lang, stamp.name, w.Text, i, c, got, want)
					}
				}
			}
			if found != len(words) {
				t.Errorf("%s %s: %d of %d words read back", lang, stamp.name, found, len(words))
			}
			if _, err := exec.LookPath("pdftotext"); err != nil || stamp.name != "stamped" {
				continue
			}
			path := filepath.Join(t.TempDir(), "layer.pdf")
			if err := os.WriteFile(path, out, 0o600); err != nil {
				t.Fatal(err)
			}
			txt, err := exec.Command("pdftotext", path, "-").Output()
			if err != nil {
				t.Fatalf("pdftotext: %v", err)
			}
			for _, w := range words {
				if !strings.Contains(string(txt), w.Text) {
					t.Errorf("%s: poppler does not read %q in reading order: %q", lang, w.Text, txt)
				}
			}
		}
	}
}

// TestOnlyAWholeRightToLeftWordIsSetInReverse: a reader turns a right-to-left word round as a whole only when the
// whole word runs one way. A digit or a Latin letter keeps its own direction inside it, and a bracket may be
// mirrored, so those words are set as they were — in reading order, unmarked, and taken whole by a match.
func TestOnlyAWholeRightToLeftWordIsSetInReverse(t *testing.T) {
	for _, c := range []struct {
		in, set  string
		reversed bool
	}{
		{"שלום", "םולש", true},
		{"مرحبا،", "،ابحرم", true}, // a mark of punctuation goes round with its word
		{"Invoice", "Invoice", false},
		{"123-45-6789", "123-45-6789", false},
		{"٢٠٢٠", "٢٠٢٠", false},     // Arabic-Indic digits run left to right
		{"ב-2020", "ב-2020", false}, // European digits inside a Hebrew word
		{"م2", "م2", false},
		{"م٢", "م٢", false},
		{"שלוםabc", "שלוםabc", false},
		{"(שלום)", "(שלום)", false},
		{"...", "...", false},
		{"", "", false},
	} {
		if set, reversed := setOrder(c.in); set != c.set || reversed != c.reversed {
			t.Errorf("setOrder(%q) = %q, %v; want %q, %v", c.in, set, reversed, c.set, c.reversed)
		}
	}
	// And such a word, stamped, is not marked: the map reads it as it is set.
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampTextLayer(base, []Word{{Page: 1, Text: "م2", Rect: [4]float64{60, 500, 100, 512}}}, "ara")
	if err != nil {
		t.Fatal(err)
	}
	pm, err := MapPage(out, 1)
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, tx := range pm.Text {
		if tx.Hidden {
			seen = true
			if tx.Reversed || tx.Text != "م2" || len(tx.Cuts) != 3 || tx.Cuts[0] > tx.Cuts[2] {
				t.Errorf("a word with a digit in it reads %q reversed %v cuts %v; want it as it was stamped, from the left", tx.Text, tx.Reversed, tx.Cuts)
			}
		}
	}
	if !seen {
		t.Error("the word is not in the layer")
	}

	// A reversed word that cannot be FITTED — a box with no height — is still marked: unmarked, it would be read in
	// the order it is set, which is backwards.
	out, err = StampTextLayer(base, []Word{{Page: 1, Text: "שלום", Rect: [4]float64{60, 500, 100, 500}}}, "heb")
	if err != nil {
		t.Fatal(err)
	}
	if pm, err = MapPage(out, 1); err != nil {
		t.Fatal(err)
	}
	seen = false
	for _, tx := range pm.Text {
		if tx.Hidden {
			seen = true
			if !tx.Reversed || !tx.Short || tx.Text != "שלום" {
				t.Errorf("an unfitted right-to-left word reads %q reversed %v short %v; want it in reading order, marked, and short", tx.Text, tx.Reversed, tx.Short)
			}
		}
	}
	if !seen {
		t.Error("the unfitted word is not in the layer")
	}
}
