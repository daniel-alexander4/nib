package server

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"nib/internal/browserrender"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"nib/internal/gate"
	"nib/internal/pdfops"
)

// # An imported document is PATHLESS, and that is the whole hazard
//
// `readInstallablePDF`'s comment names it: a path-opened document reports canSave, so a non-PDF installed
// with its path would be overwritten with PDF bytes by the next Save — the one open surface where getting
// this wrong destroys the file. A converted document (an image, a .docx, a .md) is therefore installed
// WITHOUT a path, Save routes to Save As, and the source is never a destination (L1). This file replaced
// `imageopen.go`'s `asOpenableDocument` (PLAN-import-export P01.S02a), which decided the same thing for the
// image alone and left every Office file to a fifth route.
//
// The server's side of the import door (ADR-185, D8): the ONE place an import's options are built and its
// refusals are given a status and a sentence. `pdfops.Import` decides what a file is and converts it; the
// doors — path, upload, URL, hand-off, Combine — only say where the bytes are and who is asking, so no
// door keeps a refusal of its own (ADR-009, and the defect: the same Office file was converted at one door
// and refused at four).

// roomError is Import's Admit refusing: the open-document cap. It stays a 409, as roomToOpen's refusals
// have always been.
type roomError struct{ error }

func (e roomError) Unwrap() error { return e.error }

// importOptions are the options every door shares. door labels the counters.
func (s *Server) importOptions(door string) pdfops.ImportOptions {
	return pdfops.ImportOptions{
		Door: door,
		// Asked after a gated conversion has its slot and before it does any work: the wait is long enough
		// for the answer to change, and it is seconds of LibreOffice for a document the install would then
		// refuse (the question `handleOffice` asked before converting, now asked once, here).
		Admit: func(context.Context) error {
			if err := s.roomToOpen(); err != nil {
				return roomError{err}
			}
			return nil
		},
	}
}

// importRefusal is the status and sentence for an Import error, from ONE function. The sentence is the
// error's own (one sentence per cause); a missing tool is the exception, worded the way every surface
// words it (ADR-009 classifies, `MissingToolFor`) with the vendor link left to the page, which carries its
// own — a remedy URL on the wire would let a response body choose where the page navigates (ADR-039).
func importRefusal(err error) (status int, msg string) {
	var room roomError
	var conv *pdfops.ConversionError
	switch {
	case errors.As(err, &room):
		return http.StatusConflict, room.Error()
	case errors.Is(err, pdfops.ErrImportTooLarge), errors.Is(err, pdfops.ErrConvertedTooLarge), errors.Is(err, pdfops.ErrTooManyPages),
		errors.Is(err, pdfops.ErrTextTooLong), errors.Is(err, pdfops.ErrCSVTooLong), errors.Is(err, pdfops.ErrCSVTooWide), errors.Is(err, pdfops.ErrCSVTooManyCells):
		return http.StatusRequestEntityTooLarge, err.Error()
	case errors.Is(err, pdfops.ErrConverterOverMemory), errors.Is(err, pdfops.ErrGhostscriptTooOld), errors.Is(err, pdfops.ErrGhostscriptNotConfined), errors.Is(err, pdfops.ErrPostScriptUnreadable), errors.Is(err, pdfops.ErrHEIFCannotRead), errors.Is(err, pdfops.ErrHEIFUnreadable), errors.Is(err, pdfops.ErrBrowserCannotRender), errors.Is(err, pdfops.ErrHTMLUnreadable), errors.Is(err, browserrender.ErrNotServed), errors.Is(err, pdfops.ErrPackageTooBig), errors.Is(err, pdfops.ErrPackageUnreadable), errors.Is(err, pdfops.ErrMsgNotSupported):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, gate.ErrQueueFull), errors.Is(err, pdfops.ErrShuttingDown):
		return http.StatusServiceUnavailable, err.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "that document took too long to convert"
	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout, "the conversion was cancelled"
	case errors.Is(err, pdfops.ErrNotADocument), errors.Is(err, pdfops.ErrCorruptDocument), errors.Is(err, pdfops.ErrNotText), errors.Is(err, pdfops.ErrNotAPDF),
		errors.Is(err, pdfops.ErrExtensionDisagrees), errors.Is(err, pdfops.ErrArchiveRefused), errors.Is(err, pdfops.ErrPackageNamesOutside),
		errors.Is(err, pdfops.ErrNoPages), errors.Is(err, pdfops.ErrTIFFCompression), errors.Is(err, pdfops.ErrImagePixels), errors.Is(err, pdfops.ErrCSVUnreadable):
		return http.StatusUnsupportedMediaType, err.Error()
	}
	if tool, ok := pdfops.MissingToolFor(err); ok {
		return http.StatusBadRequest, "Nib " + tool.NotFound() + ", so it cannot convert this document."
	}
	if errors.As(err, &conv) && conv.Image {
		return http.StatusUnsupportedMediaType, err.Error()
	}
	return http.StatusBadRequest, err.Error()
}

// importOrRefuse runs Import and, on a refusal, answers it. ok is false when it did.
func (s *Server) importOrRefuse(w http.ResponseWriter, ctx context.Context, src io.ReaderAt, size int64, name string, o pdfops.ImportOptions) (*pdfops.ImportResult, bool) {
	res, err := pdfops.Import(ctx, src, size, name, o)
	if err != nil {
		status, msg := importRefusal(err)
		httpError(w, status, msg)
		return nil, false
	}
	return res, true
}

