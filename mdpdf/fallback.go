package mdpdf

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Font is an embeddable face for text the Base-14 core fonts cannot print.
//
// **Why the caller supplies these rather than mdpdf holding them.** This package sits at
// the repo root, not under internal/, precisely so other projects can import it through a
// local `replace` — and a package that reached into Nib's vendored font pool would drag
// Nib's OCR machinery along with it for anyone who did. Fonts are also large: the pan-CJK
// face alone is 4 MB, and a caller rendering only English should not carry it. So mdpdf
// states the interface and the caller decides what to spend.
//
// Covers is what makes selection possible without asking a TTF which glyphs it holds:
// pdfcpu installs a face by name and offers no glyph-coverage query, so the caller — who
// chose the font — declares the scripts it is for. Getting it wrong is visible rather than
// silent: an uncovered rune still reports through Unsupported.
type Font struct {
	// Name is the PostScript name pdfcpu registers the face under, which is what the
	// generated page spec names. It is NOT the file name — for the two faces where they
	// differ, the PostScript name is the one that works.
	Name string
	// Data is the TTF.
	Data []byte
	// Covers are the Unicode ranges this face is being supplied for, in the form
	// unicode.Thai, unicode.Han, unicode.Cyrillic and so on.
	Covers []*unicode.RangeTable
}

// covers reports whether f is offered for r.
func (f Font) covers(r rune) bool {
	for _, t := range f.Covers {
		if unicode.Is(t, r) {
			return true
		}
	}
	return false
}

// installFallbacks registers each face with pdfcpu so the page spec can name it.
//
// Idempotent by necessity rather than by taste: pdfcpu keeps user fonts in a shared
// on-disk directory, so two conversions running at once install the same face, and the
// loser of that race must not fail the render. Mirrors internal/pdfops.InstallOCRFonts,
// which learned the same thing.
func installFallbacks(fonts []Font) error {
	if len(fonts) == 0 {
		return nil
	}
	model.NewDefaultConfiguration() // sets font.UserFontDir
	for _, f := range fonts {
		if f.Name == "" || len(f.Data) == 0 {
			return fmt.Errorf("fallback font %q has no name or no data", f.Name)
		}
		if installedFallback(f.Name) {
			continue
		}
		if err := font.InstallFontFromBytes(font.UserFontDir, f.Name, f.Data); err != nil {
			// A concurrent installer of the same face is the expected cause. Re-check
			// rather than retrying blind: if it is there now, someone else finished it.
			if !installedFallback(f.Name) {
				return fmt.Errorf("install fallback font %s: %w", f.Name, err)
			}
		}
		if !installedFallback(f.Name) {
			// pdfcpu writes the .gob under the font's OWN PostScript name, read out of
			// the TTF, and ignores the name it was handed. So a Name that does not match
			// produces a successful install of a face nothing can then refer to — and
			// the next symptom is a panic from deep inside pdfcpu ("user font not
			// loaded"), several frames from the cause. Said plainly here instead.
			return fmt.Errorf("fallback font %q installed under a different name: pdfcpu names a face by the PostScript name inside the TTF, so Font.Name must be that name", f.Name)
		}
		if err := registerMetrics(f.Name); err != nil {
			return err
		}
	}
	return nil
}

// registerMetrics puts a just-installed face into pdfcpu's IN-MEMORY registry.
//
// **Installing is not enough, and the failure is a panic rather than an error.** pdfcpu
// fills that registry from the user-font directory exactly once per process, behind a
// sync.Once — so a face installed after any earlier font operation is on disk and unknown
// to the running program, and the first measurement of it panics with "user font not
// loaded". Nib's own InstallOCRFonts sidesteps this by running at startup before anything
// else touches a font; a library called at arbitrary times cannot.
//
// The registry and its lock are exported, and the .gob is a plain gob-encoded
// font.TTFLight, so the entry can be added directly. That is a narrower reach into pdfcpu
// than it looks: the alternative is requiring every caller to install fonts before the
// process performs its first font operation, which is a constraint a package cannot state
// and a caller cannot honour.
func registerMetrics(name string) error {
	font.UserFontMetricsLock.RLock()
	_, ok := font.UserFontMetrics[name]
	font.UserFontMetricsLock.RUnlock()
	if ok {
		return nil
	}
	f, err := os.Open(filepath.Join(font.UserFontDir, name+".gob"))
	if err != nil {
		return fmt.Errorf("read installed font %s: %w", name, err)
	}
	defer f.Close()
	var ttf font.TTFLight
	if err := gob.NewDecoder(f).Decode(&ttf); err != nil {
		return fmt.Errorf("decode installed font %s: %w", name, err)
	}
	font.UserFontMetricsLock.Lock()
	font.UserFontMetrics[name] = ttf
	font.UserFontMetricsLock.Unlock()
	return nil
}

// installedFallback reports whether pdfcpu already holds this face on disk. The .gob is
// named for the PostScript name, which is what Font.Name carries. Existence and
// non-emptiness rather than an integrity check: the .gob holds an unexported pdfcpu
// struct, so nothing outside that package can decode one to verify it.
func installedFallback(name string) bool {
	fi, err := os.Stat(filepath.Join(font.UserFontDir, name+".gob"))
	return err == nil && fi.Size() > 0
}

// pickFont returns the face to set s in: the active core font when it can print every
// rune, otherwise the first supplied fallback that covers the runes it cannot.
//
// **Per WORD, not per rune.** addText already splits on whitespace, so a word is the
// smallest unit the layout positions independently — and the fallback faces (Noto, Droid)
// carry Latin too, so a word mixing scripts sets correctly in the fallback. Splitting
// mid-word would mean measuring and positioning two fragments that must not be allowed to
// drift apart, for a gain no reader would see.
//
// Returns "" when nothing supplied can print the word. The caller keeps the core font in
// that case: the text still renders as spaces, which is what it did before any of this,
// and Unsupported still reports it. Substituting a face that cannot print it either would
// only move the blanks.
func pickFont(s string, core style, fonts []Font) string {
	var need []rune
	for _, r := range s {
		if !printable(r) {
			need = append(need, r)
		}
	}
	if len(need) == 0 {
		return core.font
	}
	for _, f := range fonts {
		ok := true
		for _, r := range need {
			if !f.covers(r) {
				ok = false
				break
			}
		}
		if ok {
			return f.Name
		}
	}
	return ""
}
