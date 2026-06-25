package pdfops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestStampTextLayerSearchable proves the OCR text layer is (a) EXTRACTABLE —
// pdftotext recovers the words — and (b) UNICODE-CORRECT: an accented name, smart
// quotes, an em-dash, and an ellipsis all survive. These are all > U+00FF; the
// core-font (Helvetica) path would have truncated each to a space, so this is the
// regression that pins the Roboto-user-font choice.
func TestStampTextLayerSearchable(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	base, err := testpdf.Text("scan") // a one-page PDF standing in for the scanned image
	if err != nil {
		t.Fatal(err)
	}
	words := []Word{
		{Page: 1, Rect: [4]float64{50, 120, 150, 132}, Text: "Hello"},
		{Page: 1, Rect: [4]float64{50, 100, 150, 112}, Text: "café"},
		{Page: 1, Rect: [4]float64{50, 80, 250, 92}, Text: "“smart”"},
		{Page: 1, Rect: [4]float64{50, 60, 200, 72}, Text: "em—dash"},
		{Page: 1, Rect: [4]float64{50, 40, 200, 52}, Text: "more…"},
		{Page: 1, Rect: [4]float64{50, 20, 200, 32}, Text: "50%"}, // bare % must survive as a literal, not a pdfcpu placeholder
	}
	out, err := StampTextLayer(base, words, "eng")
	if err != nil {
		t.Fatalf("StampTextLayer: %v", err)
	}
	txt := pdfToText(t, out)
	for _, want := range []string{"Hello", "café", "“smart”", "em—dash", "more…", "50%"} {
		if !strings.Contains(txt, want) {
			t.Errorf("extracted text missing %q (Unicode lost?)\n--- pdftotext gave ---\n%s", want, txt)
		}
	}
}

// TestStampTextLayerCyrillicGreek pins the coverage that lets Nib offer Cyrillic and
// Greek OCR languages with NO extra font: pdfcpu's bundled Roboto already carries
// those glyphs, so the invisible layer bakes and extracts correctly. Each sample
// includes that language's characteristic letters (Ukrainian ґ/є/і/ї, Serbian
// ђ/ј/љ/њ/ћ/џ, Macedonian ѓ/ѕ/ќ, Belarusian ў/і). A failure here means Roboto's
// subset lacks a glyph — that language must be pulled from the picker, since the
// drop is silent (the OCR layer would be present but textually empty for it).
func TestStampTextLayerCyrillicGreek(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	samples := map[string]string{
		"rus": "Привет мир",
		"ukr": "Привіт ґ є і ї",
		"bul": "Здравей свят",
		"srp": "Здраво ђ ј љ њ ћ џ",
		"mkd": "Здраво ѓ ѕ ќ",
		"bel": "Прывітанне ў і",
		"ell": "Γειά σου κόσμε",
	}
	for lang, sample := range samples {
		base, err := testpdf.Text("scan")
		if err != nil {
			t.Fatal(err)
		}
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{50, 100, 300, 112}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
			t.Errorf("%s: Roboto does not cover %q — extracted %q; pull this language from the picker", lang, sample, strings.TrimSpace(txt))
		}
	}
}

// TestStampTextLayerLatinExtended pins the same NO-extra-font coverage for the
// Latin-script European languages — they fall through ocrFontFor to pdfcpu's
// bundled Roboto. Each sample carries that language's hardest diacritics, the two
// at-risk being Romanian's comma-below ș/ț (U+0218–021B, distinct from cedilla
// ş/ţ) and Vietnamese's stacked tone+vowel marks (ế/ộ/ữ). A failure means Roboto
// lacks a glyph and that language must be pulled (its OCR layer would be empty).
func TestStampTextLayerLatinExtended(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	samples := map[string]string{
		"ces": "příliš žluťoučký kůň",
		"nld": "drieëntwintig coördinatie",
		"hun": "árvíztűrő tükörfúrógép",
		"pol": "zażółć gęślą jaźń",
		"por": "coração não àção",
		"ron": "așază țară în mâine", // comma-below ș/ț + â/î
		"swe": "Skåne flygande bäckasiner",
		"tur": "İstanbul ığdır şçöü", // dotted/dotless İ/ı + ğ
		"vie": "Tiếng Việt chữ Quốc ngữ",
	}
	for lang, sample := range samples {
		base, err := testpdf.Text("scan")
		if err != nil {
			t.Fatal(err)
		}
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{40, 100, 540, 116}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
			t.Errorf("%s: Roboto does not cover %q — extracted %q; pull this language from the picker", lang, sample, strings.TrimSpace(txt))
		}
	}
}

