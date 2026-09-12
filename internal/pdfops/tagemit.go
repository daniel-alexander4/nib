package pdfops

import (
	"fmt"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The wrapping emitter — `PLAN-accessibility.md` P05.S04, D3.
//
// **The first place nib emits content-stream operators of its own**, which is why S01 built a
// walker whose round trip is byte-identical: everything here is an insertion at an offset, and the
// content between the inserted operators is never re-serialised.
//
// # The honest structure type, which is the slice's real decision
//
// This emitter knows where a page's content is and **nothing about what it says**. So the element
// it creates must claim grouping without claiming role:
//
//   - `/P` would say every page is one paragraph, which is false of any page with a heading on it.
//     ADR-031's law 1 is about not claiming what you have not got, and "this heading is a
//     paragraph" is a claim of exactly that kind — a quieter one than `/MarkInfo` with no tree, and
//     the same species.
//   - `/Div` is ISO 32000-1's generic block-level grouping element. It says *this is real content,
//     grouped* and nothing else, which is the whole of what this code knows.
//
// Real structure — headings as `H1`, lists as `L`/`LI`, from `mdpdf`'s own AST — is **P06**, which
// has the information this does not. The two are not competing: P06 replaces the type, not the
// mechanism.
//
// # What it refuses to do
//
// A page that already carries marked content is left alone. Wrapping it again would produce nested
// marked content whose inner MCIDs belong to a producer's tree and whose outer one belongs to
// nib's, and no reader can be expected to make sense of a page described twice.
const authoredStructType = "Div"

// tagAuthoredContent wraps every unmarked page's content in a `BDC`/`EMC` pair and builds the
// structure tree that describes it.
//
// It returns the number of pages it wrapped, so a caller can tell "nothing needed doing" from
// "nothing was done" — the two are the same bytes and different facts.
func tagAuthoredContent(pdf []byte) (out []byte, wrapped int, err error) {
	out, err = writeMutated(pdf, func(ctx *model.Context) error {
		live := map[int]bool{}
		for p := 1; p <= ctx.PageCount; p++ {
			if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
				live[ir.ObjectNumber.Value()] = true
			}
		}
		tree, terr := ensureStructTree(ctx, live)
		if terr != nil {
			return terr
		}
		for p := 1; p <= ctx.PageCount; p++ {
			did, werr := wrapOnePage(ctx, tree, p)
			if werr != nil {
				return werr
			}
			if did {
				wrapped++
			}
		}
		return nil
	})
	return out, wrapped, err
}

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

// wrapOnePage brackets one page's content stream and registers the element that describes it.
//
// It reports whether it wrapped, which is false for a page already carrying marked content and for
// a page with no content at all.
func wrapOnePage(ctx *model.Context, tree *structTree, pageNr int) (bool, error) {
	d, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil || d == nil {
		return false, nil // a page that does not resolve is not this operation's business
	}
	src, cerr := ctx.PageContent(d, pageNr)
	if cerr == model.ErrNoContent || len(src) == 0 {
		// A blank page has nothing to tag. Not a failure — `InsertBlank` makes these.
		return false, nil
	}
	if cerr != nil {
		return false, cerr
	}
	if alreadyMarked(src) {
		return false, nil
	}

	mcid, _, aerr := addMarkedElement(ctx, tree, pageNr, authoredStructType)
	if aerr != nil {
		return false, aerr
	}

	// **Insertions at offsets in the ORIGINAL stream**, so the wrapped bytes are copied and never
	// re-serialised — S01's whole reason for existing. The trailing newline before `EMC` matters:
	// a content stream ending in an operator with no separator would run `EMC` onto it.
	edited, eerr := contentstream.NewEdit(src).
		InsertBefore(0, []byte(fmt.Sprintf("/%s <</MCID %d>> BDC\n", authoredStructType, mcid))).
		InsertBefore(len(src), []byte("\nEMC")).
		Apply()
	if eerr != nil {
		return false, eerr
	}
	return true, setPageContent(ctx, d, edited)
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
