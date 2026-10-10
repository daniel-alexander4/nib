package pdfops

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// ErrPageNotInDocument is the refusal of a request that names a page the document does not have. It is a fact
// about the request against THESE bytes, not a failed operation, so the server answers it 422 rather than 500.
//
// **One door for every page-taking operation** (`/pending 819`, `/pending 823`, ADR-009). Redaction refused an
// unmatched key first; rotate, delete and crop passed pdfcpu's page selection, which drops a page past the end
// without a word, so "rotate page 5" of a one-page document — and "delete page 5", a removal — reported success
// and changed nothing, and a stamp keyed page 0 was moved onto page 1. A page number goes through
// pageInDocument; a selection through selectionInDocument.
var ErrPageNotInDocument = errors.New("pdfops: that is not a page of this document")

// PageRangeError names what was refused and the page count it was checked against.
type PageRangeError struct {
	Page  int    // the page number named; for a selection term, the number it spells, or 0 (`even`, `l-3`)
	Term  string // the selection term that named no page; empty when a bare page number was refused
	Pages int    // how many pages the document has
}

func (e *PageRangeError) Error() string {
	if e.Term != "" {
		return fmt.Sprintf("%v: %q names no page, and the document has %d", ErrPageNotInDocument, e.Term, e.Pages)
	}
	return fmt.Sprintf("%v: page %d, and the document has %d", ErrPageNotInDocument, e.Page, e.Pages)
}

// Unwrap makes errors.Is(err, ErrPageNotInDocument) hold.
func (e *PageRangeError) Unwrap() error { return ErrPageNotInDocument }

// pageInDocument refuses a page number outside 1..pages.
func pageInDocument(page, pages int) error {
	if page < 1 || page > pages {
		return &PageRangeError{Page: page, Pages: pages}
	}
	return nil
}

// pageRefusedAs is the door's refusal in the words of the operation that met it — a bookmark, a split
// range — for the two inputs a user types and reads the answer to as it is (`/pending 834`). It is still
// the door's: errors.Is and errors.As find the PageRangeError under it.
type pageRefusedAs struct {
	words string
	err   error
}

func (e *pageRefusedAs) Error() string { return e.words }
func (e *pageRefusedAs) Unwrap() error { return e.err }

// selectionInDocument refuses a page selection that names a page the document does not have.
//
// **The unit is the TERM, not the page.** pdfcpu clips a range at the document's end, and that is kept: `3-`
// and `2-9` are how a user says "to the end", and refusing the pages past it would refuse a selection that
// meant something. What is refused is a term that names NO page — `5` of a one-page document, `7-9` of six,
// `even` of one — because nothing it meant can happen; and a term naming page 0. A negation (`!3`, `n3`)
// removes rather than names, so it is exempt; and a selection whose terms all pass but which nets to nothing
// (`1,!1`) is refused whole, since the operation would again report success and do nothing. Empty (nil) means "every page" to the doors that
// allow it and is not this function's question. A term pdfcpu cannot parse is left to the parse that follows,
// so its refusal keeps its own wording.
func selectionInDocument(sel []string, pages int) error {
	if len(sel) == 0 {
		return nil
	}
	for _, term := range sel {
		t := strings.TrimSpace(term)
		if t == "" || t[0] == '!' || t[0] == 'n' {
			continue
		}
		named, err := api.PagesForPageSelection(pages, []string{t}, false, false)
		if err != nil {
			continue
		}
		if !namesAPage(named, pages) {
			n, _ := strconv.Atoi(t)
			return &PageRangeError{Page: n, Term: t, Pages: pages}
		}
	}
	if all, err := api.PagesForPageSelection(pages, sel, false, false); err == nil && !namesAPage(all, pages) {
		return &PageRangeError{Term: strings.Join(sel, ","), Pages: pages}
	}
	return nil
}

// namesAPage reports whether a pdfcpu page set selects a page of the document and nothing below it. A negated
// term leaves its pages in the set with the value false; and pdfcpu takes `0` and `0-` at their word, putting a
// page 0 in the set, which no document has and no clipping excuses.
func namesAPage(set map[int]bool, pages int) bool {
	some := false
	for k, on := range set {
		if !on {
			continue
		}
		if k < 1 {
			return false
		}
		if k <= pages {
			some = true
		}
	}
	return some
}