// openRegular opens path and answers for the DESCRIPTOR: the file it returns is the one that was checked. A
// stat followed by an open is two looks at a name, and a FIFO swapped in between would block the read under
// `setupMu`; O_NONBLOCK makes the open itself unable to wait for a writer, and the descriptor's own Stat must
// then say regular file. The caller closes f.
//
// A missing path, a directory and anything that is not a regular file are 404 as they always were; a path
// Nib may not read is a 403 with its own sentence, which "could not read file" (500) said nothing true about.
func openRegular(path string) (*os.File, os.FileInfo, *pathRefusal) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, nil, &pathRefusal{kind: refuseUnreadable, status: http.StatusForbidden, msg: "Nib is not allowed to read that file", err: err}
		}
		return nil, nil, &pathRefusal{kind: refuseUnopenable, status: http.StatusNotFound, msg: "file not found"}
	}
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		f.Close()
		return nil, nil, &pathRefusal{kind: refuseUnopenable, status: http.StatusNotFound, msg: "file not found"}
	}
	// **Only a regular file is read** (/pending 691 R6-5): a FIFO, a socket or a device stats with size 0, so it
	// would clear the size cap, and a read of a FIFO with no writer never returns.
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, nil, &pathRefusal{kind: refuseUnopenable, status: http.StatusNotFound, msg: "not a file that can be opened"}
	}
	return f, info, nil
}

// importOpened runs Import over a file openRegular returned: its handle is held for the import, so a queued import
// holds a descriptor, not its bytes. The call to Import stays in this file (ADR-185's census).
func importOpened(ctx context.Context, f *os.File, info os.FileInfo, name string, o pdfops.ImportOptions) (*pdfops.ImportResult, error) {
	return pdfops.Import(ctx, f, info.Size(), name, o)
}

// isImportRefusal reports whether err is Import REFUSING or FAILING a conversion — a verdict about the file
// — as opposed to a read error, which is the disk's.
func isImportRefusal(err error) bool {
	var room roomError
	var conv *pdfops.ConversionError
	if errors.As(err, &room) || errors.As(err, &conv) {
		return true
	}
	for _, target := range []error{
		pdfops.ErrImportTooLarge, pdfops.ErrConvertedTooLarge, pdfops.ErrConverterOverMemory, pdfops.ErrOptionsNeedMarkdown, gate.ErrQueueFull, pdfops.ErrShuttingDown, context.DeadlineExceeded, context.Canceled,
		pdfops.ErrNotADocument, pdfops.ErrCorruptDocument, pdfops.ErrNotText, pdfops.ErrNotAPDF, pdfops.ErrExtensionDisagrees, pdfops.ErrArchiveRefused, pdfops.ErrPackageNamesOutside,
		pdfops.ErrImagePixels, pdfops.ErrTooManyPages, pdfops.ErrNoPages, pdfops.ErrTIFFCompression,
		pdfops.ErrGhostscriptTooOld, pdfops.ErrGhostscriptNotConfined, pdfops.ErrPostScriptUnreadable, pdfops.ErrHEIFCannotRead, pdfops.ErrHEIFUnreadable, pdfops.ErrBrowserCannotRender, pdfops.ErrHTMLUnreadable, browserrender.ErrNotServed, pdfops.ErrPackageTooBig, pdfops.ErrPackageUnreadable, pdfops.ErrMsgNotSupported,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	_, missing := pdfops.MissingToolFor(err)
	return missing
}

// combineOptions are Import's options for a file being combined: no Admit, because Combine installs ONE
// document and the install asks the cap once.
func (s *Server) combineOptions() pdfops.ImportOptions { return pdfops.ImportOptions{Door: "combine"} }

// maxQueuedUpload bounds the bytes of uploads admitted at once, counted from the declared Content-Length
// BEFORE the body is read (R6, X2). The queue behind the gate is sixteen deep and an upload may be 200 MiB, so
// without this a burst could spill 3 GiB to disk for conversions that have not started. The figure is an
// assumption, tuned with the measured run.
const maxQueuedUpload = 1 << 30

// uploadReadSlack and uploadReadRate size a body's read deadline from its declared length: a fixed floor
// plus the time the declared bytes need at a rate a loopback browser beats by orders of magnitude. A
// GLOBAL ReadTimeout would kill the window's event stream (`cmd/nib` sets none), so the deadline is
// per-route and per-request (Y4).
const (
	uploadReadSlack = 60 * time.Second
	uploadReadRate  = 4 << 20 // bytes per second
)

// admitUpload is asked by every route that takes a document upload, BEFORE parseMultipart reads a byte: a
// declared length past the input cap is refused outright (413), a burst past the queued-upload total is
// refused by name (503), and the body gets a read deadline sized from what it declared. The returned
// release gives the declared bytes back and must be deferred. An unknown length (-1) is bounded by
// MaxBytesReader alone and counts for the whole cap, the conservative reading.
func (s *Server) admitUpload(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	n := r.ContentLength
	if n > pdfops.MaxImportBytes+(1<<20) { // the cap plus the multipart framing
		httpError(w, http.StatusRequestEntityTooLarge, pdfops.ErrImportTooLarge.Error())
		return nil, false
	}
	if n < 0 {
		n = pdfops.MaxImportBytes
	}
	if s.queuedUpload.Add(n) > maxQueuedUpload && n > 0 {
		s.queuedUpload.Add(-n)
		httpError(w, http.StatusServiceUnavailable, "too many large files are being received at once; try fewer")
		return nil, false
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadReadSlack + time.Duration(n/uploadReadRate)*time.Second))
	return func() {
		s.queuedUpload.Add(-n)
		// The deadline was sized for THIS body; a kept-alive connection must not carry it into the next request.
		_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	}, true
}

// importedBase is the source name the window says it imported from — the file's base name — when res is a
// conversion, and "" for a PDF that came as it was (nothing was imported, so nothing is announced).
func importedBase(res *pdfops.ImportResult, name string) string {
	if res == nil || !res.Converted {
		return ""
	}
	return filepath.Base(name)
}
