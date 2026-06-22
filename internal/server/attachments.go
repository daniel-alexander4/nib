package server

import (
	"net/http"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// attachmentsResponse lists the document-level embedded files.
type attachmentsResponse struct {
	Attachments []pdfops.AttachmentInfo `json:"attachments"`
}

// handleAttachmentsList reports the embedded files in the current document. It is
// read-only, so it is a plain GET.
func (s *Server) handleAttachmentsList(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	aa, err := pdfops.Attachments(doc.data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not list attachments: "+err.Error())
		return
	}
	writeJSON(w, attachmentsResponse{Attachments: aa})
}

// handleAttachmentAdd embeds an uploaded file into the current document and makes
// the result the current document (the client reloads it). The file rides in as
// the multipart field "file"; its name comes from the "name" field.
func (s *Server) handleAttachmentAdd(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	data, ok := formFileBytes(w, r, "file")
	if !ok {
		return
	}
	result, err := pdfops.AddAttachment(doc.data, r.FormValue("name"), data)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not add attachment: "+err.Error())
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

// handleAttachmentExtract streams one embedded file back to the browser as a
// download. The attachment name rides in as the "name" field.
func (s *Server) handleAttachmentExtract(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	name := r.FormValue("name")
	data, err := pdfops.ExtractAttachment(doc.data, name)
	if err != nil {
		httpError(w, http.StatusNotFound, "could not extract attachment: "+err.Error())
		return
	}
	sendDownload(w, name, "application/octet-stream", data)
}
