package server

import (
	"io"
	"log"
	"net/http"
	"strings"

	"nib/internal/objedit"
	"nib/internal/pdfops"
)

// The page objects' route (PLAN-image-edit P01.S02, ADR-219): remove pictures from the open document by the address
// a listing handed out. It goes through `internal/objedit`, which refuses a signed or ceremony document for this
// route and for `nib objects remove` alike. The listing route (`GET /api/page/objects`) is P01.S04's, built with the
// window that calls it.

// maxObjectEditBytes caps a removal request. One edit is under 200 bytes of JSON and a request carries at most 64
// (`pdfops`' ceiling); 64 KiB is five times that, and anything larger is not a request this route makes.
const maxObjectEditBytes = 64 << 10

// objectEditResponse is a removal's answer: the document as every install route answers it, and what was removed.
type objectEditResponse struct {
	docResponse
	Removed []pdfops.RemovedObject `json:"removed"`
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
	result, res, err := objedit.Remove(before, edits)
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
