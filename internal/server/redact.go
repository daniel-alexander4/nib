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
	ws := r.MultipartForm.Value["pageW"]
	hs := r.MultipartForm.Value["pageH"]
	if len(files) == 0 || len(files) != len(nums) || len(files) != len(ws) || len(files) != len(hs) {
		httpError(w, http.StatusBadRequest, "page images, numbers, and sizes must match")
		return
	}

	raster := make(map[int]pdfops.RasterPage, len(files))
	for i, fh := range files {
		n, err := strconv.Atoi(nums[i])
		if err != nil {
			httpError(w, http.StatusBadRequest, "bad page number")
			return
		}
		pw, ph, ok := pageSize(ws[i], hs[i])
		if !ok {
			httpError(w, http.StatusBadRequest, "bad page size")
			return
		}
		b, err := readFormFile(fh)
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read page image")
			return
		}
		raster[n] = pdfops.RasterPage{Image: b, W: pw, H: ph}
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

// maxPageDim is PDF's largest legal page side in points (200 inches); any larger
// page size in a request is rejected as bogus.
const maxPageDim = 14400

// pageSize parses a width/height pair (PDF points) the client sent for a
// rasterized page, validating both are positive and within PDF's legal range.
func pageSize(ws, hs string) (w, h float64, ok bool) {
	w, errW := strconv.ParseFloat(ws, 64)
	h, errH := strconv.ParseFloat(hs, 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 || w > maxPageDim || h > maxPageDim {
		return 0, 0, false
	}
	return w, h, true
}
