package pdfops

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nib/mdpdf"
)

// Office-document → PDF conversion: shell out to an installed LibreOffice in
// headless mode. Pure Go can't render office layout (the declined path B — a Go
// office library — trades fidelity and licensing for that), so this is path A: an
// optional external runtime dependency, detected at runtime and never bundled, so
// the binary stays cgo-free. Opt-in by design — same posture as the Ghostscript
// PDF/A path.
//
// SECURITY: LibreOffice *interprets* the input document (a large, historically
// CVE-prone surface; office files can also carry macros). Mitigations: it's opt-in
// (the user installed LibreOffice), each conversion runs in its own temp directory
// with an isolated user profile (no access to the user's real LibreOffice config),
// under a timeout, with no shell (args as a slice). Headless LibreOffice does not
// auto-run macros at the default macro-security level. Nib is loopback/single-user,
// so the residual risk is the same class as opening the document in LibreOffice
// directly. Fidelity is LibreOffice's: complex documents may convert imperfectly.

// ErrLibreOfficeMissing is returned when no LibreOffice binary is on PATH, so the
// caller can degrade gracefully (CLI hint, hidden GUI menu item) rather than fail
// obscurely.
var ErrLibreOfficeMissing = errors.New("LibreOffice (soffice) is not installed")

// ErrUnsupportedOffice is returned for an input whose extension isn't in the
// allowlist, so we never hand an arbitrary file to LibreOffice.
var ErrUnsupportedOffice = errors.New("unsupported office document type")

const officeConvertTimeout = 2 * time.Minute

// OfficeExtensions is the set of input formats offered for conversion (lowercase,
// no leading dot). Kept to the common word-processor / spreadsheet / presentation
// formats LibreOffice handles well.
var OfficeExtensions = []string{
	"doc", "docx", "odt", "rtf", "txt",
	"xls", "xlsx", "ods", "csv",
	"ppt", "pptx", "odp",
}

// SupportedOfficeExt reports whether ext (any case, with or without a leading dot)
// is an office format Nib will convert.
func SupportedOfficeExt(ext string) bool {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	for _, e := range OfficeExtensions {
		if e == ext {
			return true
		}
	}
	return false
}

// SupportedMarkdownExt reports whether ext (any case, with or without a leading
// dot) is Markdown, which converts natively — no LibreOffice needed.
func SupportedMarkdownExt(ext string) bool {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	return ext == "md" || ext == "markdown"
}

// UnprintableMarkdown reports the runes in a Markdown source that the conversion CANNOT
// print — neither the Base-14 core fonts nor any face Nib supplies covers them.
//
// It exists because mdpdf.Unsupported had no caller outside its own tests, and its whole
// purpose is to be called: pdfcpu maps a rune it cannot encode to a SPACE rather than
// erroring, so a document silently loses characters and there is nothing to catch
// afterwards. The guard has to run BEFORE the PDF becomes a record, and only a caller can
// decide what to do about it — which is why this reports rather than refuses.
//
// Asked with Nib's own font pool, so it answers the question that matters here ("will this
// print in Nib?") rather than the narrower one mdpdf.Unsupported answers on its own
// ("would this print with no fonts supplied?"). Since v1.109.0 those differ for most of the
// world's scripts.
func UnprintableMarkdown(data []byte) []rune {
	return mdpdf.UnsupportedWith(string(data), markdownFallbackFonts())
}

// SupportedDocExt reports whether ext converts to PDF via ConvertDocToPDF,
// natively (Markdown) or through LibreOffice (office formats).
func SupportedDocExt(ext string) bool {
	return SupportedMarkdownExt(ext) || SupportedOfficeExt(ext)
}

