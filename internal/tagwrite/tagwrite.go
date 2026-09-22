// Package tagwrite is the one door every USER-DIRECTED structure write goes through — committing a proposal
// or editing an existing tree (`PLAN-accessibility.md` P10.S02).
//
// **It is not the door for every structure write, and this comment said it was until `/pending 503`.** The
// operations that author structure as part of producing a document — `TagOCRLayer`, `AuthorTaggedForm`,
// the Markdown conversion's `tagMarkdown`, nib's own co-sign and ceremony pages (`TagAuthoredPages`,
// P02.S09), the n-up carry (`carryTagsThroughNUp`) — write a tree inside
// `pdfops` and never pass here, so this package's signed refusal does not reach them. What guards those is
// each one's own caller, not this door.
//
// Two surfaces write structure on a person's say-so: the Tags panel's routes (`internal/server/tags.go`) and `nib tag commit` /
// `nib tag edit` (`internal/cli/tag.go`). Each needs the same three rules — a signed document is refused,
// the written document must validate, and a request body reads the same way — and ADR-009 says a rule
// holding at more than one site is written once. `pdfops` cannot hold the refusal: `sign`'s own tests
// import `pdfops`, so `pdfops` importing `sign` is a cycle. This package imports both and holds all
// three; `tagdoor_test.go` (repo root) asserts nothing outside it calls the `pdfops` writers.
package tagwrite

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// ErrSigned is a write refused because the document carries a signature: structure changes the bytes
// every signature covers, so a signed document is refused rather than broken.
var ErrSigned = errors.New("tagwrite: the document is signed")

// ErrInvalid is a write whose result did not validate. Nothing is returned to write.
var ErrInvalid = errors.New("tagwrite: the written document did not validate")

// ErrMalformed is a request body that is not a review or a batch of edits at all.
var ErrMalformed = errors.New("tagwrite: the request could not be read")

// signedRefusal carries the sentence a signed refusal says, which names what was being attempted.
type signedRefusal string

func (s signedRefusal) Error() string      { return string(s) }
func (signedRefusal) Is(target error) bool { return target == ErrSigned }

// Commit writes a reviewed proposal into pdf. A signed document is ErrSigned, a result that does not
// validate ErrInvalid; the rest are CommitTags' own errors (ErrTagsStale, ErrTagsReview).
func Commit(pdf []byte, reviews []pdfops.TagReview) ([]byte, error) {
	if sign.HasSignatureBlob(pdf) {
		return nil, signedRefusal("this document is signed, and adding structure would change the bytes its signatures cover — tag a document before it is signed")
	}
	out, err := pdfops.CommitTags(pdf, reviews)
	if err != nil {
		return nil, err
	}
	return validated(out)
}

// Edit applies a batch of corrections to pdf's existing structure tree, as one write. Errors as Commit's,
// with EditStructure's own in place of CommitTags'.
func Edit(pdf []byte, edits []pdfops.StructureEdit) ([]byte, error) {
	if sign.HasSignatureBlob(pdf) {
		return nil, signedRefusal("this document is signed, and correcting its structure would change the bytes its signatures cover — correct it before it is signed")
	}
	out, err := pdfops.EditStructure(pdf, edits)
	if err != nil {
		return nil, err
	}
	return validated(out)
}

func validated(out []byte) ([]byte, error) {
	if err := pdfops.Validate(out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return out, nil
}

// DecodeReview reads a review: `{"elements": [{"id", "role", "ignore", "text"}]}`. The proposal's own JSON
// (`nib tag propose --json`, the propose route) is a review that keeps every role it proposed — the fields
// a review does not carry are ignored.
func DecodeReview(r io.Reader) ([]pdfops.TagReview, error) {
	var body struct {
		Elements []struct {
			ID     int    `json:"id"`
			Role   string `json:"role"`
			Ignore bool   `json:"ignore"`
			Text   string `json:"text"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	reviews := make([]pdfops.TagReview, len(body.Elements))
	for i, e := range body.Elements {
		reviews[i] = pdfops.TagReview{ID: e.ID, Role: e.Role, Ignore: e.Ignore, Text: e.Text}
	}
	return reviews, nil
}

// DecodeEdits reads a batch of edits: `{"edits": [{"kind", "element", "value", "parent", "index"}]}`. An
// absent index appends — Go's zero value would put a moved element first.
func DecodeEdits(r io.Reader) ([]pdfops.StructureEdit, error) {
	var body struct {
		Edits []struct {
			Kind    string `json:"kind"`
			Element int    `json:"element"`
			Value   string `json:"value"`
			Parent  int    `json:"parent"`
			Index   *int   `json:"index"`
		} `json:"edits"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	edits := make([]pdfops.StructureEdit, len(body.Edits))
	for i, e := range body.Edits {
		index := -1
		if e.Index != nil {
			index = *e.Index
		}
		edits[i] = pdfops.StructureEdit{Kind: e.Kind, Element: e.Element, Value: e.Value, Parent: e.Parent, Index: index}
	}
	return edits, nil
}
