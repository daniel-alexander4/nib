package pdfops

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestMain installs the vendored non-Latin OCR fonts before any test runs — pdfcpu
// loads its font registry exactly once (sync.Once) on the first font op, so the
// Noto .gob files must be on disk before then, exactly as the server does at
// startup. Without this, whichever font test runs first would freeze a registry
// that lacks Thai/Devanagari.
//
// A failure here is FATAL, not logged. The comment it replaces said "the
// Thai/Devanagari test will fail loudly" — which is true only where poppler is
// installed, because every test that would notice is gated on
// exec.LookPath("pdftotext") and skips silently without it. On the fresh clone the
// build contract targets ("a fresh clone runs tiers 0 and 1 unaided"), a broken
// font install therefore produced a green package and a log line addressed to
// nobody. Exiting non-zero makes the failure visible independently of poppler.
func TestMain(m *testing.M) {
	// The unwritable-config child does its own install inside the test body, with pdfcpu's
	// config global still cold. Installing here would either warm that global — making the
	// child's subject unreachable, which is how the first version of that test went vacuous
	// — or exit 1 on the error, which the parent would then be unable to tell from a panic.
	if os.Getenv("NIB_OCRFONT_CHILD") != "" {
		os.Exit(m.Run())
	}
	if err := InstallOCRFonts(); err != nil {
		fmt.Fprintf(os.Stderr, "InstallOCRFonts: %v\n", err)
		os.Exit(1)
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
		"chi_sim": "你好世界",    // Simplified Chinese
		"chi_tra": "繁體中文世界",  // Traditional Chinese (繁/體 are traditional-only forms)
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

// TestStampTextLayerKorean is the coverage gate for Korean: the vendored NanumGothic
// glyf face, installed by its PostScript name, must carry the modern hangul
// syllables (which Droid Sans Fallback's cmap lacks) and round-trip through
// pdftotext. Korean is LTR, so a plain strings.Contains check applies. (Hanja —
// Chinese chars sometimes in formal Korean — are NOT covered by NanumGothic; see
// deferred.md/completed.md. Modern Korean is overwhelmingly hangul.)
func TestStampTextLayerKorean(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	sample := "안녕하세요세계" // "Hello, world" in hangul
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{50, 100, 320, 116}, Text: sample}}, "kor")
	if err != nil {
		t.Fatalf("kor: StampTextLayer: %v", err)
	}
	if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
		t.Errorf("kor: %q did not round-trip — extracted %q (font missing or lacks glyphs)", sample, strings.TrimSpace(txt))
	}
}

// TestStampTextLayerLatinBreadth is the coverage gate for the European-Latin
// languages added on top of Roboto: their distinctive diacritics (Polish, Czech,
// Hungarian, Romanian comma-accents, Turkish dotless-i, Vietnamese stacked
// diacritics, etc.) must round-trip through pdftotext via the bundled Roboto — no
// dedicated font. Catches a coverage regression if the bundled Roboto ever changes.
func TestStampTextLayerLatinBreadth(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	samples := map[string]string{
		"por": "coração não ação",
		"pol": "zażółć gęślą jaźń",
		"nld": "déjà café",
		"tur": "İstanbul güneş çığ",
		"vie": "Tiếng Việt Đà Nẵng",
		"ces": "příliš žluťoučký kůň",
		"hun": "árvíztűrő tükörfúrógép",
		"ron": "în România țară șters",
		"swe": "smörgås Åke",
	}
	for lang, sample := range samples {
		base, err := testpdf.Text("scan")
		if err != nil {
			t.Fatal(err)
		}
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{40, 100, 360, 116}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
			t.Errorf("%s: %q did not round-trip — extracted %q", lang, sample, strings.TrimSpace(txt))
		}
	}
}

