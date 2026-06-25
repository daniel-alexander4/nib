package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("LibreOffice timed out converting this document")
		}
		return nil, fmt.Errorf("LibreOffice could not convert this document: %s", gsErrTail(stderr.String(), err))
	}
	out, err := os.ReadFile(filepath.Join(outDir, "in.pdf"))
	if err != nil || len(out) == 0 {
		return nil, fmt.Errorf("LibreOffice produced no output: %s", gsErrTail(stderr.String(), err))
	}
	return out, nil
}
