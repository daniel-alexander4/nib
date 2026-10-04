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
//
// **Bounded at `MaxPageContentBytes` over the whole page** (`/pending 748`). pdfcpu caps each stream's decode at
// 512 MiB and nothing caps the join, and an array may name one stream any number of times: measured, a 199 KB
// file whose `/Contents` named one 200 MiB stream six times read as 1.2 GiB here (3.6 GiB peak heap) and the
// checker ran 13.7 s over it, linear in the count. Past the bound the page is refused (ErrDecodeLimit), never
// read short. Each element is still pdfcpu's own decode, so the work done before the refusal is at most the
// bound plus one stream's 512 MiB.
func PageContent(ctx *model.Context, page types.Dict, pageNr int) ([]byte, error) {
	return pageContent(ctx, page, pageNr, true)
}

// PageContentAsPdfcpu is `PageContent` joined as pdfcpu joins — the streams of a `/Contents` array appended
// with nothing between them — and bounded as `PageContent` is. Its one caller is `ContentDigest`'s rule 4
// (ADR-084), which hashed pdfcpu's join and is kept to check the records written under it; keeping pdfcpu's join
// there does not also mean keeping its unbounded one. (The checker read it until /pending 719 measured that
// veraPDF separates a `/Contents` array's streams, as `PageContent` does.)
func PageContentAsPdfcpu(ctx *model.Context, page types.Dict, pageNr int) ([]byte, error) {
	return pageContent(ctx, page, pageNr, false)
}

func pageContent(ctx *model.Context, page types.Dict, pageNr int, separate bool) ([]byte, error) {
	out, _, err := joinContent(ctx, page, pageNr, separate)
	return out, err
}

// joinContent is pageContent, also reporting — for an array — the index of every element a separator was put in
// front of, which is what SeparateContents writes into the page.
//
// **Each distinct stream is decoded once per page** (`/pending 728`). An array may name one stream any number of
// times, and the byte bound cannot see the cost when the stream decodes to little or nothing: measured, a page
// naming one EMPTY flate stream 100,000 times — a file of about 1 KB, the array compressed in an object stream —
// took 5.5 s and allocated 3.9 GB here while producing no content at all, linear in the count (each decode sets up
// a fresh inflater). A repeat reuses the first decode; what it contributes to the join, and so the bound, is
// unchanged.
func joinContent(ctx *model.Context, page types.Dict, pageNr int, separate bool) ([]byte, []int, error) {
	o, _ := page.Find("Contents")
	if o == nil {
		return nil, nil, model.ErrNoContent
	}
	resolved, err := ctx.Dereference(o)
	if err != nil {
		return nil, nil, err
	}
	arr, isArray := resolved.(types.Array)
	if !isArray {
		b, err := ctx.PageContent(page, pageNr) //pagecontent:door
		if err == nil && len(b) > MaxPageContentBytes {
			return nil, nil, pageTooLarge(pageNr)
		}
		return b, nil, err
	}
	var out, prev []byte
	var seps []int
	decoded := map[int][]byte{}
	for i, e := range arr {
		if e == nil {
			continue
		}
		ir, isRef := e.(types.IndirectRef)
		b, seen := decoded[ir.ObjectNumber.Value()]
		if !isRef || !seen {
			// pdfcpu refuses an element that is not a stream; handed one alone, its call would accept a nested array.
			if r, _ := ctx.Dereference(e); r != nil {
				if _, nested := r.(types.Array); nested {
					return nil, nil, fmt.Errorf("page %d content: a /Contents array holds an array, not a stream", pageNr)
				}
			}
			b, err = ctx.PageContent(types.Dict{"Contents": e}, pageNr) //pagecontent:door
			if err == model.ErrNoContent {
				b, err = nil, nil
			}
			if err != nil {
				return nil, nil, err
			}
			if isRef {
				decoded[ir.ObjectNumber.Value()] = b
			}
		}
		if len(b) == 0 {
			continue
		}
		// **The previous STREAM, never the join so far.** A stream begins on a token boundary, so its own
		// tokenization is the one that matters at its end; handing the accumulated join instead re-tokenized the
		// whole page at every join — quadratic, measured at 13.8 s for 1,000 streams against pdfcpu's 10 ms.
		sep := separate && needsSeparator(prev, b)
		if n := len(out) + len(b); n > MaxPageContentBytes || sep && n+1 > MaxPageContentBytes {
			return nil, nil, pageTooLarge(pageNr)
		}
		if sep {
			out = append(out, '\n')
			seps = append(seps, i)
		}
		out = append(out, b...)
		prev = b
	}
	if len(out) == 0 {
		return nil, nil, model.ErrNoContent
	}
	return out, seps, nil
}

