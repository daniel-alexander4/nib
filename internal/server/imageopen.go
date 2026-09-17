package server

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"nib/internal/pdfops"
)

// Opening an image as a document, server side — `/pending 400`.
//
// # One conversion, four doors
//
// Four routes admit bytes — path open, upload, fetch-by-URL and the OS hand-off — and every one of
// them refused anything without a PDF header. `asOpenableDocument` is the single place that decides
// whether bytes nib cannot open as a PDF are an image it can convert, so a format is added once and
// the page-size rule cannot drift between routes (ADR-009).
//
// # An opened image is PATHLESS, and that is the whole hazard
//
// `readInstallablePDF`'s own comment names it: *"A path-opened document reports canSave, so a
// non-PDF installed here would be overwritten with PDF bytes by the next Save — the one open
// surface where getting this wrong destroys the file."* An image opened by path is exactly that
// case. So the conversion reports `converted`, the path route drops the path, and Save routes to
// Save As — the same shape an office conversion already takes (`office.go`, `path: ""`).
//
// The source path is still worth recording in Open Recent, and reopening converts again. That
// recording is `/pending 490`'s.

// openability is what the bytes — and, for the last case, the name — turned out to be.
//
// **An enum rather than the `(converted, ok bool)` pair it replaces.** /pending 541 added a fourth
// outcome, and a pair of booleans can express four states while saying which is which nowhere: a
// caller reading `ok == false` cannot tell "this is not a document" from "this is a document nib
// converts by another route", and the second wants a different sentence. The enum makes each
// caller name the case it is handling.
type openability int

const (
	openAsIs        openability = iota // already a PDF; install the bytes unchanged
	openConverted                      // an image, converted here; install it WITHOUT a path
	openConvertible                    // not a PDF or an image, but a name `/api/office` converts
	openRefused                        // nothing nib opens
)

// asOpenableDocument decides what these bytes can become.
//
// **The convertible case is decided by the NAME, and that is the honest signal.** A `.docx` is a
// ZIP and a `.md` is text, so their bytes say nothing a sniff can use; what says a document is
// convertible is its extension, which is what `/api/office` itself routes on
// (`pdfops.SupportedDocExt`). So this case makes no claim that the conversion would SUCCEED —
// LibreOffice may be absent, which is ADR-040's subject and a different sentence — only that the
// user picked a type nib has a route for and this route is the wrong one.
func asOpenableDocument(data []byte, name string) (out []byte, kind openability, err error) {
	if pdfops.LooksLikePDF(data) {
		return data, openAsIs, nil
	}
	if !pdfops.LooksLikeImage(data) {
		if pdfops.SupportedDocExt(strings.ToLower(filepath.Ext(name))) {
			return nil, openConvertible, nil
		}
		return nil, openRefused, nil
	}
	pdf, cerr := pdfops.ImageToDocument(data, name)
	if cerr != nil {
		// An image nib recognised and could not convert is NOT the not-a-PDF refusal: the user
		// picked something nib said it opens, and "that file isn't a PDF" would be a lie about a
		// PNG. Errors.Is on ErrNotAnImage cannot fire here — the sniff already passed — so this is
		// a decode or size refusal and it carries its own sentence.
		if errors.Is(cerr, pdfops.ErrNotAnImage) {
			return nil, openRefused, nil
		}
		return nil, openConverted, fmt.Errorf("%w", cerr)
	}
	return pdf, openConverted, nil
}

// convertibleRefusal is the one sentence every door prints for a document nib converts elsewhere.
//
// It names the route by the label the UI actually carries (`web/index.html`'s officeOpenBtn), so a
// user can act on it rather than being told only what went wrong.
func convertibleRefusal(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "Nib converts that kind of document — use Open & convert to PDF…"
	}
	return "Nib converts " + ext + " through Open & convert to PDF… — this route opens PDFs and images"
}
