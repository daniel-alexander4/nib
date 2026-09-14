package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// The tag review routes — `PLAN-accessibility.md` P08.S06b.
//
// Propose is a GET: it reads the document and writes nothing, as D5 and law 3 require of a proposal.
// Commit takes the review the Tags card built — every element, in the reviewer's order, with the role
// chosen or marked ignored — and installs the result through `commitMutation`, so it is undoable like
// every other edit.

// tagElementView is one proposed element on the wire.
type tagElementView struct {
	ID      int        `json:"id"`
	Role    string     `json:"role"`
	Page    int        `json:"page"`
	Text    string     `json:"text"`
	Marker  string     `json:"marker,omitempty"`
	List    int        `json:"list"`
	Rect    [4]float64 `json:"rect"`
	PageBox [4]float64 `json:"pageBox"`
}

// tagPageView is a page the grouping could not read, and why.
type tagPageView struct {
	Page   int    `json:"page"`
	Reason string `json:"reason"`
}

// tagProposalResponse is the proposed structure of the open document.
type tagProposalResponse struct {
	Elements    []tagElementView `json:"elements"`
	Unsupported []tagPageView    `json:"unsupported"`
	NoText      []int            `json:"noText"`
}

// maxTagReviewBytes bounds a review: one short entry per element, and a long document has thousands.
const maxTagReviewBytes = 8 << 20

// handleTagsPropose proposes a structure for the open document.
func (s *Server) handleTagsPropose(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("tags: recovered panic proposing: %v", rec)
			httpError(w, http.StatusUnprocessableEntity, "could not propose a structure for this document")
		}
	}()
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	prop, err := pdfops.ProposeTags(s.docBytes(doc))
	if err != nil {
		httpError(w, http.StatusUnprocessableEntity, "could not propose a structure: "+strings.TrimPrefix(err.Error(), "pdfops: "))
		return
	}
	out := tagProposalResponse{Elements: []tagElementView{}, Unsupported: []tagPageView{}, NoText: prop.NoText}
	if out.NoText == nil {
		out.NoText = []int{}
	}
	for _, e := range prop.Elements {
		out.Elements = append(out.Elements, tagElementView{
			ID: e.ID, Role: e.Role, Page: e.Page, Text: e.Text, Marker: e.Marker, List: e.List,
			Rect: e.Rect, PageBox: e.PageBox,
		})
	}
	for _, n := range prop.Unsupported {
		out.Unsupported = append(out.Unsupported, tagPageView{Page: n.Page, Reason: n.Reason})
	}
	writeJSON(w, out)
}

// handleTagsCommit writes the reviewed structure into the open document.
func (s *Server) handleTagsCommit(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("tags: recovered panic committing: %v", rec)
			httpError(w, http.StatusUnprocessableEntity, "could not write the structure")
		}
	}()
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// The request shape is anonymous: the server is its only reader, and a named type would be a
	// published shape the client never reads.
	var body struct {
		Elements []struct {
			ID     int    `json:"id"`
			Role   string `json:"role"`
			Ignore bool   `json:"ignore"`
			Text   string `json:"text"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxTagReviewBytes)).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "could not read the review")
		return
	}
	// Read ONCE: this is the state the undo entry records (commitMutation's contract).
	before := s.docBytes(doc)
	// **The refusal lives here, at the server door** — S06's acceptance, and reflow D11's rule for the
	// same reason: a UI that disables the button is not the door. Structure changes the bytes every
	// signature covers, so a signed document is refused rather than broken.
	if sign.HasSignatureBlob(before) {
		httpError(w, http.StatusConflict, "this document is signed, and adding structure would change the bytes its signatures cover — tag a document before it is signed")
		return
	}
	reviews := make([]pdfops.TagReview, len(body.Elements))
	for i, e := range body.Elements {
		reviews[i] = pdfops.TagReview{ID: e.ID, Role: e.Role, Ignore: e.Ignore, Text: e.Text}
	}
	result, err := pdfops.CommitTags(before, reviews)
	if err != nil {
		reason := strings.TrimPrefix(err.Error(), "pdfops: ")
		switch {
		case errors.Is(err, pdfops.ErrTagsStale):
			httpError(w, http.StatusConflict, reason)
		case errors.Is(err, pdfops.ErrTagsReview):
			httpError(w, http.StatusBadRequest, reason)
		default:
			httpError(w, http.StatusUnprocessableEntity, "could not write the structure: "+reason)
		}
		return
	}
	if verr := pdfops.Validate(result); verr != nil {
		log.Printf("tags: committed output failed validation: %v", verr)
		httpError(w, http.StatusUnprocessableEntity, "could not write the structure")
		return
	}
	if err := s.commitMutation(doc, before, result, false); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, s.docResponse(doc))
}

// handleTagsEdit applies a batch of corrections to the open document's existing structure tree —
// `PLAN-accessibility.md` P09.S04. One batch is one commit, so one undo takes the whole batch back.
func (s *Server) handleTagsEdit(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("tags: recovered panic editing: %v", rec)
			httpError(w, http.StatusUnprocessableEntity, "could not edit the structure")
		}
	}()
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// Anonymous for the reason the commit's request is: the server is its only reader.
	var body struct {
		Edits []struct {
			Kind    string `json:"kind"`
			Element int    `json:"element"`
			Value   string `json:"value"`
			Parent  int    `json:"parent"`
			// Index is a pointer so an absent position means "at the end", not "first".
			Index *int `json:"index"`
		} `json:"edits"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxTagReviewBytes)).Decode(&body); err != nil {
		httpError(w, http.StatusBadRequest, "could not read the edits")
		return
	}
	before := s.docBytes(doc)
	// The same door, for the same reason, as the commit: a correction changes the bytes every signature
	// covers.
	if sign.HasSignatureBlob(before) {
		httpError(w, http.StatusConflict, "this document is signed, and correcting its structure would change the bytes its signatures cover — correct it before it is signed")
		return
	}
	edits := make([]pdfops.StructureEdit, len(body.Edits))
	for i, e := range body.Edits {
		index := -1
		if e.Index != nil {
			index = *e.Index
		}
		edits[i] = pdfops.StructureEdit{Kind: e.Kind, Element: e.Element, Value: e.Value, Parent: e.Parent, Index: index}
	}
	result, err := pdfops.EditStructure(before, edits)
	if err != nil {
		reason := strings.TrimPrefix(err.Error(), "pdfops: ")
		switch {
		case errors.Is(err, pdfops.ErrTagsStale):
			httpError(w, http.StatusConflict, reason)
		case errors.Is(err, pdfops.ErrTagsReview):
			httpError(w, http.StatusBadRequest, reason)
		default:
			httpError(w, http.StatusUnprocessableEntity, "could not edit the structure: "+reason)
		}
		return
	}
	if verr := pdfops.Validate(result); verr != nil {
		log.Printf("tags: edited output failed validation: %v", verr)
		httpError(w, http.StatusUnprocessableEntity, "could not edit the structure")
		return
	}
	if err := s.commitMutation(doc, before, result, false); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, s.docResponse(doc))
}
