package server

import (
	"errors"
	"net/http"

	"nib/internal/pdfops"
)

// handleScan reports the active and hidden content in the current document
// (auto-run hooks, JavaScript, risky actions, attachments, layers, metadata).
// It is read-only — it never alters the document — so it is a plain GET.
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	rep, err := pdfops.Scan(s.docBytes(doc))
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
	doc, ok := s.resolveDoc(w, r)
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
	var result []byte
	var err error
	switch r.FormValue("method") {
	case "strip":
		result, err = pdfops.StripActive(before)
	case "safe":
		result, err = pdfops.RemoveFilesAndMedia(before)
	case "metadata":
		result, err = pdfops.StripMetadata(before)
	default:
		httpError(w, http.StatusBadRequest, "unknown method")
		return
	}
	// A removal failure, or a result that won't validate, is reported as
	// not-ok (not an error) so the UI can recommend the next method down. The
	// open document is deliberately left as-is in both cases.
	if err != nil || pdfops.Validate(result) != nil {
		writeJSON(w, sanitizeResponse{docResponse: s.docResponse(doc), Ok: false})
		return
	}

	residual, _ := pdfops.Scan(result)
	if err := s.commitMutation(doc, before, result); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, sanitizeResponse{docResponse: s.docResponse(doc), Ok: true, Residual: residual})
}

// decryptResponse reports a remove-password attempt: the usual document metadata,
// whether it succeeded, and — when it didn't — why ("password" = wrong or missing
// password, so the UI reprompts; "plain" = the document wasn't protected).
type decryptResponse struct {
	docResponse
	Ok     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// handleEncrypt password-protects the posted document and returns the encrypted
// bytes as a download — it deliberately does NOT touch the working copy. Encrypted
// bytes can't be re-rendered without the password (the editor would land on the
// unlock prompt), so protection is a separate, signature-free export: the open
// document is left untouched. Like the other export handlers (finalize/bake) it
// operates on the uploaded "pdf" part — the client's current, overlay-baked state —
// not the server's committed copy, so unbaked edits are included. The password
// arrives in the same multipart form and must be non-empty.
func (s *Server) handleEncrypt(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	password := r.FormValue("password")
	if password == "" {
		httpError(w, http.StatusBadRequest, "a password is required")
		return
	}
	result, err := pdfops.Encrypt(pdfBytes, password)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not protect the document")
		return
	}
	sendDownload(w, "protected.pdf", "application/pdf", result)
}

// handleDecrypt removes password protection — an open password and/or owner
// restriction flags — from the current document, replacing the working copy with
// the decrypted bytes. The password arrives in the request body; an empty one
// still drops owner-only restrictions. Only the supplied password is tried (no
// cracking), so a wrong/missing one returns reason "password" and an already
// unprotected document returns reason "plain", both leaving the document as-is.
func (s *Server) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
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
	result, err := pdfops.RemovePassword(before, r.FormValue("password"))
	if err != nil {
		switch {
		case errors.Is(err, pdfops.ErrWrongPassword):
			writeJSON(w, decryptResponse{docResponse: s.docResponse(doc), Reason: "password"})
		case errors.Is(err, pdfops.ErrNotEncrypted):
			writeJSON(w, decryptResponse{docResponse: s.docResponse(doc), Reason: "plain"})
		default:
			httpError(w, http.StatusInternalServerError, "could not remove protection")
		}
		return
	}
	if err := s.commitMutation(doc, before, result); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, decryptResponse{docResponse: s.docResponse(doc), Ok: true})
}