// SeparateContents makes pdfcpu's OWN join of a page's `/Contents` read as this door's (`/pending 728`, ADR-084).
//
// pdfcpu's page operations — `NUpFromPDF` (nup.go:328), `Resize` (resize.go:276), `CutPage` (cut.go:175/263/353) —
// read a page through their own bare join and write what they read into the document nib hands back, so a page
// divided `(A) Tj` | `ET` came out of every one of them as the unknown operator `TjET`, its text gone (measured:
// pdftotext reads the input and none of the outputs). Handing those operations a context they join correctly is
// the one fix that reaches all of them: wherever the door puts a `\n` between two elements, a stream holding only
// `\n` is put into the array. pdfcpu's join of the result is then byte-identical to the door's join of the
// original, so a carry comparing pdfcpu's output with the source page reads the source through the door.
//
// Nothing is rewritten where no separator is due — every document measured at ADR-056 — and no stream is edited,
// so a stream shared between pages, or named twice in one array, is untouched. pageNrs names the pages to
// separate; nil is every page. A page the door refuses is refused here: it is the page the operation was about to
// read unbounded.
func SeparateContents(ctx *model.Context, pageNrs []int) error {
	want := map[int]bool{}
	for _, n := range pageNrs {
		want[n] = true
	}
	var sepRef *types.IndirectRef
	for _, pa := range Pages(ctx) {
		if pa.Err != nil || pa.Dict == nil || pageNrs != nil && !want[pa.Nr] {
			continue
		}
		_, seps, err := joinContent(ctx, pa.Dict, pa.Nr, true)
		if err == model.ErrNoContent {
			continue
		}
		if err != nil {
			return err
		}
		if len(seps) == 0 {
			continue
		}
		if sepRef == nil {
			sd, err := ctx.NewStreamDictForBuf([]byte{'\n'})
			if err != nil {
				return err
			}
			if err := sd.Encode(); err != nil {
				return err
			}
			if sepRef, err = ctx.IndRefForNewObject(*sd); err != nil {
				return err
			}
		}
		o, _ := pa.Dict.Find("Contents")
		r, _ := ctx.Dereference(o)
		arr, _ := r.(types.Array) // joinContent reported separators, so it read an array
		out := make(types.Array, 0, len(arr)+len(seps))
		for i, e := range arr {
			if len(seps) > 0 && seps[0] == i {
				out = append(out, *sepRef)
				seps = seps[1:]
			}
			out = append(out, e)
		}
		// The page's own array, never the object it named: an indirect `/Contents` array may be shared with a page
		// this call was not asked to separate.
		pa.Dict["Contents"] = out
	}
	return nil
}

// pageTooLarge is the refusal of a page whose content decodes past MaxPageContentBytes.
func pageTooLarge(pageNr int) error {
	return fmt.Errorf("page %d content: %w: its /Contents decode past %d bytes together, and nib stopped reading "+
		"there", pageNr, ErrDecodeLimit, MaxPageContentBytes)
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
	// tokenize: removing either leaves every answer unchanged (probed), so neither carries CORRECTNESS — but both carry
	// COST (/pending 803). The tokenize builds every token of prev (24 bytes each) to read the last: measured, a 32 MB
	// stream of `a ` behind a `%` costs this join 2.3 s and 4.7 GB allocated, against 38 ms without the `%`. It is
	// bounded, not removed: each element is tokenized once, as prev, so the whole page's joins tokenize at most
	// `MaxPageContentBytes` — what every caller then tokenizes again over the joined page (1.3 s on that page).
	if last == '\n' || last == '\r' {
		return false
	}
	// The same end-of-line at the start of the later stream ends the comment as well, and it is what makes
	// SeparateContents idempotent: a `\n` stream it put after a comment is not separated again.
	if next[0] == '\n' || next[0] == '\r' {
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
