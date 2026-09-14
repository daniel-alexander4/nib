package server

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"nib/internal/pdfops"
	"nib/internal/tagwrite"
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

// tagTreeElementView is one element of the open document's existing structure tree on the wire —
// `PLAN-accessibility.md` P09.S06a.
type tagTreeElementView struct {
	ID       int        `json:"id"`
	Parent   int        `json:"parent"`
	Kids     []int      `json:"kids"`
	Kind     string     `json:"kind"`
	Standard string     `json:"standard"`
	Page     int        `json:"page"`
	Text     string     `json:"text"`
	Alt      string     `json:"alt"`
	HasAlt   bool       `json:"hasAlt"`
	Scope    string     `json:"scope"`
	Rect     [4]float64 `json:"rect"`
	PageBox  [4]float64 `json:"pageBox"`
}

// tagTreeResponse is the open document's existing structure tree.
type tagTreeResponse struct {
	Tagged        bool                 `json:"tagged"`
	Unaddressable int                  `json:"unaddressable"`
	Elements      []tagTreeElementView `json:"elements"`
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

// handleTagsTree reads the open document's existing structure tree for the Tags panel. It writes
// nothing; a document with no tree answers untagged.
func (s *Server) handleTagsTree(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("tags: recovered panic reading the tree: %v", rec)
			httpError(w, http.StatusUnprocessableEntity, "could not read the structure of this document")
		}
	}()
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	tree, err := pdfops.ReadStructure(s.docBytes(doc))
	if err != nil {
		httpError(w, http.StatusUnprocessableEntity, "could not read the structure: "+strings.TrimPrefix(err.Error(), "pdfops: "))
		return
	}
	out := tagTreeResponse{Tagged: tree.Tagged, Unaddressable: tree.Unaddressable, Elements: []tagTreeElementView{}}
	for _, e := range tree.Elements {
		// Kids is never nil: ReadStructure copies it into a fresh slice, and its own test holds that.
		out.Elements = append(out.Elements, tagTreeElementView{
			ID: e.ID, Parent: e.Parent, Kids: e.Kids, Kind: e.Kind, Standard: e.Standard, Page: e.Page,
			Text: e.Text, Alt: e.Alt, HasAlt: e.HasAlt, Scope: e.Scope, Rect: e.Rect, PageBox: e.PageBox,
		})
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
	// The request shape is the door's (`tagwrite.DecodeReview`), which `nib tag commit` reads too.
	reviews, err := tagwrite.DecodeReview(io.LimitReader(r.Body, maxTagReviewBytes))
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read the review")
		return
	}
	// Read ONCE: this is the state the undo entry records (commitMutation's contract).
	before := s.docBytes(doc)
	// **The refusal is the door's, not this handler's** — S06's acceptance, and reflow D11's rule for the
	// same reason: a UI that disables the button is not the door. `tagwrite.Commit` refuses a signed
	// document for this route and for `nib tag commit` alike (P10.S02).
	result, err := tagwrite.Commit(before, reviews)
	if tagWriteFailed(w, err, "could not write the structure") {
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
	// The door's request shape: an absent index appends, not "first" (`tagwrite.DecodeEdits`).
	edits, err := tagwrite.DecodeEdits(io.LimitReader(r.Body, maxTagReviewBytes))
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read the edits")
		return
	}
	before := s.docBytes(doc)
	// The same door as the commit's, and the one `nib tag edit` reaches: it refuses a signed document.
	result, err := tagwrite.Edit(before, edits)
	if tagWriteFailed(w, err, "could not edit the structure") {
		return
	}
	if err := s.commitMutation(doc, before, result, false); wroteCommitFailure(w, err) {
		return
	}
	writeJSON(w, s.docResponse(doc))
}

// tagWriteFailed answers a refused structure write and reports whether it did: a signed document or a
// stale review 409, a malformed one 400, anything else 422 under what (a result that did not validate is
// logged, not shown).
func tagWriteFailed(w http.ResponseWriter, err error, what string) bool {
	if err == nil {
		return false
	}
	reason := strings.TrimPrefix(err.Error(), "pdfops: ")
	switch {
	case errors.Is(err, tagwrite.ErrSigned), errors.Is(err, pdfops.ErrTagsStale):
		httpError(w, http.StatusConflict, reason)
	case errors.Is(err, pdfops.ErrTagsReview):
		httpError(w, http.StatusBadRequest, reason)
	case errors.Is(err, tagwrite.ErrInvalid):
		log.Printf("tags: %s: %v", what, err)
		httpError(w, http.StatusUnprocessableEntity, what)
	default:
		httpError(w, http.StatusUnprocessableEntity, what+": "+reason)
	}
	return true
}
