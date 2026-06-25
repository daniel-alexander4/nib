package pdfops

import (
	"embed"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Non-Latin OCR fonts. The OCR text layer is stamped with a real CID font so the
// invisible run extracts via /ToUnicode (see ocr.go). pdfcpu's bundled Roboto
// covers Latin, Cyrillic and Greek, but not Thai or Devanagari — those scripts
// need their own embedded font, installed into pdfcpu's user-font dir. (This is
// the only font Nib vendors itself; Roboto comes from inside pdfcpu.)

//go:embed fonts/NotoSansThai-Regular.ttf fonts/NotoSansDevanagari-Regular.ttf fonts/NotoSansArabic-Regular.ttf fonts/NotoSansHebrew-Regular.ttf fonts/NotoSansBengali-Regular.ttf fonts/DroidSansFallbackFull.ttf fonts/NanumGothic-Regular.ttf
var ocrFontFS embed.FS

// ocrFontFiles maps each font's PostScript name (the name pdfcpu registers it
// under, and the name a watermark references) to its embedded TTF.
var ocrFontFiles = map[string]string{
	"NotoSansThai-Regular":       "fonts/NotoSansThai-Regular.ttf",
	"NotoSansDevanagari-Regular": "fonts/NotoSansDevanagari-Regular.ttf",
	"NotoSansArabic-Regular":     "fonts/NotoSansArabic-Regular.ttf",
	"NotoSansHebrew-Regular":     "fonts/NotoSansHebrew-Regular.ttf",
	"NotoSansBengali-Regular":    "fonts/NotoSansBengali-Regular.ttf",
	// One pan-CJK face covers Simplified/Traditional Chinese and Japanese; its
	// embedded PostScript name is "DroidSansFallback" (the .ttf file is *Full*).
	"DroidSansFallback": "fonts/DroidSansFallbackFull.ttf",
	// Korean hangul: Droid's hangul coverage is incomplete, so Korean gets its own
	// glyf face (all 11,172 modern syllables). PostScript name "NanumGothic".
	"NanumGothic": "fonts/NanumGothic-Regular.ttf",
}

// ocrLangBCP47 maps Nib's OCR language codes (ISO 639-2/3, as tesseract uses) to
// the BCP 47 tags PDF's catalog /Lang wants, so an OCR'd scan can declare its
// language for assistive technology. A code not listed here yields "" (no /Lang).
var ocrLangBCP47 = map[string]string{
	"eng": "en", "fra": "fr", "deu": "de", "spa": "es", "ita": "it",
	"rus": "ru", "ukr": "uk", "bul": "bg", "srp": "sr", "mkd": "mk",
	"bel": "be", "ell": "el", "tha": "th", "hin": "hi",
	"ara": "ar", "heb": "he",
	"ces": "cs", "nld": "nl", "hun": "hu", "pol": "pl", "por": "pt",
	"ron": "ro", "swe": "sv", "tur": "tr", "vie": "vi",
	"mar": "mr", "nep": "ne", "san": "sa", "fas": "fa", "urd": "ur", "ben": "bn",
	"chi_sim": "zh-Hans", "chi_tra": "zh-Hant", "jpn": "ja", "kor": "ko",
}

// OCRLangToBCP47 returns the BCP 47 language tag for an OCR language code (for
// SetLang), or "" if the code isn't recognized.
func OCRLangToBCP47(lang string) string { return ocrLangBCP47[lang] }

// ocrFontFor returns the font a given OCR language must be stamped in. Latin,
// Cyrillic and Greek languages fall through to Roboto (pdfcpu's bundled default);
// Thai, Devanagari, Arabic and Hebrew scripts use their vendored Noto face. The
// stamped text layer is invisible (render mode 3) and written in logical order
// with a correct /ToUnicode, so RTL scripts stay searchable — a bidi-reordering
// extractor (e.g. poppler) only reverses them on *display*, not in the bytes.
func ocrFontFor(lang string) string {
	switch lang {
	case "tha":
		return "NotoSansThai-Regular"
	case "hin", "mar", "nep", "san": // Devanagari-script languages
		return "NotoSansDevanagari-Regular"
	case "ben": // Bengali script
		return "NotoSansBengali-Regular"
	case "ara", "fas", "urd": // Arabic-script languages
		return "NotoSansArabic-Regular"
	case "heb":
		return "NotoSansHebrew-Regular"
	case "chi_sim", "chi_tra", "jpn": // CJK — one Droid pan-CJK face covers all three
		return "DroidSansFallback"
	case "kor": // Korean hangul — its own glyf face (Droid's hangul is incomplete)
		return "NanumGothic"
	default:
		return ocrFont // Roboto-Regular — Latin, Cyrillic, Greek
	}
}

// InstallOCRFonts writes the vendored non-Latin OCR fonts into pdfcpu's user-font
// dir so StampTextLayer can stamp Thai/Devanagari. pdfcpu loads its in-memory font
// registry exactly once (sync.Once) on the first font operation, so the .gob files
// must be on disk before then: call this once at startup, before serving any
// request. font.InstallFontFromBytes only writes the font (it does not trigger the
// load), so installing here and letting the first op lazy-load picks them all up.
func InstallOCRFonts() error {
	model.NewDefaultConfiguration() // sets font.UserFontDir (+ installs Roboto if absent)
	for name, path := range ocrFontFiles {
		bb, err := ocrFontFS.ReadFile(path)
		if err != nil {
			return err
		}
		if err := font.InstallFontFromBytes(font.UserFontDir, name, bb); err != nil {
			return fmt.Errorf("install OCR font %s: %w", name, err)
		}
	}
	return nil
}
