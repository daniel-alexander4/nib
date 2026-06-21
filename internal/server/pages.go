package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// handlePages applies a structural page operation (rotate, delete, reorder,
// append) to the posted document and makes the result the current document.
// The client posts its saved bytes (edits already baked) since these ops
// restructure the PDF in ways pdf.js cannot do client-side.
func (s *Server) handlePages(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	pages := splitPages(r.FormValue("pages"))

	var (
		result []byte
		err    error
	)
	switch r.FormValue("op") {
	case "rotate":
		deg, convErr := strconv.Atoi(r.FormValue("deg"))
		if convErr != nil {
			httpError(w, http.StatusBadRequest, "rotation must be a whole number of degrees")
			return
		}
		result, err = pdfops.Rotate(pdfBytes, pages, deg)
	case "delete":
		result, err = pdfops.RemovePages(pdfBytes, pages)
	case "reorder":
		result, err = pdfops.Reorder(pdfBytes, pages)
	case "append":
		other, ok2 := formFileBytes(w, r, "append")
		if !ok2 {
			return
		}
		result, err = pdfops.Append(pdfBytes, other)
	case "split":
		cols, cErr := strconv.Atoi(r.FormValue("cols"))
		rows, rErr := strconv.Atoi(r.FormValue("rows"))
		pageNum, pErr := strconv.Atoi(r.FormValue("page"))
		if cErr != nil || rErr != nil || pErr != nil {
			httpError(w, http.StatusBadRequest, "split needs whole-number page, cols and rows")
			return
		}
		resize := r.FormValue("resize") == "1" || r.FormValue("resize") == "true"
		result, err = pdfops.SplitPage(pdfBytes, pageNum, cols, rows, resize)
	case "splitrects":
		pageNum, pErr := strconv.Atoi(r.FormValue("page"))
		if pErr != nil {
			httpError(w, http.StatusBadRequest, "split needs a whole-number page")
			return
		}
		var rects [][4]float64
		if uErr := json.Unmarshal([]byte(r.FormValue("rects")), &rects); uErr != nil {
			httpError(w, http.StatusBadRequest, "could not read split regions")
			return
		}
		result, err = pdfops.SplitRegions(pdfBytes, pageNum, rects)
	default:
		httpError(w, http.StatusBadRequest, "unknown page operation")
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "page operation failed: "+err.Error())
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

// splitPages parses a comma-separated page selection ("1,3,5"); empty means all.
func splitPages(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
