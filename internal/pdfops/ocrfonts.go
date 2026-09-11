package pdfops

import (
	"embed"
	"fmt"
	"log"
	"nib/mdpdf"
	"os"
	"path/filepath"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Non-Latin OCR fonts. The OCR text layer is stamped with a real CID font so the
// invisible run extracts via /ToUnicode (see ocr.go). pdfcpu's bundled Roboto
// covers Latin, Cyrillic and Greek, but not Thai or Devanagari — those scripts
// need their own embedded font, installed into pdfcpu's user-font dir. (This is
// the only font Nib vendors itself; Roboto comes from inside pdfcpu.)

//go:embed fonts/NotoSansThai-Regular.ttf fonts/NotoSansDevanagari-Regular.ttf fonts/NotoSansArabic-Regular.ttf fonts/NotoSansHebrew-Regular.ttf fonts/NotoSansBengali-Regular.ttf fonts/NotoSansTamil-Regular.ttf fonts/NotoSansTelugu-Regular.ttf fonts/NotoSansKannada-Regular.ttf fonts/NotoSansMalayalam-Regular.ttf fonts/NotoSansGujarati-Regular.ttf fonts/NotoSansGurmukhi-Regular.ttf fonts/DroidSansFallbackFull.ttf fonts/NanumGothic-Regular.ttf fonts/Roboto-Regular.ttf fonts/Roboto-Bold.ttf fonts/Roboto-Italic.ttf fonts/Roboto-BoldItalic.ttf fonts/LiberationMono-Regular.ttf
var ocrFontFS embed.FS

// The faces nib AUTHORS text in — `PLAN-accessibility.md` P04.S01.
//
// # Why these are vendored when a Base-14 face costs nothing
//
// PDF/UA rule 7.21.4.1 requires the font program of every font used for rendering to be
// embedded in the file. A Base-14 core font cannot be: it is a name the reader supplies. So
// `mdpdf`'s five core faces are exactly why nib's own Markdown output fails that rule, and the
// only way past it is to draw in faces whose bytes ship with the document.
//
// # Why four are vendored and not one
//
// **pdfcpu bundles exactly one Latin face**, `Roboto-Regular` — measured: `font.IsUserFont` is
// false for `Roboto-Bold`, `Roboto-Italic`, `Roboto-BoldItalic` and any mono face. A document
// that embeds its body face and draws headings in core Helvetica-Bold still fails the rule, so
// the set is all-or-nothing.
//
// `Roboto-Regular` is vendored too, even though pdfcpu installs its own copy, so that all five
// faces come from ONE place and succeed or fail together. Without that, an install failure
// leaves a document set in a vendored bold and pdfcpu's regular, or the reverse.
//
// # Why the monospace face is not a Roboto
//
// `RobotoMono` is not bundled and Google's Roboto repository does not carry it; Liberation Mono
// is OFL, already a licence class this repo ships, and metrically a Courier New substitute —
// which matters not at all for layout, because `mdpdf` measures an embedded face by RUNE
// (`style.width`), but does mean code blocks look like what they replaced.
var authoringFontFiles = map[string]string{
	"Roboto-Regular":    "fonts/Roboto-Regular.ttf",
	"Roboto-Bold":       "fonts/Roboto-Bold.ttf",
	"Roboto-Italic":     "fonts/Roboto-Italic.ttf",
	"Roboto-BoldItalic": "fonts/Roboto-BoldItalic.ttf",
	// PostScript name "LiberationMono"; the FILE is *-Regular.ttf. Same split as
	// DroidSansFallback above, and mdpdf refuses a mismatch rather than installing under a
	// name nothing can reference — which is how this was found.
	"LiberationMono": "fonts/LiberationMono-Regular.ttf",
}

// authoringFaces is the base face set handed to `mdpdf`, or nil when any face is unreadable.
//
// **Nil is a degrade, not an error, and `mdpdf` treats it as one**: a document set in core
// fonts fails one PDF/UA rule, while refusing to convert fails the thing the user asked for.
// The bytes are embedded in the binary, so in a built nib this cannot fail — the nil path
// exists for the case that can, which is P04.S03's: an install into an unwritable font
// directory, handled inside `mdpdf`.
func authoringFaces() *mdpdf.Faces {
	read := func(name string) mdpdf.Font {
		bb, err := ocrFontFS.ReadFile(authoringFontFiles[name])
		if err != nil {
			return mdpdf.Font{}
		}
		return mdpdf.Font{Name: name, Data: bb}
	}
	f := &mdpdf.Faces{
		Body:       read("Roboto-Regular"),
		Bold:       read("Roboto-Bold"),
		Italic:     read("Roboto-Italic"),
		BoldItalic: read("Roboto-BoldItalic"),
		Code:       read("LiberationMono"),
	}
	return f
}

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
// It WAS reproduced by `go test ./...`, where internal/pdfops and internal/server ran in
// parallel and both called this: TestStampTextLayerCJK failed on chi_tra and jpn while
// passing whenever the package was run alone. That intermittency is what made the
// symptom look environmental — it was filed as a suspected truncated write from a
// full disk, and it is really two writers and a non-atomic library.
//
// **That reproduction no longer reproduces, and the sentence is corrected rather than
// left standing.** P05.S01 gave internal/server its own pinned XDG_CONFIG_HOME, so the two
// packages no longer share a font directory and the suite cannot collide with itself. The
// production race below is what this code is still for.
//
// Two nib processes starting together hit the same race in production, which is why
// this is not a test-only fix. A cold start where both find the directory empty can
// still collide, so an install that fails is retried once before it is reported: the
// second attempt finds the other process's completed file and skips it.
func InstallOCRFonts() (err error) {
	// **pdfcpu PANICS here, and the caller only handles an error.**
	//
	// `NewDefaultConfiguration` calls `fault.Fail` when the config directory cannot be
	// created (model/configuration.go:558), and `fault.Fail` panics. `server.New` calls this
	// inside `if err := …; err != nil { log; carry on }` — which a panic walks straight past,
	// so a read-only or unwritable $HOME crashed Nib at STARTUP instead of costing only
	// Thai/Devanagari OCR. That is the opposite of the self-healing rule the project takes
	// from STANDARDS §9 and D34 adopts: corrupt or unusable state degrades, it never blocks
	// startup.
	//
	// `fault.Catch` is pdfcpu's own mechanism for this and re-panics on anything that is not
	// its own Panic type — so a genuine bug still crashes loudly rather than being swallowed
	// by a hand-rolled recover.
	defer fault.Catch(&err)
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

// AuthoredTextFaces names the faces nib draws its OWN prose in — the co-signing readme and the
// signature pages — and says whether they are embedded, so the caller measures with the matching
// rule (`mdpdf.Width`).
//
// **It installs on the way past and degrades**, which is the whole reason it is a function and not
// a pair of constants. The faces come from an install that can fail on the user's machine (P04.S03),
// and a readme set in Helvetica is a worse PDF than one set in Roboto, while a readme that fails to
// render stops a co-signing ceremony. The Base-14 names are the fallback and they always work.
//
// Callers must use the returned `embedded` for BOTH drawing and measuring. The readme's own comment
// records why: the font it is rendered in and the font its wrap is computed against were once two
// independent literals, which is a wrap for one font and a page drawn in another the moment either
// moves.
func AuthoredTextFaces() (body, bold string, embedded bool) {
	faces := authoringFaces()
	if err := mdpdf.InstallFaces(faces); err != nil {
		log.Printf("authored text: the embedded faces are unavailable, so nib's own pages are set "+
			"in Base-14 core fonts and will not embed them: %v", err)
		return "Helvetica", "Helvetica-Bold", false
	}
	return faces.Body.Name, faces.Bold.Name, true
}
