package server

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"nib/internal/objedit"
	"nib/internal/pdfops"
)

// The page objects' routes (PLAN-image-edit P01.S02 and S04, ADR-219): list the pictures one page of the open
// document draws, each with its address, and remove pictures by that address. Both go through `internal/objedit`,
// which refuses a signed or ceremony document for these routes and for `nib objects` alike.

// maxObjectEditBytes caps a removal request. One edit is under 200 bytes of JSON and a request carries at most 64
// (`pdfops`' ceiling); 64 KiB is five times that, and anything larger is not a request this route makes.
const maxObjectEditBytes = 64 << 10

// objectEditResponse is a removal's answer: the document as every install route answers it, and what was removed.
type objectEditResponse struct {
	docResponse
	Removed []pdfops.RemovedObject `json:"removed"`
}

// handlePageObjects answers GET /api/page/objects?page=N: the pictures that page draws, from the server's own bytes,
// for the window's Edit Objects (P01.S04). It writes nothing.
func (s *Server) handlePageObjects(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("objects: recovered panic reading a page's pictures: %v", rec)
			writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"cause": "page-unreadable", "error": "could not read the pictures on this page"})
		}
	}()
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		httpError(w, http.StatusBadRequest, "page must be a page number, 1 or more")
		return
	}
	got, err := objedit.List(s.docBytes(doc), page)
	var outOfRange *pdfops.PageRangeError
	if errors.As(err, &outOfRange) {
		httpError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "pdfops: "))
		return
	}
	if err != nil {
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{"cause": "page-unreadable",
			"error": "could not read the pictures on this page: " + strings.TrimPrefix(err.Error(), "pdfops: ")})
		return
	}
	writeJSON(w, got)
}

// handlePageObjectsEdit answers POST /api/page/objects: `{"edits": [{"kind": "delete", "page", "ordinal", "digest"}]}`
// removes those pictures from the open document as one undoable install. A refusal is 422 `{cause, error}`; an
// address that no longer names its picture is 409 `object-changed` (ADR-146: the window reads the page again).
func (s *Server) handlePageObjectsEdit(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("objects: recovered panic removing a picture: %v", rec)
			writeObjectRefusal(w, pdfops.RefuseObject(pdfops.CauseNotRewritable, 0, -1, nil))
		}
	}()
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	edits, err := objedit.DecodeEdits(io.LimitReader(r.Body, maxObjectEditBytes))
	if err != nil {
		writeObjectRefusal(w, err)
		return
	}
	// Read ONCE: this is the state the undo entry records, and the base the commit checks (commitMutation's
	// contract) — the route edits the server's own bytes, so no client revision is echoed (lostupdate_test.go).
	before := s.docBytes(doc)
	result, res, err := objedit.Edit(before, edits)
	if err != nil {
		writeObjectRefusal(w, err)
		return
	}
	if err := s.commitMutation(doc, snapshotBase(before), result, false); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, objectEditResponse{docResponse: s.docResponse(doc), Removed: res.Removed})
}

// writeObjectRefusal answers a refused object edit: its cause and its sentence, 409 for a stale address and 422
// for every other cause (ADR-072). An error that is not a refusal is the document's not surviving the rewrite.
func writeObjectRefusal(w http.ResponseWriter, err error) {
	ref, ok := objedit.Refusal(err)
	if !ok {
		ref = pdfops.RefuseObject(pdfops.CauseNotRewritable, 0, -1, err)
	}
	code := http.StatusUnprocessableEntity
	switch ref.Cause {
	case pdfops.CauseObjectChanged:
		code = http.StatusConflict
	case pdfops.CauseBadRequest:
		code = http.StatusBadRequest
	}
	writeJSONStatus(w, code, map[string]string{"cause": ref.Cause, "error": strings.TrimPrefix(ref.Error(), "pdfops: ")})
}
