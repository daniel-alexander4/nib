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
		httpError(w, http.StatusBadRequest, "choose at least two PDFs to combine")
		return
	}
	pdfs := make([][]byte, 0, len(headers))
	for _, fh := range headers {
		data, err := readFormFile(fh)
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read "+fh.Filename)
			return
		}
		pdfs = append(pdfs, data)
	}
	combined, err := pdfops.Combine(pdfs)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not combine the PDFs: "+err.Error())
		return
	}
	s.setDoc(&document{path: "", data: combined, sig: sign.Verify(combined)})
	resp := s.docResponse()
	resp.Name = "combined.pdf"
	writeJSON(w, resp)
}
