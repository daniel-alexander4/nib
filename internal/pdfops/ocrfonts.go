package pdfops

import (
	"embed"
	"fmt"
	"nib/mdpdf"
	"os"
	"path/filepath"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Non-Latin OCR fonts. The OCR text layer is stamped with a real CID font so the
// invisible run extracts via /ToUnicode (see ocr.go). pdfcpu's bundled Roboto
// covers Latin, Cyrillic and Greek, but not Thai or Devanagari — those scripts
// need their own embedded font, installed into pdfcpu's user-font dir. (This is
// the only font Nib vendors itself; Roboto comes from inside pdfcpu.)

//go:embed fonts/NotoSansThai-Regular.ttf fonts/NotoSansDevanagari-Regular.ttf fonts/NotoSansArabic-Regular.ttf fonts/NotoSansHebrew-Regular.ttf fonts/NotoSansBengali-Regular.ttf fonts/NotoSansTamil-Regular.ttf fonts/NotoSansTelugu-Regular.ttf fonts/NotoSansKannada-Regular.ttf fonts/NotoSansMalayalam-Regular.ttf fonts/NotoSansGujarati-Regular.ttf fonts/NotoSansGurmukhi-Regular.ttf fonts/DroidSansFallbackFull.ttf fonts/NanumGothic-Regular.ttf
var ocrFontFS embed.FS

// ocrFontFiles maps each font's PostScript name (the name pdfcpu registers it
// under, and the name a watermark references) to its embedded TTF.
var ocrFontFiles = map[string]string{
	"NotoSansThai-Regular":       "fonts/NotoSansThai-Regular.ttf",
	"NotoSansDevanagari-Regular": "fonts/NotoSansDevanagari-Regular.ttf",
	"NotoSansArabic-Regular":     "fonts/NotoSansArabic-Regular.ttf",
	"NotoSansHebrew-Regular":     "fonts/NotoSansHebrew-Regular.ttf",
	"NotoSansBengali-Regular":    "fonts/NotoSansBengali-Regular.ttf",
	"NotoSansTamil-Regular":      "fonts/NotoSansTamil-Regular.ttf",
	"NotoSansTelugu-Regular":     "fonts/NotoSansTelugu-Regular.ttf",
	"NotoSansKannada-Regular":    "fonts/NotoSansKannada-Regular.ttf",
	"NotoSansMalayalam-Regular":  "fonts/NotoSansMalayalam-Regular.ttf",
	"NotoSansGujarati-Regular":   "fonts/NotoSansGujarati-Regular.ttf",
	"NotoSansGurmukhi-Regular":   "fonts/NotoSansGurmukhi-Regular.ttf",
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
	"tam": "ta", "tel": "te", "kan": "kn", "mal": "ml", "guj": "gu", "pan": "pa",
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
	case "tam": // Tamil
		return "NotoSansTamil-Regular"
	case "tel": // Telugu
		return "NotoSansTelugu-Regular"
	case "kan": // Kannada
		return "NotoSansKannada-Regular"
	case "mal": // Malayalam
		return "NotoSansMalayalam-Regular"
	case "guj": // Gujarati
		return "NotoSansGujarati-Regular"
	case "pan": // Punjabi (Gurmukhi script)
		return "NotoSansGurmukhi-Regular"
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

// installedOCRFont reports whether pdfcpu already holds this face on disk.
//
// The .gob is named for the font's PostScript name, which is exactly what the keys
// of ocrFontFiles are (see the DroidSansFallback and NanumGothic notes there, where
// the PostScript name and the .ttf filename deliberately differ).
//
// Existence and non-emptiness, not a full integrity check: the .gob holds an
// unexported pdfcpu struct, so nothing outside that package can decode one to
// verify it. The trade this makes is worth stating — rewriting unconditionally, as
// this used to, meant a .gob left truncated by a crash was repaired on the next
// start, and now it is not. That is acceptable because the unconditional rewrite was
// itself the main way such a file came to exist: it kept a non-atomic write open on
// every startup. Removing the cause beats repairing the effect. A .gob that is
// corrupt anyway is repaired by deleting it (or the whole font dir) — pdfcpu and
// this function then reinstall from the embedded originals.
func installedOCRFont(name string) bool {
	fi, err := os.Stat(filepath.Join(font.UserFontDir, name+".gob"))
	return err == nil && fi.Size() > 0
}

// InstallOCRFonts writes the vendored non-Latin OCR fonts into pdfcpu's user-font
// dir so StampTextLayer can stamp Thai/Devanagari. pdfcpu loads its in-memory font
// registry exactly once (sync.Once) on the first font operation, so the .gob files
// must be on disk before then: call this once at startup, before serving any
// request. font.InstallFontFromBytes only writes the font (it does not trigger the
// load), so installing here and letting the first op lazy-load picks them all up.
// It installs only what is MISSING, and that is a correctness requirement rather
// than an optimization. pdfcpu's installer writes each .gob directly to its final
// path and then re-reads it to verify (font/install.go, writeGob then readGob) —
// there is no temp-and-rename — so while one process is rewriting a font, any other
// process reading that directory sees a truncated file. Because the font dir is
// shared (~/.config/pdfcpu/fonts) and the registry load is lazy and once-only, the
// reader gets "failed to load user fonts: unexpected EOF" and loses EVERY non-Latin
// face at once, not just the one being written.
//
// Rewriting all thirteen fonts on every startup held that window open on every run.
// Reproduced by `go test ./...`, where internal/pdfops and internal/server run in
// parallel and both call this: TestStampTextLayerCJK failed on chi_tra and jpn while
// passing whenever the package was run alone. That intermittency is what made the
// symptom look environmental — it was filed as a suspected truncated write from a
// full disk, and it is really two writers and a non-atomic library.
//
// Two nib processes starting together hit the same race in production, which is why
// this is not a test-only fix. A cold start where both find the directory empty can
// still collide, so an install that fails is retried once before it is reported: the
// second attempt finds the other process's completed file and skips it.
func InstallOCRFonts() error {
	model.NewDefaultConfiguration() // sets font.UserFontDir (+ installs Roboto if absent)
	for name, path := range ocrFontFiles {
		if installedOCRFont(name) {
			continue
		}
		bb, err := ocrFontFS.ReadFile(path)
		if err != nil {
			return err
		}
		if err := font.InstallFontFromBytes(font.UserFontDir, name, bb); err != nil {
			// A concurrent installer of the same font is the expected cause. Re-check
			// rather than retrying blind: if the file is there now, someone else
			// finished it and this is a success, not a failure to paper over.
			if installedOCRFont(name) {
				continue
			}
			if err2 := font.InstallFontFromBytes(font.UserFontDir, name, bb); err2 != nil {
				return fmt.Errorf("install OCR font %s: %w", name, err2)
			}
		}
	}
	return nil
}

// markdownFallbackFonts offers the vendored faces to mdpdf, in the order they should be
// tried for text the Base-14 core fonts cannot print.
//
// **Order is the whole of the policy.** A word is set in the FIRST face whose declared
// ranges cover its non-WinAnsi runes, so the script-specific faces come before the pan-CJK
// one: DroidSansFallback carries a great deal besides Han, and putting it first would win
// words that a purpose-built Noto face sets better. Roboto is not offered — pdfcpu falls
// back to it for Latin/Cyrillic/Greek on its own, and naming it here would have mdpdf
// embed a face for text the core fonts already print.
//
// The bytes come from the same embedded FS the OCR text layer uses, so nothing new is
// vendored and nothing is read from disk.
func markdownFallbackFonts() []mdpdf.Font {
	specs := []struct {
		name   string
		ranges []*unicode.RangeTable
	}{
		{"NotoSansThai-Regular", []*unicode.RangeTable{unicode.Thai}},
		{"NotoSansDevanagari-Regular", []*unicode.RangeTable{unicode.Devanagari}},
		{"NotoSansBengali-Regular", []*unicode.RangeTable{unicode.Bengali}},
		{"NotoSansTamil-Regular", []*unicode.RangeTable{unicode.Tamil}},
		{"NotoSansTelugu-Regular", []*unicode.RangeTable{unicode.Telugu}},
		{"NotoSansKannada-Regular", []*unicode.RangeTable{unicode.Kannada}},
		{"NotoSansMalayalam-Regular", []*unicode.RangeTable{unicode.Malayalam}},
		{"NotoSansGujarati-Regular", []*unicode.RangeTable{unicode.Gujarati}},
		{"NotoSansGurmukhi-Regular", []*unicode.RangeTable{unicode.Gurmukhi}},
		{"NotoSansArabic-Regular", []*unicode.RangeTable{unicode.Arabic}},
		{"NotoSansHebrew-Regular", []*unicode.RangeTable{unicode.Hebrew}},
		{"NanumGothic", []*unicode.RangeTable{unicode.Hangul}},
		// Last: broad coverage, so it catches Han, Hiragana, Katakana and anything the
		// faces above did not claim.
		{"DroidSansFallback", []*unicode.RangeTable{
			unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Bopomofo,
		}},
	}
	out := make([]mdpdf.Font, 0, len(specs))
	for _, sp := range specs {
		path, ok := ocrFontFiles[sp.name]
		if !ok {
			continue // a face renamed in ocrFontFiles: skip rather than ship an empty Font
		}
		bb, err := ocrFontFS.ReadFile(path)
		if err != nil {
			continue // embedded, so this cannot fail in a built binary
		}
		out = append(out, mdpdf.Font{Name: sp.name, Data: bb, Covers: sp.ranges})
	}
	return out
}
