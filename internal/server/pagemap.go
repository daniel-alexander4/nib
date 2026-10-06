package server

import (
	"net/http"
	"strconv"

	"nib/internal/pdfops"
)

// handlePageMap answers where a page's text, rules, boxes and form fields are (ADR-088).
//
// Read-only and pinned like every document read: the map is of the document the request names, as the server holds
// it — which is what a field placed from it will be baked into.
func (s *Server) handlePageMap(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "page must be a number")
		return
	}
	m, err := pdfops.MapPage(s.docBytes(doc), page)
	if err != nil {
		// 422, not 400: the request was well-formed and the PAGE could not be read (ADR-072). The caller falls back to
		// what it did before there was a map, and says so.
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"cause": "page-unreadable", "error": err.Error()})
		return
	}
	writeJSON(w, m)
}
