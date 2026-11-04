package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"nib/internal/browser"
	"nib/internal/browserrender"
	"nib/internal/tempdir"
)

// A saved web page in (PLAN-import-export P04.S04a, D29, R3, X4, X5, Y8, ADR-200). The page is sanitised
// (sanitizeHTML), served once from a loopback origin (internal/browserrender) and printed to a PDF by a
// Chromium-family browser run headless. This file starts the browser — through runConvert, so it is in the
// converter's process group, memory watchdog and Quit registry — and asks the origin whether the page was
// ever delivered. The flags are browserrender's alone (Origin.Args).
//
// MHTML and EPUB are S04b: they unpack in memory and arrive here as the same sanitised document.

// Refusals named by the browser door.
var (
	// ErrBrowserMissing: no Chromium-family browser was found. Classified by MissingToolFor.
	ErrBrowserMissing = errors.New("no Chromium-family browser (Chrome, Chromium, Edge or Brave) is installed")
	// ErrBrowserCannotRender: a browser was found and could not print a page to a PDF here (a snap-confined
	// build that cannot use Nib's profile folder, a sandbox that refuses to start, a stub).
	ErrBrowserCannotRender = errors.New("the browser on this computer could not print a test page to a PDF, so Nib will not use it to open web pages")
	// ErrHTMLUnreadable: the browser ran and produced nothing that is the page.
	ErrHTMLUnreadable = errors.New("the browser could not turn that web page into a document")
)

const htmlRenderTimeout = 60 * time.Second

// browserPath finds the browser; a test names a stand-in.
var browserPath = func() string {
	p, _ := browser.FindChromium()
	return p
}

// BrowserAvailable reports whether a browser is found and has printed a test page here.
func BrowserAvailable() bool {
	p := browserPath()
	if p == "" {
		return false
	}
	return renderProbe(context.Background(), p) == nil
}

type renderState struct {
	path string
	size int64
	mod  time.Time
	err  error
}

var renderCache struct {
	mu sync.Mutex
	st renderState
}

// renderProbe answers once per browser binary whether it can print a page through the door at all — the
// whole door, listener and flags included, so what is tested is what is used. It is the capability probe D29
// asks for, and it is what a snap-confined Chromium (which cannot read a profile under Nib's temp folder)
// or a root-run one (which refuses its sandbox) fails.
func renderProbe(ctx context.Context, chromium string) error {
	fi, serr := os.Stat(chromium)
	renderCache.mu.Lock()
	defer renderCache.mu.Unlock()
	if serr == nil {
		c := renderCache.st
		if c.path == chromium && c.size == fi.Size() && c.mod.Equal(fi.ModTime()) {
			return c.err
		}
	}
	pdf, err := renderDocument(ctx, chromium, []byte("<!doctype html><meta charset=utf-8><p>Nib probe</p>"))
	if err == nil {
		if n, perr := PageCount(pdf); perr != nil || n != 1 {
			err = fmt.Errorf("%w: it wrote %d pages", ErrBrowserCannotRender, n)
		}
	} else if !errors.Is(err, ErrBrowserCannotRender) && ctx.Err() == nil {
		err = fmt.Errorf("%w: %v", ErrBrowserCannotRender, err)
	}
	if ctx.Err() != nil {
		return err // a cancelled probe says nothing about the browser
	}
	if serr == nil {
		renderCache.st = renderState{path: chromium, size: fi.Size(), mod: fi.ModTime(), err: err}
	}
	return err
}

// ConvertHTMLToPDF turns a web page into a PDF through an installed browser. The caller holds the gate
// (Import does) and its context ends the render, killing the process tree.
func ConvertHTMLToPDF(ctx context.Context, data []byte) ([]byte, error) {
	chromium := browserPath()
	if chromium == "" {
		return nil, ErrBrowserMissing
	}
	if err := renderProbe(ctx, chromium); err != nil {
		return nil, err
	}
	doc, _, err := sanitizeHTML(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHTMLUnreadable, err)
	}
	pdf, err := renderDocument(ctx, chromium, doc)
	if err != nil {
		return nil, err
	}
	n, perr := PageCount(pdf)
	if perr != nil || n == 0 {
		return nil, ErrHTMLUnreadable
	}
	if n > importPages {
		return nil, fmt.Errorf("%w: Nib opens up to %d", ErrTooManyPages, importPages)
	}
	return pdf, nil
}

// renderDocument serves doc once and prints it. The origin is closed on every path, and the profile folder
// is removed by the temp door's Close.
func renderDocument(ctx context.Context, chromium string, doc []byte) ([]byte, error) {
	o, err := browserrender.Listen(doc)
	if err != nil {
		return nil, err
	}
	defer o.Close()
	dir, err := tempdir.New("html")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	profile := dir.Join("profile")
	if err := os.Mkdir(profile, 0o700); err != nil {
		return nil, err
	}
	outPath := dir.Join("out.pdf")
	ctx, cancel := context.WithTimeout(ctx, htmlRenderTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, chromium, o.Args(profile, outPath)...)
	cmd.Env = toolEnv(os.Environ(), dir.Path())
	pdf, err := runConvert(ctx, cmd, outPath, "the browser", dir)
	if err != nil {
		return nil, err
	}
	// A retry after the use is spent gets a 404 and a browser may then write a blank page and exit 0 (X4 c):
	// what it wrote is the page only if the page was delivered whole.
	if !o.Served() {
		return nil, browserrender.ErrNotServed
	}
	if !strings.HasPrefix(string(pdf), "%PDF-") {
		return nil, ErrHTMLUnreadable
	}
	return pdf, nil
}

// isHTML reports whether the head of a file is a web page: after a byte-order mark and white space it
// opens `<!doctype html` or `<html`, or it opens with a tag and an `<html`, `<head` or `<body` follows in the
// head. A Markdown file that merely mentions a tag is not one — the first character must be `<`.
func isHTML(head []byte) bool {
	h := bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF})
	h = bytes.TrimLeft(h, " \t\r\n\f")
	if len(h) == 0 || h[0] != '<' || bytes.IndexByte(h, 0) >= 0 {
		return false
	}
	l := bytes.ToLower(h)
	if bytes.HasPrefix(l, []byte("<!doctype html")) || bytes.HasPrefix(l, []byte("<html")) {
		return true
	}
	for _, t := range []string{"<html", "<head", "<body"} {
		if bytes.Contains(l, []byte(t)) {
			return true
		}
	}
	return false
}
