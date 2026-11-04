package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"nib/internal/tempdir"
)

// Refusals named by the SVG export (ADR-205).
var (
	// ErrPdftocairoMissing: poppler's pdftocairo was not found. Classified by MissingToolFor.
	ErrPdftocairoMissing = errors.New("pdftocairo (poppler) is not installed")
	// ErrSVGPage: the page asked for is not one the document has.
	ErrSVGPage = errors.New("that page is not in the document")
	// ErrSVGUnreadable: pdftocairo ran and did not produce a drawing of that page.
	ErrSVGUnreadable = errors.New("that page could not be drawn as an SVG")
)

const (
	// pdftocairoTimeout is the tool's wall clock (the plan's starting value, W-table: 60 s).
	pdftocairoTimeout = 60 * time.Second
)

var pdftocairoCache toolCache

func pdftocairoPath() string {
	names := []string{"pdftocairo"}
	return pdftocairoCache.find(names, pdftocairoCandidates())
}

// pdftocairoCandidates: poppler is packaged onto PATH on Linux (poppler-utils) and macOS (Homebrew); it is not
// on stock Windows, where only a user-installed copy is found (declared, D7).
func pdftocairoCandidates() []string { return nil }

// PdftocairoAvailable reports whether pdftocairo is FOUND. It starts no process (/api/status runs no probe).
func PdftocairoAvailable() bool { return pdftocairoPath() != "" }

// pdftocairoArgs is the ONE spelling of the tool's arguments. `-svg` writes the file it is told to, exactly, when
// one page is asked for; `-nocrop`/`-nocenter` are not needed for vector output.
func pdftocairoArgs(in, out string, page int) []string {
	p := fmt.Sprint(page)
	return []string{"-svg", "-f", p, "-l", p, in, out}
}

// ExportPageSVG draws one page (1-based) of pdf as an SVG file through pdftocairo, in a directory of its own, under
// the converters' environment and timeout. The page is drawn from the bytes as they are: nothing is rewritten first.
func ExportPageSVG(ctx context.Context, pdf []byte, page int) ([]byte, error) {
	exe := pdftocairoPath()
	if exe == "" {
		return nil, ErrPdftocairoMissing
	}
	if n, err := PageCount(pdf); err != nil || page < 1 || page > n {
		return nil, ErrSVGPage
	}
	dir, err := tempdir.New("svgexport")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	in, out := filepath.Join(dir.Path(), "in.pdf"), filepath.Join(dir.Path(), "out.svg")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, pdftocairoTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, pdftocairoArgs(in, out, page)...)
	cmd.Env = toolEnv(os.Environ(), dir.Path())
	body, err := runConvert(ctx, cmd, out, "pdftocairo", dir)
	if err != nil {
		return nil, err
	}
	if !isSVGDrawing(body) {
		return nil, ErrSVGUnreadable
	}
	return body, nil
}

// isSVGDrawing is the check that what came back is the tool's SVG and not an empty file or an error page.
func isSVGDrawing(b []byte) bool {
	head := b
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.Contains(head, []byte("<svg"))
}
