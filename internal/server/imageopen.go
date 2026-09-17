package server

import (
	"errors"
	"fmt"

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

// asOpenableDocument returns bytes nib can install.
//
// converted says the source was an image and the result is a freshly built PDF, which the caller
// must install WITHOUT a path. ok is false when the bytes are neither a PDF nor an image, which is
// the caller's existing refusal and keeps its existing sentence.
func asOpenableDocument(data []byte, name string) (out []byte, converted, ok bool, err error) {
	if pdfops.LooksLikePDF(data) {
		return data, false, true, nil
	}
	if !pdfops.LooksLikeImage(data) {
		return nil, false, false, nil
	}
	pdf, cerr := pdfops.ImageToDocument(data, name)
	if cerr != nil {
		// An image nib recognised and could not convert is NOT the not-a-PDF refusal: the user
		// picked something nib said it opens, and "that file isn't a PDF" would be a lie about a
		// PNG. Errors.Is on ErrNotAnImage cannot fire here — the sniff already passed — so this is
		// a decode or size refusal and it carries its own sentence.
		if errors.Is(cerr, pdfops.ErrNotAnImage) {
			return nil, false, false, nil
		}
		return nil, true, true, fmt.Errorf("%w", cerr)
	}
	return pdf, true, true, nil
}
