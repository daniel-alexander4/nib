package server

import (
	"net/http"
	"strconv"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// handleRedact replaces the marked pages with flat images (in which the
// redaction boxes are already painted) so the underlying content is genuinely
// removed, not covered. The client posts its saved bytes plus, for each redacted
// page, a "page" image part and a parallel "pageNum" value. Non-redacted pages
// keep their original vector content.
func (s *Server) handleRedact(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	files := r.MultipartForm.File["page"]
	nums := r.MultipartForm.Value["pageNum"]
	if len(files) == 0 || len(files) != len(nums) {
		httpError(w, http.StatusBadRequest, "page images and numbers must match")
		return
	}

	raster := make(map[int][]byte, len(files))
	for i, fh := range files {
		n, err := strconv.Atoi(nums[i])
		if err != nil {
			httpError(w, http.StatusBadRequest, "bad page number")
			return
		}
		b, err := readFormFile(fh)
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read page image")
			return
		}
		raster[n] = b
	}

	result, err := pdfops.RedactPages(pdfBytes, raster)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "redaction failed: "+err.Error())
		return
	}

	sig := sign.Verify(result)
	s.mu.Lock()
	if s.doc != nil {
		s.doc.data = result
		s.doc.sig = sig
	}
	s.mu.Unlock()
	writeJSON(w, s.docResponse())
}
