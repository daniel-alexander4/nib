package server

import (
	"encoding/json"
	"net/http"

	"nib/internal/pdfops"
)

// outlineResponse carries the document outline as a flat, leveled list.
type outlineResponse struct {
	Items []pdfops.OutlineItem `json:"items"`
}

// handleOutlineGet reports the current document's bookmark outline (read-only GET).
func (s *Server) handleOutlineGet(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	items, err := pdfops.Outline(doc.data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read the outline: "+err.Error())
		return
	}
	writeJSON(w, outlineResponse{Items: items})
}

// handleOutlineSet replaces the document outline and makes the result the current
// document. It takes the client's baked PDF (field "pdf", so pending overlay edits
// are preserved) plus the new outline as JSON in the "outline" field.
func (s *Server) handleOutlineSet(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	var items []pdfops.OutlineItem
	if err := json.Unmarshal([]byte(r.FormValue("outline")), &items); err != nil {
		httpError(w, http.StatusBadRequest, "could not read the outline")
		return
	}
	result, err := pdfops.SetOutline(pdfBytes, items)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.commitMutation(pdfBytes, result)
	writeJSON(w, s.docResponse())
}
