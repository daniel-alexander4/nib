// Package objedit is the one door an edit of a picture a page draws goes through (PLAN-image-edit D5, ADR-219).
//
// `pdfops.EditPageObjects` is the content writer, and it does not hold the document's refusals: `pdfops` cannot import
// `sign` (`sign`'s tests import `pdfops`), and a ceremony record is refused beside the signature so both document
// refusals live in one place — a signed document and a document carrying a ceremony record are refused HERE, before any read of the page. The server's routes and `nib objects`
// call only this package; `TestEveryPageObjectEditRoutesThroughTheDoor` (repo root) fails anything else that calls
// the writer.
package objedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// documentRefusal is the refusal the document itself earns, whatever picture is named: a signature, or a ceremony
// record (D5's pins). It does NOT ask whether a ceremony is live: a convened, unsigned document is refused cold,
// because every counterpart's arrival gate recomputes its digest against the record's (`ceremony.CheckDocument`) and
// an edit would read as substitution.
//
// **The record is looked for through `pdfops.CeremonyRecord`, not `ceremony.Extract`** (code-review 2026-11-09):
// Extract answers `ErrNoRecord` for a record entry that is THERE and will not read (no file specification, a stream
// that does not decode, one past the size cap), which is a convened document all the same. Only "no entry at all"
// lets an edit through; an entry, two entries, an embedded-files list nib could not read in full, and any other
// failure to look refuse.
func documentRefusal(pdf []byte) *pdfops.ObjectRefusal {
	if sign.HasSignatureBlob(pdf) {
		return pdfops.RefuseObject(pdfops.CauseSigned, 0, -1, nil)
	}
	switch rec, err := pdfops.CeremonyRecord(pdf); {
	case err != nil:
		ref := pdfops.RefuseObject(pdfops.CauseCeremony, 0, -1, err)
		ref.Detail = "Nib could not read whether it carries a ceremony record."
		return ref
	case rec != nil:
		return pdfops.RefuseObject(pdfops.CauseCeremony, 0, -1, nil)
	}
	return nil
}

// List reads the pictures page of pdf draws, as `pdfops.ReadPageObjects` does, with the document's own refusal laid
// over every picture: on a signed or ceremony document none is editable or movable, and Why and MoveWhy say so.
func List(pdf []byte, page int) (pdfops.PageObjects, error) {
	objs, err := pdfops.ReadPageObjects(pdf, page)
	if err != nil {
		return objs, err
	}
	if ref := documentRefusal(pdf); ref != nil {
		for i := range objs.Objects {
			objs.Objects[i].Editable, objs.Objects[i].Why = false, ref.Cause
			objs.Objects[i].Movable, objs.Objects[i].MoveWhy = false, ref.Cause
		}
	}
	return objs, nil
}

// Edit applies edits — removals and moves — to pdf as one rewrite. A signed or ceremony document is refused before
// anything is read; the rest are `pdfops.EditPageObjects`' refusals, every one an `*pdfops.ObjectRefusal`.
func Edit(pdf []byte, edits []pdfops.ObjectEdit) ([]byte, pdfops.ObjectEditResult, error) {
	if ref := documentRefusal(pdf); ref != nil {
		return nil, pdfops.ObjectEditResult{}, ref
	}
	out, res, err := pdfops.EditPageObjects(pdf, edits)
	if err != nil {
		return nil, res, err
	}
	if err := pdfops.Validate(out); err != nil {
		return nil, pdfops.ObjectEditResult{}, pdfops.RefuseObject(pdfops.CauseNotRewritable, 0, -1, err)
	}
	return out, res, nil
}

// DecodeEdits reads a request: `{"edits": [{"kind": "delete" | "move", "page", "ordinal", "digest", "box"}]}`, the
// box a move's place in displayed-page fractions (the window's Edit Objects sends it, PLAN-image-edit P02.S03). A body
// that is not that shape is a `bad-request` refusal; whether a box fits its kind is the door's to say.
func DecodeEdits(r io.Reader) ([]pdfops.ObjectEdit, error) {
	var body struct {
		Edits []routeEdit `json:"edits"`
	}
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return nil, pdfops.RefuseObject(pdfops.CauseBadRequest, 0, -1, fmt.Errorf("%v", err))
	}
	if dec.More() {
		return nil, pdfops.RefuseObject(pdfops.CauseBadRequest, 0, -1, errors.New("more follows the request's object"))
	}
	out := make([]pdfops.ObjectEdit, len(body.Edits))
	for i, e := range body.Edits {
		out[i] = pdfops.ObjectEdit{Kind: e.Kind, Page: e.Page, Ordinal: e.Ordinal, Digest: e.Digest}
		if e.Box != nil {
			// A slice, not `pdfops.MapRect`: Go drops a fifth number into a fixed array and pads a short one with zeros.
			if len(e.Box) != 4 {
				return nil, pdfops.RefuseObject(pdfops.CauseBadRequest, e.Page, e.Ordinal,
					fmt.Errorf("a box is four numbers, left, top, right and bottom; this one has %d", len(e.Box)))
			}
			out[i].Box = &pdfops.MapRect{e.Box[0], e.Box[1], e.Box[2], e.Box[3]}
		}
	}
	return out, nil
}

// routeEdit is an edit as the route reads it.
type routeEdit struct {
	Kind    string    `json:"kind"`
	Page    int       `json:"page"`
	Ordinal int       `json:"ordinal"`
	Digest  string    `json:"digest"`
	Box     []float64 `json:"box"`
}

// Refusal is err's refusal, when it is one.
func Refusal(err error) (*pdfops.ObjectRefusal, bool) {
	var ref *pdfops.ObjectRefusal
	ok := errors.As(err, &ref)
	return ref, ok
}
