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

//go:embed fonts/NotoSansThai-Regular.ttf fonts/NotoSansDevanagari-Regular.ttf
var ocrFontFS embed.FS

// ocrFontFiles maps each font's PostScript name (the name pdfcpu registers it
// under, and the name a watermark references) to its embedded TTF.
var ocrFontFiles = map[string]string{
	"NotoSansThai-Regular":       "fonts/NotoSansThai-Regular.ttf",
	"NotoSansDevanagari-Regular": "fonts/NotoSansDevanagari-Regular.ttf",
}

// ocrFontFor returns the font a given OCR language must be stamped in. Latin,
// Cyrillic and Greek languages fall through to Roboto (pdfcpu's bundled default);
// Thai and Devanagari scripts use their vendored Noto face.
func ocrFontFor(lang string) string {
	switch lang {
	case "tha":
		return "NotoSansThai-Regular"
	case "hin", "mar", "nep", "san": // Devanagari-script languages
		return "NotoSansDevanagari-Regular"
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
