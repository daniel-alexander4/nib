package server

import (
	"net/http"

	"nib/internal/pdfops"
)

// handleInspect answers what the open document says about itself (/pending 406): its document information,
// its XMP packet, the fonts it names, and the hidden-content scan. Read-only, so a GET, and allowed on a signed
// document. Every value is one the document chose; `pdfops.InspectDocument` cleans and caps each, and the
// answer goes in the body only, never a header (ADR-193). A document that cannot be read is a 422 naming its
// cause (ADR-072); one the server no longer holds is resolveDoc's 409 (ADR-004).
func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	got, err := pdfops.InspectDocument(s.docBytes(doc))
	if err != nil {
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"cause": "document-unreadable", "error": "Nib could not read this document's own description: " + err.Error()})
		return
	}
	writeJSON(w, got)
}
