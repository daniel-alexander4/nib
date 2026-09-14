package pdfops

import (
	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Shared structure-tree plumbing — what every door that writes a tree uses.
//
// **This file held P05.S04's generic wrapping emitter and `TagAuthored`, deleted at
// `PLAN-accessibility.md` P08.S07 (`/pending 480`).** It bracketed a whole page's content in one
// `/Div`, because it knew nothing about what the content said. P05.S05 left it unwired by decision,
// P06 built typed doors instead, and P08's proposer and commit writer tag what that emitter could
// only group. Its own entry's rule — *"if P08 opens and does not call it, that is B"* — was met: P08
// proposes from the page, and a page with no text is OCR's, not a generic `/Div`'s. What every
// other door needs from it stays here.

// ensureStructTree returns the document's parsed tree, creating an empty one when it has none.
func ensureStructTree(ctx *model.Context, live map[int]bool) (*structTree, error) {
	tree, err := readStructTree(ctx, live)
	if err == nil {
		return tree, nil
	}
	if err != errNoStructTree {
		// A tree that exists and cannot be represented is not one to add to. Refusing here is the
		// same rule S02's parser follows, for the same reason: a partial model of somebody's tree
		// plus nib's additions is a document nothing can read correctly.
		return nil, err
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return nil, cerr
	}
	ptRef, perr := ctx.IndRefForNewObject(types.Dict{"Nums": types.Array{}})
	if perr != nil {
		return nil, perr
	}
	rootDict := types.Dict{
		"Type":       types.Name("StructTreeRoot"),
		"K":          types.Array{},
		"ParentTree": *ptRef,
	}
	rootRef, rerr := ctx.IndRefForNewObject(rootDict)
	if rerr != nil {
		return nil, rerr
	}
	cat["StructTreeRoot"] = *rootRef
	return readStructTree(ctx, live)
}

// alreadyMarked reports whether a content stream carries marked content of its own.
//
// **Tokenized, not searched.** `BDC` appears inside a string literal in any document that draws the
// characters B, D and C consecutively — and since P04 every glyph nib draws is a two-byte index, so
// arbitrary byte pairs inside strings are the normal case rather than the exotic one. A byte scan
// would report a page as already tagged because of what it says.
func alreadyMarked(src []byte) bool {
	for _, tk := range contentstream.Tokenize(src) {
		if tk.Kind != contentstream.Operator {
			continue
		}
		switch string(tk.Bytes(src)) {
		case "BDC", "BMC":
			return true
		}
	}
	return false
}

// setPageContent replaces a page's content stream with b.
//
// The same shape `wrapPageToBox` uses (`pdfops.go`): a new stream object, encoded, and the page's
// `/Contents` pointed at it. Replacing rather than editing in place because `/Contents` may be an
// ARRAY of streams, and a page whose content is split across three objects has no single one to
// edit — the decoded bytes this wrote came from all of them concatenated, which is what
// `PageContent` returns and what a single replacement stream is equivalent to.
func setPageContent(ctx *model.Context, page types.Dict, b []byte) error {
	sd, err := ctx.NewStreamDictForBuf(b)
	if err != nil {
		return err
	}
	if err := sd.Encode(); err != nil {
		return err
	}
	ref, err := ctx.IndRefForNewObject(*sd)
	if err != nil {
		return err
	}
	page["Contents"] = *ref
	return nil
}
