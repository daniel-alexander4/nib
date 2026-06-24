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

// The general, vector-preserving PDF/A path: shell out to an installed Ghostscript.
// It handles what the pure-Go PreparePDFA refuses — re-embedding referenced-but-
// unembedded fonts and converting colour — at the cost of an optional external
// runtime dependency (detected at runtime, never bundled; the binary stays
// cgo-free). Opt-in by design.
//
// SECURITY: Ghostscript *interprets* the input PDF (a historically CVE-prone
// interpreter). This is opt-in (the user installed gs and chose this path), runs
// under SAFER (gs ≥9.50 default; file access limited to the one ICC we permit),
// with a timeout and no shell (args as a slice). Nib is loopback/single-user, so
// the residual risk is the same class as opening a malicious PDF in any viewer.
// Like the pure-Go path the result is a *candidate*; verify with veraPDF.

// ErrGhostscriptMissing is returned when no Ghostscript binary is on PATH, so the
// caller can degrade gracefully (CLI hint, hidden GUI button) rather than fail
// obscurely.
var ErrGhostscriptMissing = errors.New("Ghostscript (gs) is not installed")

const gsConvertTimeout = 2 * time.Minute

var gsPath struct {
	sync.Once
	p string
}

// ghostscriptPath returns the Ghostscript executable path (cached), or "" if none
// is installed. The binary is "gs" on Unix, "gswin64c"/"gswin32c" on Windows.
func ghostscriptPath() string {
	gsPath.Do(func() {
		for _, name := range []string{"gs", "gswin64c", "gswin32c"} {
			if p, err := exec.LookPath(name); err == nil {
				gsPath.p = p
				return
			}
		}
	})
	return gsPath.p
}

// GhostscriptAvailable reports whether Ghostscript is installed, so a caller can
// offer the general converter (and the UI can show/hide it).
func GhostscriptAvailable() bool { return ghostscriptPath() != "" }

// ConvertPDFAGhostscript converts pdf to a PDF/A-2b candidate via an installed
// Ghostscript. It writes the document, the vendored sRGB profile, and a PDF/A
// definition (which attaches the sRGB OutputIntent — bare -dPDFA=2 omits it) into
// a temp dir, then runs gs under SAFER. Returns ErrGhostscriptMissing when gs is
// absent.
func ConvertPDFAGhostscript(pdf []byte) ([]byte, error) {
	gs := ghostscriptPath()
	if gs == "" {
		return nil, ErrGhostscriptMissing
	}
	dir, err := os.MkdirTemp("", "nib-pdfa-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	inPath := filepath.Join(dir, "in.pdf")
	outPath := filepath.Join(dir, "out.pdf")
	iccPath := filepath.Join(dir, "srgb.icc")
	defPath := filepath.Join(dir, "pdfa_def.ps")
	for _, w := range []struct {
		path string
		data []byte
	}{{inPath, pdf}, {iccPath, srgbICC}, {defPath, []byte(pdfaDefPS(iccPath))}} {
		if err := os.WriteFile(w.path, w.data, 0o600); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), gsConvertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gs,
		"-dPDFA=2", "-dPDFACompatibilityPolicy=1",
		"-dBATCH", "-dNOPAUSE", "-dQUIET",
		"-sColorConversionStrategy=RGB", "-sDEVICE=pdfwrite",
		"--permit-file-read="+iccPath,
		"-sOutputFile="+outPath,
		defPath, inPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("Ghostscript timed out converting this document")
		}
		return nil, fmt.Errorf("Ghostscript could not convert this document: %s", gsErrTail(stderr.String(), err))
	}
	out, err := os.ReadFile(outPath)
	if err != nil || len(out) == 0 {
		return nil, fmt.Errorf("Ghostscript produced no output: %s", gsErrTail(stderr.String(), err))
	}
	return out, nil
}

// pdfaDefPS builds the PDF/A definition Ghostscript needs to attach the sRGB
// OutputIntent. It references the vendored ICC at iccPath, which the caller
// permits via --permit-file-read under SAFER.
func pdfaDefPS(iccPath string) string {
	return "%!\n" +
		"[/_objdef {icc_PDFA} /type /stream /OBJ pdfmark\n" +
		"[{icc_PDFA} <</N 3>> /PUT pdfmark\n" +
		"[{icc_PDFA} (" + psEscape(iccPath) + ") (r) file /PUT pdfmark\n" +
		"[/_objdef {OutputIntent_PDFA} /type /dict /OBJ pdfmark\n" +
		"[{OutputIntent_PDFA} <<\n" +
		"  /Type /OutputIntent /S /GTS_PDFA1\n" +
		"  /DestOutputProfile {icc_PDFA}\n" +
		"  /OutputConditionIdentifier (sRGB)\n" +
		"  /Info (sRGB IEC61966-2.1)\n" +
		">> /PUT pdfmark\n" +
		"[{Catalog} <</OutputIntents [ {OutputIntent_PDFA} ]>> /PUT pdfmark\n"
}

// psEscape escapes a path for a PostScript ( ) literal string.
func psEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s)
}

// gsErrTail returns a short, useful tail of Ghostscript's stderr for an error.
func gsErrTail(stderr string, err error) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return err.Error()
	}
	lines := strings.Split(stderr, "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.Join(lines, "; ")
}
