package server

import (
	"net/http"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// handleCombine merges several uploaded PDFs — in the order the client sent them,
// which multipart preserves — into one document and makes it the open document.
// Like an upload it produces a fresh, path-less doc (Save As to keep it), so
// Combine works whether or not a document is already open. The files ride in as
// repeated multipart "file" fields, in the user's chosen order.
func (s *Server) handleCombine(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	headers := r.MultipartForm.File["file"]
	if len(headers) < 2 {
		httpError(w, http.StatusBadRequest, "choose at least two PDFs or images to combine")
		return
	}
	pdfs := make([][]byte, 0, len(headers))
	for _, fh := range headers {
		data, err := readFormFile(fh)
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read "+fh.Filename)
			return
		}
		// An image is a page like any other: the same door that opens one converts it here, so
		// its size rule is the opener's (ADR-009). A refused file is left as it came and Combine
		// names it, as it always has; a convertible office file says where it is opened.
		doc, kind, cerr := asOpenableDocument(data, fh.Filename)
		switch {
		case cerr != nil:
			httpError(w, http.StatusBadRequest, fh.Filename+": "+cerr.Error())
			return
		case kind == openConvertible:
			httpError(w, http.StatusBadRequest, fh.Filename+": "+convertibleRefusal(fh.Filename))
			return
		case kind == openRefused:
			doc = data
		}
		pdfs = append(pdfs, doc)
	}
	combined, err := pdfops.Combine(pdfs)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not combine the PDFs: "+err.Error())
		return
	}
	installed, cerr := s.addDocCapped(&document{path: "", name: "combined.pdf", data: combined, sig: sign.Verify(combined)})
	if cerr != nil {
		httpError(w, http.StatusConflict, cerr.Error())
		return
	}
	resp := s.docResponse(installed)
	writeJSON(w, resp)
}