// TestStampTextLayerScriptBreadth gates the languages that reuse an existing
// vendored non-Latin face: Marathi/Nepali/Sanskrit on Noto Devanagari (LTR), and
// Persian/Urdu on Noto Arabic (RTL → rune-multiset round-trip, since a bidi
// extractor reverses RTL on display).
func TestStampTextLayerScriptBreadth(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	ltr := map[string]string{
		"mar": "मराठी भाषा ळ",
		"nep": "नेपाली भाषा",
		"san": "संस्कृतम् ऋ",
	}
	rtl := map[string]string{
		"fas": "فارسی پژوهش گچ",
		"urd": "اردو زبان ٹھ ڈ",
	}
	for lang, sample := range ltr {
		base, _ := testpdf.Text("scan")
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{40, 100, 360, 116}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
			t.Errorf("%s: %q did not round-trip — extracted %q", lang, sample, strings.TrimSpace(txt))
		}
	}
	for lang, sample := range rtl {
		base, _ := testpdf.Text("scan")
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{40, 100, 360, 116}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		if miss := missingRunes(sample, pdfToText(t, out)); miss != "" {
			t.Errorf("%s: %q missing glyphs %q", lang, sample, miss)
		}
	}
}

// TestStampTextLayerBengali gates Bengali: the vendored Noto Sans Bengali glyf
// face, installed by PostScript name, must cover the Bengali block and round-trip
// through pdftotext — including a virama conjunct (ক্ত), proving the invisible
// layer is searchable regardless of visual shaping (the /ToUnicode carries logical
// order, as for Devanagari).
func TestStampTextLayerBengali(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	sample := "বাংলা ক্ত" // "Bangla" + a conjunct (ka + virama + ta)
	base, err := testpdf.Text("scan")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{40, 100, 360, 116}, Text: sample}}, "ben")
	if err != nil {
		t.Fatalf("ben: StampTextLayer: %v", err)
	}
	if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
		t.Errorf("ben: %q did not round-trip — extracted %q (font missing or lacks glyphs)", sample, strings.TrimSpace(txt))
	}
}

// TestStampTextLayerSouthAsian gates the six South-Asian scripts, each on its own
// vendored static Noto glyf face, installed by PostScript name. Samples include a
// virama conjunct where natural; like Bengali/Devanagari the invisible layer
// round-trips on logical-order /ToUnicode regardless of visual shaping.
func TestStampTextLayerSouthAsian(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) not installed")
	}
	samples := map[string]string{
		"tam": "தமிழ் மொழி",   // Tamil (with ழ் virama)
		"tel": "తెలుగు భాష",   // Telugu
		"kan": "ಕನ್ನಡ ಭಾಷೆ",   // Kannada (ನ್ನ conjunct)
		"mal": "മലയാളം ക്ത",   // Malayalam (ക്ത conjunct)
		"guj": "ગુજરાતી ભાષા", // Gujarati
		"pan": "ਪੰਜਾਬੀ ਭਾਸ਼ਾ", // Punjabi (Gurmukhi)
	}
	for lang, sample := range samples {
		base, err := testpdf.Text("scan")
		if err != nil {
			t.Fatal(err)
		}
		out, err := StampTextLayer(base, []Word{{Page: 1, Rect: [4]float64{40, 100, 360, 116}, Text: sample}}, lang)
		if err != nil {
			t.Errorf("%s: StampTextLayer: %v", lang, err)
			continue
		}
		if txt := pdfToText(t, out); !strings.Contains(txt, sample) {
			t.Errorf("%s [%s]: %q did not round-trip — extracted %q", lang, ocrFontFor(lang), sample, strings.TrimSpace(txt))
		}
	}
}

func TestOCRFontFor(t *testing.T) {
	cases := map[string]string{
		"eng":     "Roboto-Regular",
		"rus":     "Roboto-Regular",
		"ell":     "Roboto-Regular",
		"tha":     "NotoSansThai-Regular",
		"hin":     "NotoSansDevanagari-Regular",
		"mar":     "NotoSansDevanagari-Regular",
		"ben":     "NotoSansBengali-Regular",
		"tam":     "NotoSansTamil-Regular",
		"tel":     "NotoSansTelugu-Regular",
		"kan":     "NotoSansKannada-Regular",
		"mal":     "NotoSansMalayalam-Regular",
		"guj":     "NotoSansGujarati-Regular",
		"pan":     "NotoSansGurmukhi-Regular",
		"ara":     "NotoSansArabic-Regular",
		"heb":     "NotoSansHebrew-Regular",
		"chi_sim": "DroidSansFallback",
		"chi_tra": "DroidSansFallback",
		"jpn":     "DroidSansFallback",
		"kor":     "NanumGothic",
		"":        "Roboto-Regular",
		"zzz":     "Roboto-Regular",
	}
	for lang, want := range cases {
		if got := ocrFontFor(lang); got != want {
			t.Errorf("ocrFontFor(%q) = %q, want %q", lang, got, want)
		}
	}
}

