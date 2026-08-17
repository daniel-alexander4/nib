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
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	items, err := pdfops.Outline(s.docBytes(doc))
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
	// Resolve the document this result is being installed INTO. These four routes
	// never read the open document — they work from posted bytes — so before the
	// registry there was nothing to resolve and "the open one" was unambiguous. It
	// is not any more: without this, an operation addressed to one document commits
	// its result into another.
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	if !s.commitMutation(doc, pdfBytes, result) {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	writeJSON(w, s.docResponse(doc))
}
