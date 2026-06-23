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

func TestOCRFontFor(t *testing.T) {
	cases := map[string]string{
		"eng": "Roboto-Regular",
		"rus": "Roboto-Regular",
		"ell": "Roboto-Regular",
		"tha": "NotoSansThai-Regular",
		"hin": "NotoSansDevanagari-Regular",
		"mar": "NotoSansDevanagari-Regular",
		"":    "Roboto-Regular",
		"zzz": "Roboto-Regular",
	}
	for lang, want := range cases {
		if got := ocrFontFor(lang); got != want {
			t.Errorf("ocrFontFor(%q) = %q, want %q", lang, got, want)
		}
	}
}
