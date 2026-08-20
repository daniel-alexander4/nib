package server

import (
	"net/http"

	"nib/internal/pdfops"
)

// attachmentsResponse lists the document-level embedded files.
type attachmentsResponse struct {
	Attachments []pdfops.AttachmentInfo `json:"attachments"`
}

// handleAttachmentsList reports the embedded files in the current document. It is
// read-only, so it is a plain GET.
func (s *Server) handleAttachmentsList(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	aa, err := pdfops.Attachments(s.docBytes(doc))
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
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	data, ok := formFileBytes(w, r, "file")
	if !ok {
		return
	}
	// Read ONCE, and this is the state the undo entry records.
	//
	// commitMutation's contract: "Callers pass the input they actually operated on … so
	// undo restores precisely the pre-op document." Calling docBytes twice — as the
	// operation's input and again as the commit's — lets a concurrent mutation land in
	// between, so the undo entry records the NEW bytes as the state to return to and a
	// later undo restores a document that never existed.
	before := s.docBytes(doc)
	result, err := pdfops.AddAttachment(before, r.FormValue("name"), data)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not add attachment: "+err.Error())
		return
	}
	if !s.commitMutation(doc, before, result) {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	writeJSON(w, s.docResponse(doc))
}

// handleAttachmentExtract streams one embedded file back to the browser as a
// download. The attachment name rides in as the "name" field.
func (s *Server) handleAttachmentExtract(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// Through parseMultipart, not a bare FormValue. The client posts a FormData
	// here, so FormValue would trigger ParseMultipartForm implicitly — with no size
	// cap on the body, and with the parts it spills to temp files never removed,
	// which is exactly the leak parseMultipart's doc comment says it exists to
	// prevent ("skipping the cleanup leaks disk until the process exits").
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	name := r.FormValue("name")
	data, err := pdfops.ExtractAttachment(s.docBytes(doc), name)
	if err != nil {
		httpError(w, http.StatusNotFound, "could not extract attachment: "+err.Error())
		return
	}
	sendDownload(w, name, "application/octet-stream", data)
}
