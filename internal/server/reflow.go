package server

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// Reflow — `PLAN-text-reflow.md` P06.S05. Two routes: the page's paragraphs, and one paragraph re-set as new text.

type paragraphsResponse struct {
	Paragraphs []pdfops.Paragraph `json:"paragraphs"`
}

// handleParagraphs lists a page's paragraphs for the reflow editor, each with the cause it cannot be reflowed, if any.
func (s *Server) handleParagraphs(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "page must be a number")
		return
	}
	data := s.docBytes(doc)
	paras, err := pdfops.Paragraphs(data, page)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read the page's paragraphs: "+err.Error())
		return
	}
	// A signed document lists every paragraph as refused, so the editor says why before the user types (D11).
	if sign.HasSignatureBlob(data) {
		for i := range paras {
			paras[i].Refusal = pdfops.ReflowCauseSigned
		}
	}
	writeJSON(w, paragraphsResponse{Paragraphs: paras})
}

// maxReflowFormBytes bounds a reflow request: four short fields, the longest a paragraph's text twice over.
const maxReflowFormBytes = 1 << 20

type reflowResponse struct {
	docResponse
	// Ok is whether the paragraph was re-set. When it was not, the Refusal's Cause names why (law 3) — "" means the text
	// was the paragraph's own and nothing needed doing — Below names the paragraph below that could not move with it, and
	// the document is untouched.
	Ok bool `json:"ok"`
	pdfops.Refusal
}

// handleReflow re-sets one paragraph as new text and commits the result. The request names the paragraph by page and
// index AND by the text it read there (`original`): a paragraph that no longer reads that way is 409, never a rewrite of
// whatever the index names now (ADR-001). A signed document is refused here, at the server door (D11), whatever the UI
// allowed.
func (s *Server) handleReflow(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	cleanup, ok := parseMultipart(w, r, maxReflowFormBytes)
	if !ok {
		return
	}
	defer cleanup()
	page, perr := strconv.Atoi(r.FormValue("page"))
	index, ierr := strconv.Atoi(r.FormValue("paragraph"))
	if perr != nil || ierr != nil {
		httpError(w, http.StatusBadRequest, "page and paragraph must be numbers")
		return
	}
	// Read ONCE: the undo entry must record the bytes the reflow ran on (see handleSanitize).
	before := s.docBytes(doc)
	// D11, at the server door — whatever the UI allowed. tagwrite's predicate: one door for "is this signed".
	if sign.HasSignatureBlob(before) {
		writeJSON(w, reflowResponse{docResponse: s.docResponse(doc), Ok: false, Refusal: pdfops.Refusal{Cause: pdfops.ReflowCauseSigned}})
		return
	}
	result, refusal, err := pdfops.ReflowParagraph(before, page, index, r.FormValue("original"), r.FormValue("text"))
	switch {
	case errors.Is(err, pdfops.ErrReflowStale):
		httpError(w, http.StatusConflict, "that paragraph has changed since it was read — read the page again")
		return
	case err != nil:
		httpError(w, http.StatusBadRequest, "could not reflow the paragraph: "+err.Error())
		return
	case result == nil:
		writeJSON(w, reflowResponse{docResponse: s.docResponse(doc), Ok: false, Refusal: refusal})
		return
	}
	if err := pdfops.Validate(result); err != nil {
		// Nib has no telemetry, so this line is the only record of WHY the rewrite did not check out.
		log.Printf("reflow: page %d paragraph %d: the rewritten page did not validate: %v", page, index, err)
		writeJSON(w, reflowResponse{docResponse: s.docResponse(doc), Ok: false, Refusal: pdfops.Refusal{Cause: pdfops.ReflowCauseInvalidOutput}})
		return
	}
	if err := s.commitMutation(doc, snapshotBase(before), result, false); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, reflowResponse{docResponse: s.docResponse(doc), Ok: true})
}
