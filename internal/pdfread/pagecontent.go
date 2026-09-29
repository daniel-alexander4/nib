package pdfread

import (
	"bytes"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
)

// PageContent is a page's content, decoded — pdfcpu's `PageContent` with the streams of a `/Contents` ARRAY
// joined at token boundaries (ADR-056, `PLAN-text-reflow.md` P05.S01).
//
// **Why not pdfcpu's own.** `XRefTable.PageContent` (model/xreftable.go:1882, v0.13.0) appends an array's
// decoded streams with nothing between them. ISO 32000-1 §7.8.2 lets a stream end on ANY token boundary, so a
// page whose first stream ends `(A) Tj` and whose second begins `ET` reads as the one operator `TjET` — the text
// is not shown and the text object never closes — and a first stream ending inside a `%` comment comments out
// the second's first line. Every reader then misreads the page, and every writer that replaces `/Contents`
// with the join (`setPageContent`'s callers) makes the misreading the document.
//
// **The join is byte-identical to pdfcpu's wherever that is already correct**: a `\n` goes between two streams
// only where the earlier ends in a regular byte (or the empty name `/`) and the later begins with one, or where
// the earlier ends inside a comment. Measured over the real-producer and veraPDF corpora at slice open: 0 of 77 joins needed one, so no
// document measured reads differently through this door than it did before it.
//
// A single stream (the common case) is pdfcpu's call unchanged. Each stream of an array is decoded through the
// same call, so decode errors, the strict-mode refusal and pdfcpu's tolerance of a corrupt flate tail are
// pdfcpu's; an element that is not a stream is refused, as pdfcpu refuses it (the message differs). The result is
// `model.ErrNoContent` when the page has no content, as pdfcpu's is.
//
// **What it does not repair**: a split inside a token — a string, a dictionary, an inline image — which the
// spec forbids and which no separator could mend.
func PageContent(ctx *model.Context, page types.Dict, pageNr int) ([]byte, error) {
	o, _ := page.Find("Contents")
	if o == nil {
		return nil, model.ErrNoContent
	}
	resolved, err := ctx.Dereference(o)
	if err != nil {
		return nil, err
	}
	arr, isArray := resolved.(types.Array)
	if !isArray {
		return ctx.PageContent(page, pageNr) //pagecontent:door
	}
	var out, prev []byte
	for _, e := range arr {
		if e == nil {
			continue
		}
		// pdfcpu refuses an element that is not a stream; handed one alone, its call would accept a nested array.
		if r, _ := ctx.Dereference(e); r != nil {
			if _, nested := r.(types.Array); nested {
				return nil, fmt.Errorf("page %d content: a /Contents array holds an array, not a stream", pageNr)
			}
		}
		b, err := ctx.PageContent(types.Dict{"Contents": e}, pageNr) //pagecontent:door
		if err == model.ErrNoContent {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			continue
		}
		// **The previous STREAM, never the join so far.** A stream begins on a token boundary, so its own
		// tokenization is the one that matters at its end; handing the accumulated join instead re-tokenized the
		// whole page at every join — quadratic, measured at 13.8 s for 1,000 streams against pdfcpu's 10 ms.
		if needsSeparator(prev, b) {
			out = append(out, '\n')
		}
		out = append(out, b...)
		prev = b
	}
	if len(out) == 0 {
		return nil, model.ErrNoContent
	}
	return out, nil
}

// needsSeparator reports whether appending next to prev would change how either is tokenized.
//
// Two ways: a regular byte meeting a regular byte fuses the tokens either side of the join — and `/` counts as one
// on the left, because a stream may end on the empty name `/`, which a following `F1` would turn into `/F1` — and a
// comment runs to the end of the LINE, not the stream, so an unterminated one swallows what follows. Everything
// else — whitespace, or any other delimiter on either side — already ends the earlier token. A split INSIDE a
// token is forbidden and not considered.
func needsSeparator(prev, next []byte) bool {
	if len(prev) == 0 || len(next) == 0 {
		return false
	}
	last := prev[len(prev)-1]
	// An end-of-line ends every token, a comment included. This and the `%` test below are short-cuts that spare the
	// tokenize: removing either leaves every answer unchanged (probed), so neither is load-bearing.
	if last == '\n' || last == '\r' {
		return false
	}
	if (regular(last) || last == '/') && regular(next[0]) {
		return true
	}
	// A `%` anywhere is the cheap precondition; only a tokenizer can say whether it opens a comment rather than
	// sitting inside a string, and only the LAST token matters.
	if bytes.IndexByte(prev, '%') < 0 {
		return false
	}
	toks := contentstream.Tokenize(prev)
	if len(toks) == 0 {
		return false
	}
	t := toks[len(toks)-1]
	return t.Kind == contentstream.Whitespace && bytes.IndexByte(t.Bytes(prev), '%') >= 0
}

// regular is ISO 32000-1 §7.2.2's regular character: neither white-space nor a delimiter.
func regular(b byte) bool {
	switch b {
	case 0, '\t', '\n', '\f', '\r', ' ', '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}
