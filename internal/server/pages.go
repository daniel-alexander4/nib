package server

import (
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
		deg, _ := strconv.Atoi(r.FormValue("deg"))
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
	default:
		httpError(w, http.StatusBadRequest, "unknown page operation")
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "page operation failed: "+err.Error())
		return
	}

	s.mu.Lock()
	if s.doc != nil {
		s.doc.data = result
		s.doc.sig = sign.Verify(result)
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
