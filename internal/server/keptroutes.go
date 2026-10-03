package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"nib/internal/sign"
	"strconv"
)

// The routes of a copy kept when you signed (PLAN-returned-document P04.S02, ADR-073): the list, the removal, and the
// one route that hands the open document's kept copy to the dispute surface. Each reaches a file only through
// kept.go's door.

// handleKeptList answers GET /api/kept: every kept copy, newest first, from names and stats alone.
func (s *Server) handleKeptList(w http.ResponseWriter, r *http.Request) {
	kept, err := listKept()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not list the copies kept when you signed: "+err.Error())
		return
	}
	out := keptListResponse{Kept: []keptEntry{}}
	for _, k := range kept {
		out.Kept = append(out.Kept, k)
		out.TotalBytes += k.Size
	}
	writeJSON(w, out)
}

// keptRemoveRequest names the kept copy to remove — a bare name, never a path.
type keptRemoveRequest struct {
	Name string `json:"name"`
}

// keptRemoveResponse says whether a file was removed; false means it was already gone, which is not a failure.
type keptRemoveResponse struct {
	Removed bool `json:"removed"`
}

// handleKeptRemove answers POST /api/kept/remove: the repo's first route that deletes a user's file, so it takes a
// NAME, refuses anything the kept grammar does not accept (400), and removes only a regular file.
func (s *Server) handleKeptRemove(w http.ResponseWriter, r *http.Request) {
	var req keptRemoveRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	removed, err := removeKept(req.Name)
	if errors.Is(err, errNotAKeptName) || errors.Is(err, errNotAKeptCopy) {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not remove the kept copy: "+err.Error())
		return
	}
	writeJSON(w, keptRemoveResponse{Removed: removed})
}

// keptCopyFacts is what X-Nib-Kept-Copy says about the kept copy the route returns (ADR-072's shape).
type keptCopyFacts struct {
	Document string `json:"document"`
	KeptAt   string `json:"keptAt"`
	// Same is true when the open document IS the kept copy; Extends when it begins with it and is longer.
	Same    bool `json:"same,omitempty"`
	Extends bool `json:"extends,omitempty"`
}

// keptCopyRefusal is the 422 body: why the open document has no kept copy to hand over.
type keptCopyRefusal struct {
	// Cause is `no-signature` (nothing to match a kept copy against), `none-kept` (no kept copy is this document or
	// its beginning) or `unreadable` (the folder or a candidate could not be read).
	Cause string `json:"cause"`
}

// handleDocumentKeptCopy answers GET /api/document/kept-copy: the copy kept when you signed that the open document is
// or begins with, matched by keptCopyFor against the coverage ends of the document's own signatures — the status
// computed when it was opened, so no second parse. Pinned (ADR-004), registration checked in the same hold as the
// bytes, no name parameter: the client cannot choose the file. On demand only (D10). HEAD (which the GET pattern
// also matches) is the checklist's probe: the same match, the same status and headers, no body sent.
func (s *Server) handleDocumentKeptCopy(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	var data []byte
	var sig sign.Status
	if !s.snapshotRegistered(w, doc, func() { data, sig = doc.data, doc.sig }) {
		return
	}
	refuse := func(cause string) {
		// The cause rides in a header as well as the body, because the Simple Sign checklist asks with HEAD — it
		// needs the answer, never the bytes (P04.S03) — and a HEAD response carries no body.
		w.Header().Set("X-Nib-Kept-Copy-Cause", cause)
		writeJSONStatus(w, http.StatusUnprocessableEntity, keptCopyRefusal{Cause: cause})
	}
	var ends []int64
	for _, si := range sig.Signers {
		if si.CoverageEnd > 0 {
			ends = append(ends, si.CoverageEnd)
		}
	}
	if len(ends) == 0 {
		if !sign.HasSignatureBlob(data) { // the one signedness door (ADR-013's gates, /pending 456) — never the verdict's state
			refuse("no-signature")
			return
		}
		// Signed, but the verdict could not place its signatures (a refused library read, an exhausted sweep, a join
		// that disagreed): that is the document that came back hostile, so it is matched without them — never told
		// it carries no signature (the phase-close review).
	}
	k, b, err := keptCopyFor(data, ends, len(ends) > 0)
	if err != nil {
		refuse("unreadable")
		return
	}
	if b == nil {
		refuse("none-kept")
		return
	}
	same := len(b) == len(data)
	hdr, err := json.Marshal(keptCopyFacts{Document: k.Document, KeptAt: k.KeptAt, Same: same, Extends: !same})
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not describe the kept copy")
		return
	}
	w.Header().Set("X-Nib-Kept-Copy", string(hdr))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	_, _ = w.Write(b)
}
