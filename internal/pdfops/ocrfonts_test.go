package pdfops

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestMain installs the vendored non-Latin OCR fonts before any test runs — pdfcpu
// loads its font registry exactly once (sync.Once) on the first font op, so the
// Noto .gob files must be on disk before then, exactly as the server does at
// startup. Without this, whichever font test runs first would freeze a registry
// that lacks Thai/Devanagari.
func TestMain(m *testing.M) {
	if err := InstallOCRFonts(); err != nil {
		log.Printf("InstallOCRFonts: %v", err) // the Thai/Devanagari test will fail loudly
	}
	os.Exit(m.Run())
}

// TestStampTextLayerThaiDevanagari is the coverage gate for the non-Latin scripts
// that need a vendored font (unlike Cyrillic/Greek, which ride Roboto). It stamps a
// sample in each script and asserts it round-trips through pdftotext — proving the
// font is installed, selected by lang, and bakes an extractable invisible layer.
func TestStampTextLayerThaiDevanagari(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	samples := map[string]string{
		"tha": "สวัสดีชาวโลก",
		"hin": "नमस्ते दुनिया",
	}
	for lang, sample := range samples {
		base, err := testpdf.Text("scan")
		if err != nil {
			t.Fatal(err)
		}
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{50, 100, 320, 116}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
			t.Errorf("%s: %q did not round-trip — extracted %q (font missing or lacks glyphs)", lang, sample, strings.TrimSpace(txt))
		}
	}
}

// TestStampTextLayerArabicHebrew is the coverage gate for the RTL scripts. It can't
// use strings.Contains like the LTR gate: pdfcpu writes the invisible layer in
// correct logical order with a correct /ToUnicode, but a bidi-aware extractor
// (poppler/pdftotext) reverses RTL text on *display*, so the extracted string is
// the sample reversed (plus U+202A/B bidi controls). We therefore assert a
// rune-multiset round-trip — every glyph of the sample comes back — which proves
// font coverage + extractability without depending on display order. (Nib's own
// Find path, pdf.js getTextContent, reads the logical-order bytes directly.)
func TestStampTextLayerArabicHebrew(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	samples := map[string]string{
		"ara": "مرحبا بالعالم",
		"heb": "שלום עולם",
	}
	for lang, sample := range samples {
		base, err := testpdf.Text("scan")
		if err != nil {
			t.Fatal(err)
		}
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{50, 100, 320, 116}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		got := pdfToText(t, out)
		if missing := missingRunes(sample, got); missing != "" {
			t.Errorf("%s: %q did not round-trip — glyphs %q absent from extracted %q (font missing or lacks glyphs)",
				lang, sample, missing, strings.TrimSpace(got))
		}
	}
}

// missingRunes returns the runes of want (ignoring spaces) that do not appear in
// got with at least the same multiplicity — order-independent, so it tolerates the
// bidi reversal a display-oriented extractor applies to RTL text.
func missingRunes(want, got string) string {
	counts := map[rune]int{}
	for _, r := range got {
		counts[r]++
	}
	var missing []rune
	for _, r := range want {
		if r == ' ' {
			continue
		}
		if counts[r] <= 0 {
			missing = append(missing, r)
			continue
		}
		counts[r]--
	}
	return string(missing)
}

// TestStampTextLayerCJK is the coverage gate for the CJK scripts: the vendored
// Droid pan-CJK face, installed by its embedded PostScript name (DroidSansFallback),
// must carry glyphs for Simplified + Traditional Chinese and Japanese (kana+kanji),
// and the invisible layer must round-trip through pdftotext. (Korean hangul is NOT
// covered by this face — see deferred.md — and is intentionally absent here.)
func TestStampTextLayerCJK(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	samples := map[string]string{
		"chi_sim": "你好世界",       // Simplified Chinese
		"chi_tra": "繁體中文世界", // Traditional Chinese (繁/體 are traditional-only forms)
		"jpn":     "こんにちは世界", // Japanese: hiragana + kanji
	}
	for lang, sample := range samples {
		base, err := testpdf.Text("scan")
		if err != nil {
			t.Fatal(err)
		}
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{50, 100, 320, 116}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
			t.Errorf("%s: %q did not round-trip — extracted %q (font missing or lacks glyphs)", lang, sample, strings.TrimSpace(txt))
		}
	}
}

func TestOCRFontFor(t *testing.T) {
	cases := map[string]string{
		"eng": "Roboto-Regular",
		"rus": "Roboto-Regular",
		"ell": "Roboto-Regular",
		"tha": "NotoSansThai-Regular",
		"hin": "NotoSansDevanagari-Regular",
		"mar": "NotoSansDevanagari-Regular",
		"ara":     "NotoSansArabic-Regular",
		"heb":     "NotoSansHebrew-Regular",
		"chi_sim": "DroidSansFallback",
		"chi_tra": "DroidSansFallback",
		"jpn":     "DroidSansFallback",
		"":        "Roboto-Regular",
		"zzz":     "Roboto-Regular",
	}
	for lang, want := range cases {
		if got := ocrFontFor(lang); got != want {
			t.Errorf("ocrFontFor(%q) = %q, want %q", lang, got, want)
		}
	}
}