// TestStampTextLayerNoWords is a no-op pass-through (nothing to OCR on a page).
func TestStampTextLayerNoWords(t *testing.T) {
	base, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampTextLayer(base, nil, "eng")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(base) {
		t.Errorf("no-words output changed: got %d bytes, want %d (pass-through)", len(out), len(base))
	}
}

func pdfToText(t *testing.T, pdf []byte) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "in.pdf")
	if err := os.WriteFile(src, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("pdftotext", "-enc", "UTF-8", src, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	return string(out)
}

// TestStampTextLayerManyWords reproduces a real OCR run: hundreds of words per
// page plus adversarial boxes (degenerate, off-page, inverted, special chars) —
// the server crashed ("ERR_EMPTY_RESPONSE") on a real scan, so this hunts the panic.
func TestStampTextLayerManyWords(t *testing.T) {
	base, err := testpdf.Text("a", "b")
	if err != nil {
		t.Fatal(err)
	}
	var words []Word
	for pg := 1; pg <= 2; pg++ {
		for i := 0; i < 600; i++ {
			x := float64(40 + (i%8)*68)
			y := float64(40 + (i/8)*9)
			words = append(words, Word{Page: pg, Rect: [4]float64{x, y, x + 62, y + 8}, Text: "word"})
		}
	}
	words = append(words,
		Word{Page: 1, Rect: [4]float64{0, 0, 2, 1}, Text: "tiny"},
		Word{Page: 1, Rect: [4]float64{600, 785, 700, 805}, Text: "offpage"},
		Word{Page: 1, Rect: [4]float64{50, 50, 50, 50}, Text: "zeroarea"},
		Word{Page: 1, Rect: [4]float64{50, 100, 200, 90}, Text: "inverted"},
		Word{Page: 1, Rect: [4]float64{50, 120, 200, 132}, Text: "paren)(\\back"},
		Word{Page: 1, Rect: [4]float64{-5, 200, 60, 212}, Text: "negx"},
	)
	if _, err := StampTextLayer(base, words, "eng"); err != nil {
		t.Fatalf("StampTextLayer on %d words: %v", len(words), err)
	}
}

// TestStampTextLayerUnicode pins the Roboto-font path: real OCR output is full of
// characters a core font would drop, and api.TextWatermark rejects the Roboto
// user font unless the default config has loaded it — which StampTextLayer now
// forces up front. These hard tokens (symbols, ligature-ish fractions, control
// chars, an emoji, blanks) must all bake without error.
func TestStampTextLayerUnicode(t *testing.T) {
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	toks := []string{
		"Façade", "€2,775.97", "“smart”", "em—dash", "en–dash", "ellipsis…",
		"§", "№", "½", "Δ", "†", "°C", "±0.5%", "mixed™€§Δ†",
		"a\tb", "nul\x00byte", "back\\slash", "paren()", "\U0001F642", "", "   ",
		// bare % is a pdfcpu placeholder introducer — a lone "%" or a trailing %
		// used to collapse the watermark text to empty and panic (nil bbox). These
		// pin the %→%% escape that StampTextLayer now applies.
		"%", "%%", "50%", "%P", "%p", "%v", "%t", "C++%", "a%b%c",
	}
	var words []Word
	for i, tk := range toks {
		x := 40.0 + float64(i%5)*100
		y := 60.0 + float64(i/5)*14
		words = append(words, Word{Page: 1, Rect: [4]float64{x, y, x + 90, y + 11}, Text: tk})
	}
	out, err := StampTextLayer(base, words, "eng")
	if err != nil {
		t.Fatalf("StampTextLayer on Unicode tokens: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
