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

// The page objects' routes (PLAN-image-edit P01.S02, S04 and P02.S03, ADR-219, ADR-221): list the pictures one page of
// the open document draws, each with its address, and remove or move pictures by that address. Both go through `internal/objedit`,
// which refuses a signed or ceremony document for these routes and for `nib objects` alike.

// maxObjectEditBytes caps an edit request. One edit is under 200 bytes of JSON and a request carries at most 64
// (`pdfops`' ceiling); 64 KiB is five times that, and anything larger is not a request this route makes.
const maxObjectEditBytes = 64 << 10

// objectEditResponse is an edit's answer: the document as every install route answers it, what was removed and what
// was moved.
type objectEditResponse struct {
	docResponse
	Removed  []pdfops.RemovedObject  `json:"removed"`
	Moved    []pdfops.MovedObject    `json:"moved"`
	Replaced []pdfops.ReplacedObject `json:"replaced"`
}

// maxObjectReplaceBytes caps a replace's upload: a 48-megapixel PNG measured 42 MB (PLAN-image-edit P03.S01), and the
// picture's own ceiling (pdfops.ReplaceMaxPixels) refuses what is larger in pixels whatever its bytes.
const maxObjectReplaceBytes = 128 << 20

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
	if !drawingsInWindow {
		got.Drawings, got.DrawingsUngrouped = []pdfops.PageObject{}, false
	}
	writeJSON(w, got)
}

// drawingsInWindow says the window is offered a page's drawings (PLAN-image-edit P03.S03). It is off because the
// grouping did not meet the threshold fixed before it was measured: `build/drawinggroup.sh` scored 0.893 of a fresh
// hand-labelled sample's drawings matched exactly (108 of 121, 16 pages, 2026-11-10, v2.60.0), against 0.90. The misses
// are forms ruled by lines that do not touch (one grid read as a dozen drawings) and a logo or a word drawn in parts.
// `nib objects` lists and edits drawings whatever this says; only the window's offer waits. Turn it on only with a run
// of the instrument at or over the threshold on a sample labelled after the change.
const drawingsInWindow = false

// handlePageObjectsEdit answers POST /api/page/objects: `{"edits": [{"kind": "delete" | "move", "page", "ordinal",
// "digest", "box"}]}` removes or moves those pictures (a move's box in displayed-page fractions) as one undoable
// install, answering the document with what was `removed` and `moved`. A refusal is 422 `{cause, error}`; an address
// that no longer names its picture — or a document another edit changed while this one was being written — is 409
// `object-changed`, the route's only 409 (ADR-219): the window reads the page again.
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
	// The one named exemption from wroteCommitFailure's answer (ADR-009): a lost update is answered 409 as it is
	// there, with the object-changed cause the window recovers from (ADR-222's review, W3).
	if err := s.commitMutation(doc, snapshotBase(before), result, false); wroteObjectChanged(w, err) || wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, objectEditResponse{docResponse: s.docResponse(doc), Removed: res.Removed, Moved: res.Moved, Replaced: res.Replaced})
}

// handlePageObjectsReplace answers POST /api/page/objects/replace: a multipart body of the new picture (`file`) and
// the edit (`edits`, the JSON `POST /api/page/objects` reads, holding one `replace`). The document's own refusal is
// asked first, so a signed document's upload is never imported; the picture is imported through the server's one
// import door, admitting only a raster Nib reads itself, one page, under replace's pixel ceiling; the door then
// lifts it into the page as one undoable install.
func (s *Server) handlePageObjectsReplace(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	cleanup, ok := parseMultipart(w, r, maxObjectReplaceBytes)
	if !ok {
		return
	}
	defer cleanup()
	raw := r.FormValue("edits")
	if len(raw) > maxObjectEditBytes {
		writeObjectRefusal(w, pdfops.RefuseObject(pdfops.CauseBadRequest, 0, -1, errors.New("the edit is longer than any edit this route reads")))
		return
	}
	edits, err := objedit.DecodeEdits(strings.NewReader(raw))
	if err != nil {
		writeObjectRefusal(w, err)
		return
	}
	if len(edits) != 1 || edits[0].Kind != pdfops.ObjectEditReplace {
		writeObjectRefusal(w, pdfops.RefuseObject(pdfops.CauseBadRequest, 0, -1, errors.New("a replacement names one picture to replace")))
		return
	}
	before := s.docBytes(doc)
	if err := objedit.Check(before); err != nil {
		writeObjectRefusal(w, err)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		httpError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer f.Close()
	got, ok := s.importOrRefuse(w, r.Context(), f, hdr.Size, hdr.Filename, pdfops.ReplacementImport("replace"))
	if !ok {
		return
	}
	edits[0].With = got.PDF
	result, res, err := objedit.Edit(before, edits)
	if err != nil {
		writeObjectRefusal(w, err)
		return
	}
	if err := s.commitMutation(doc, snapshotBase(before), result, false); wroteObjectChanged(w, err) || wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, objectEditResponse{docResponse: s.docResponse(doc), Removed: res.Removed, Moved: res.Moved, Replaced: res.Replaced})
}

// wroteObjectChanged answers a lost update — another install landing between this edit's read and its commit
// (`errDocChanged`) — as 409 `object-changed`: every address the edit was built on may be stale, and that cause is the
// one 409 the window recovers from (`reloadIfMoved`). Every other commit refusal is `wroteCommitFailure`'s.
func wroteObjectChanged(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, errDocChanged) {
		return false
	}
	writeObjectRefusal(w, pdfops.RefuseObject(pdfops.CauseObjectChanged, 0, -1, err))
	return true
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