// TestAnUnwritableConfigDirDoesNotCrashStartup — the panic that walked past a caller which
// only handles errors.
//
// `model.NewDefaultConfiguration()` calls `fault.Fail` when the config directory cannot be
// created, and `fault.Fail` PANICS. `server.New` calls InstallOCRFonts inside
// `if err := …; err != nil { log; carry on }`, and a panic goes straight through that — so a
// read-only or unwritable $HOME crashed Nib at startup instead of costing only Thai and
// Devanagari OCR. The project's self-healing rule (STANDARDS §9, adopted by D34) says the
// opposite: unusable state degrades, it never blocks startup.
//
// **It runs in a SUBPROCESS, and the first version did not — which made it vacuous.** pdfcpu
// caches its configuration in a package global, and this package's own TestMain calls
// InstallOCRFonts before any test runs, so in-process `NewDefaultConfiguration` returns the
// cached value and never touches the filesystem. Removing the `fault.Catch` left that version
// green. A child process starts with the global cold, and its own TestMain is then the thing
// under test: without the catch the child dies on a panic, with it the child reports an error
// and runs.
func TestAnUnwritableConfigDirDoesNotCrashStartup(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions, so an unwritable parent is not unwritable")
	}
	if os.Getenv("NIB_OCRFONT_CHILD") != "" {
		// The child, with pdfcpu's config global cold. It does the call itself and reports
		// which of the two outcomes it got; the parent only cares that it is not a panic.
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("CHILD-PANIC %v\n", r)
				}
			}()
			if err := InstallOCRFonts(); err != nil {
				fmt.Printf("CHILD-ERROR %v\n", err)
				return
			}
			fmt.Println("CHILD-OK")
		}()
		return
	}

	parent := t.TempDir()
	locked := filepath.Join(parent, "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	// SETUP: the write bit is really honoured here. On a filesystem that ignores it the
	// failure cannot be provoked, and this must say so rather than pass.
	if err := os.Mkdir(filepath.Join(locked, "probe"), 0o755); err == nil {
		t.Skip("this filesystem ignores the write bit, so the failure cannot be provoked")
	}

	cmd := exec.Command(os.Args[0], "-test.run", "TestAnUnwritableConfigDirDoesNotCrashStartup")
	cmd.Env = append(os.Environ(),
		"NIB_OCRFONT_CHILD=1",
		"XDG_CONFIG_HOME="+filepath.Join(locked, "cannot-create"),
		"HOME="+filepath.Join(locked, "cannot-create"),
	)
	out, _ := cmd.CombinedOutput()

	// SETUP: the child really reached the call. Without this, a child that failed to build,
	// failed to start, or skipped would report no panic and read as a pass.
	if !bytes.Contains(out, []byte("CHILD-")) {
		t.Fatalf("the child never reached InstallOCRFonts, so nothing was exercised:\n%s", out)
	}
	if bytes.Contains(out, []byte("CHILD-PANIC")) || bytes.Contains(out, []byte("\npanic:")) {
		t.Fatalf("InstallOCRFonts PANICKED on an unwritable config directory rather than "+
			"returning an error. server.New handles an error and logs it; a panic walks "+
			"straight past that, so this is Nib crashing at startup on any machine whose "+
			"config directory cannot be written — against the self-healing rule that says "+
			"unusable state degrades and never blocks startup.\n%s", out)
	}
	// CHILD-ERROR is the correct outcome. CHILD-OK would mean the platform allowed the
	// write after all, which the setup probe above tries to rule out.
	if !bytes.Contains(out, []byte("CHILD-ERROR")) {
		t.Logf("child did not hit the unwritable path (%s) — the failure could not be "+
			"provoked here", bytes.TrimSpace(out))
	}
}