// ConvertDocToPDF converts a non-PDF document to PDF. Markdown renders in pure
// Go and is always available; everything else goes through LibreOffice.
func ConvertDocToPDF(data []byte, ext string) ([]byte, error) {
	if SupportedMarkdownExt(ext) {
		// With Nib's vendored faces, so a Markdown file containing Cyrillic, Greek, CJK,
		// Arabic or an Indic script prints instead of rendering as spaces. mdpdf itself
		// holds no fonts on purpose — it lives at the repo root so other projects can
		// import it, and a package that reached in here would drag Nib's OCR machinery
		// along with it. Nib is the caller that already has the fonts, so Nib supplies
		// them.
		faces := authoringFaces()
		// **A degrade must not be silent.** mdpdf falls back to the Base-14 core fonts when the
		// faces cannot be installed — a read-only or full user-font directory — and a document in
		// core fonts looks entirely correct while failing PDF/UA 7.21.4.1. mdpdf has no logger by
		// design, so the notice is raised here, where there is one. Asking twice costs a map
		// lookup per face: installing an already-installed face short-circuits.
		if ferr := mdpdf.InstallFaces(faces); ferr != nil {
			log.Printf("markdown: the authoring faces could not be installed, so this document is "+
				"set in Base-14 core fonts and will not embed them: %v", ferr)
		}
		// **Tagged from the document's own structure — `/pending 481`.** P06.S02 built
		// `tagMarkdown` and P06 closed on it, and until this line nothing a user could reach ever
		// called it: this door went straight to `mdpdf.ConvertWithFaces`, so every converted
		// Markdown file was untagged while the phase's criterion was met by a test. The
		// zero-caller scan could not see it, because `tagMarkdown` is unexported.
		//
		// `tagMarkdown` refuses rather than tag content by a position it cannot trust (the
		// run count and the structure disagree). **A refusal must not cost the user the
		// conversion**, so it falls back to the untagged render and says so — an untagged PDF is
		// what this door always returned, and no PDF is worse than both.
		fallbacks := markdownFallbackFonts()
		tagged, terr := tagMarkdown(data, faces, fallbacks)
		if terr == nil {
			return tagged, nil
		}
		log.Printf("markdown: the document converted but could not be tagged, so it is returned "+
			"without structure: %v", terr)
		out, err := mdpdf.ConvertWithFaces(data, faces, fallbacks)
		if err != nil {
			return nil, err
		}
		// Nib put those faces in the document, so nib owns what it claims about them: pdfcpu
		// writes a /CIDSet over the USED glyphs and PDF/UA 7.21.4.2 forbids one that does not
		// cover the program. See dropCIDSets. (`tagMarkdown` runs the same tail itself.)
		return embeddedFontsAreHonest(out), nil
	}
	return ConvertOfficeToPDF(data, ext)
}

var loPath struct {
	sync.Once
	p string
}

// libreOfficePath returns the LibreOffice executable path (cached), or "" if none
// is installed. The binary is "soffice" (also symlinked "libreoffice") on Unix and
// "soffice.exe" on Windows, all of which exec.LookPath resolves.
func libreOfficePath() string {
	loPath.Do(func() {
		for _, name := range []string{"soffice", "libreoffice"} {
			if p, err := exec.LookPath(name); err == nil {
				loPath.p = p
				return
			}
		}
	})
	return loPath.p
}

// LibreOfficeAvailable reports whether LibreOffice is installed, so a caller can
// offer office conversion (and the UI can show/hide it).
func LibreOfficeAvailable() bool { return libreOfficePath() != "" }

// ConvertOfficeToPDF converts an office document (DOCX/XLSX/ODT/… — ext is the
// source extension, used to pick LibreOffice's input filter) to PDF via an
// installed LibreOffice. It writes the document into a fresh temp dir and runs
// soffice headless with a per-conversion user profile (so concurrent conversions
// don't clash on the single-instance profile), then reads the produced PDF.
// Returns ErrLibreOfficeMissing when LibreOffice is absent and ErrUnsupportedOffice
// for an extension outside the allowlist.
func ConvertOfficeToPDF(data []byte, ext string) ([]byte, error) {
	if !SupportedOfficeExt(ext) {
		return nil, ErrUnsupportedOffice
	}
	soffice := libreOfficePath()
	if soffice == "" {
		return nil, ErrLibreOfficeMissing
	}
	dir, err := os.MkdirTemp("", "nib-office-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// Name the input with its real extension so LibreOffice selects the right
	// import filter; the basename is fixed so the output name is predictable.
	clean := strings.ToLower(strings.TrimPrefix(ext, "."))
	inPath := filepath.Join(dir, "in."+clean)
	outDir := filepath.Join(dir, "out")
	profile := filepath.Join(dir, "profile")
	if err := os.Mkdir(outDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(inPath, data, 0o600); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), officeConvertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, soffice,
		"--headless", "--nologo", "--nofirststartwizard",
		"-env:UserInstallation=file://"+profile, // isolated profile → safe to run concurrently
		"--convert-to", "pdf", "--outdir", outDir,
		inPath,
	)
	return runConvert(ctx, cmd, filepath.Join(outDir, "in.pdf"), "LibreOffice")
}
