package server

import (
	"net/http"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// handleScan reports the active and hidden content in the current document
// (auto-run hooks, JavaScript, risky actions, attachments, layers, metadata).
// It is read-only — it never alters the document — so it is a plain GET.
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	rep, err := pdfops.Scan(doc.data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not scan: "+err.Error())
		return
	}
	writeJSON(w, rep)
}

// sanitizeResponse is the result of a removal: the usual document metadata plus
// whether the removal produced a sound document (Ok) and what content still
// remains afterwards (Residual). When Ok is false the open document is left
// untouched and the UI recommends stepping down to the next removal method.
type sanitizeResponse struct {
	docResponse
	Ok       bool              `json:"ok"`
	Residual pdfops.ScanReport `json:"residual"`
}

// handleSanitize removes active or hidden content from the current document by
// the requested method: "strip" neutralizes all active content while keeping
// the visible pages and text; "safe" removes only embedded files and dangerous
// media annotations through pdfcpu's own APIs. (The guaranteed flatten is the
// client-rasterized /api/assemble path.) A strip that fails to validate leaves
// the open document unchanged so the user can fall back safely.
func (s *Server) handleSanitize(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}

	var result []byte
	var err error
	switch r.FormValue("method") {
	case "strip":
		result, err = pdfops.StripActive(doc.data)
	case "safe":
		result, err = pdfops.RemoveFilesAndMedia(doc.data)
	case "metadata":
		result, err = pdfops.StripMetadata(doc.data)
	default:
		httpError(w, http.StatusBadRequest, "unknown method")
		return
	}
	// A removal failure, or a result that won't validate, is reported as
	// not-ok (not an error) so the UI can recommend the next method down. The
	// open document is deliberately left as-is in both cases.
	if err != nil || pdfops.Validate(result) != nil {
		writeJSON(w, sanitizeResponse{docResponse: s.docResponse(), Ok: false})
		return
	}

	residual, _ := pdfops.Scan(result)
	sig := sign.Verify(result)
	s.mu.Lock()
	if s.doc != nil {
		s.doc.data = result
		s.doc.sig = sig
	}
	s.mu.Unlock()
	writeJSON(w, sanitizeResponse{docResponse: s.docResponse(), Ok: true, Residual: residual})
}
