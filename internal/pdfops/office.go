package pdfops

import (
	"context"
	"errors"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// ErrLibreOfficeMissing is returned when no LibreOffice binary can be found, so the caller
// can degrade gracefully rather than fail obscurely. Both surfaces classify it through
// `MissingToolFor` and then say it in their own voice: the CLI prints a sentence naming the
// remedy, and the File card reveals a line that carries the vendor link.
//
// This comment used to promise a "hidden GUI menu item" and `web/app.js` claimed the convert
// button was "hidden otherwise" — neither was ever built, and the button has always been
// visible with only the file picker narrowing. The text is the surface that exists.
//
// The string itself never reaches a user: every door intercepts the sentinel and substitutes
// its own wording, which is why widening discovery did not require rewording it.
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

var loCache toolCache

// libreOfficePath returns the LibreOffice executable path, or "" if none is found.
//
// PATH first — "soffice", also symlinked "libreoffice", and "soffice.exe" on Windows, which
// exec.LookPath resolves via PATHEXT. Then the stock install locations, because the default
// macOS and Windows installs are NOT on PATH and a PATH-only probe reports them as absent.
// `toolpath.go` carries the reasoning, including why a found path is cached for the process
// and an empty answer is re-probed on every call.
func libreOfficePath() string {
	return loCache.find([]string{"soffice", "libreoffice"}, libreOfficeCandidates())
}

// LibreOfficeAvailable reports whether LibreOffice can be found, so a caller can offer office
// conversion (and the UI can say so when it cannot).
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
		"-env:UserInstallation="+fileURL(filepath.ToSlash(profile)), // isolated profile → safe to run concurrently
		"--convert-to", "pdf", "--outdir", outDir,
		inPath,
	)
	return runConvert(ctx, cmd, filepath.Join(outDir, "in.pdf"), "LibreOffice")
}

// fileURL spells a slash-separated local path as a `file:` URL, for LibreOffice's `-env:UserInstallation`
// (`/pending 503`).
//
// `"file://"+path` is right only for an absolute POSIX path. A Windows temp directory is
// `C:\Users\…\Temp\nib-office-…`, which that spelling makes `file://C:\Users…` — `C:` in the host position
// and backslashes a URL does not have — and a user name with a space in it is not a URL at all. So the
// caller hands the path slash-separated (`filepath.ToSlash`, the identity on Unix), a drive-letter path
// gets the root slash a URL path needs, and `net/url` escapes the rest.
func fileURL(slashPath string) string {
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	return (&url.URL{Scheme: "file", Path: slashPath}).String()
}
