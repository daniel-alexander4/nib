package pdfops

import (
	"context"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// What a splice keeps of the document it was asked to change — `/pending 602`.
//
// `splice` sets its page order with `selectPages`, and that primitive reduces a document to what an
// EXTRACT may carry: it empties the document `/Info` (the title, nib's own `NibFlags`), forgets the
// trailer's `/ID` and deletes the `/EmbeddedFiles` name tree, each for a reason stated there — the
// output is an artifact somebody is about to be handed. An insert or a split is not that. Its output IS
// the document, with a page added or one page cut into tiles, and the same file through `Append`,
// `InsertBlank` or `Crop` keeps all four. Measured before this was written, on a three-page host with one
// attachment, a title, a flag and an `/ID`: `InsertPDF` and `SplitPage` each returned 0 attachments, no
// title, no flags and a new `/ID[0]`; `Append` returned all of them.
//
// So `spliceOnce` captures them before the selection and puts them back after it, in the SAME context
// and the same write. They are read from the merged context, which is what `Append` writes: the host's
// `/Info` and `/ID` (a merge keeps the first document's), and the embedded files of BOTH documents —
// pdfcpu's merge joins the name trees, so a document inserted at a position brings its attachments exactly
// as one appended at the end does. `selectPages` itself is unchanged, and every other caller of it still
// gets the extract's reduction.
//
// **`NibFlags` is the one entry NOT put back, and that is a decision rather than an omission.** A flag is a
// page NUMBER and a fraction of that page (flags.go), and a splice renumbers every page after the insertion
// point and removes the page a split replaces — so the list carried as it stood would put each later flag on
// the page before the one it was placed on, which is worse than the loss. Renumbering it here would be a
// second writer of that blob beside the reflow door's, for a caller that does not exist: the only route to a
// splice is `handlePages`, and the window strips the flags from what it posts there (`internal/server`'s
// flag census, `flagsStripped`). So a flagged document handed straight to this package loses its flags
// through a splice, as it always has.
//
// **Nothing is put back that reaches a page the splice dropped.** pdfcpu writes by reachability, so a
// file specification or an `/Info` entry holding a reference to the page a split replaced would write that
// page's content into the output — `pageselect.go`'s `/StructTreeRoot` hazard on a key nobody would think
// to check. `reachesObject` is the test `/OCProperties` is carried under, and the answer to a group that
// fails it is the loss this door produced before.
//
// **The catalog's metadata packet goes with `/Info`, or not at all** (`/pending 870`). The selection drops
// `/Metadata` with every catalog key its allowlist does not name, and where it carries a structure tree it builds
// a packet holding the title alone — so a splice kept the `/Info` title and lost the packet that says the same
// thing and more (measured: `InsertPDF` and `SplitPage` returned the title in `/Info` and no packet; `Append` and
// `InsertBlank` returned both). The two describe one document and a reader may ask either, so the packet is put
// back exactly when the `/Info` entries are, over the title-only one. **Never with a conformance claim** (ADR-032):
// this puts back the stream the context already held, and each caller drops the identification from that same
// stream inside its own write — `rewriteWithConf` before the change runs, `mergeOnce` after it.
type documentFacts struct {
	id       types.Array
	info     types.Dict   // the entries of the `/Info` dictionary, held aside while the selection empties it
	metadata types.Object // the catalog's `/Metadata` as written, or nil
	files    *model.Node  // the `/EmbeddedFiles` name tree as the merged context holds it
	pages    map[int]bool // every page object the context held before the selection
}

// captureDocumentFacts reads what restore puts back. It must run before `selectPages`.
func captureDocumentFacts(ctx *model.Context) documentFacts {
	xt := ctx.XRefTable
	f := documentFacts{id: ctx.ID, files: xt.Names["EmbeddedFiles"], pages: map[int]bool{}}
	if root, err := xt.Catalog(); err == nil {
		f.metadata = root["Metadata"]
	}
	if ctx.Info != nil {
		if info := derefDict(xt, *ctx.Info); info != nil {
			f.info = make(types.Dict, len(info))
			for k, v := range info {
				if k != flagsKey {
					f.info[k] = v
				}
			}
		}
	}
	for _, pa := range pdfread.Pages(ctx) {
		if pa.Ref != nil {
			f.pages[pa.Ref.ObjectNumber.Value()] = true
		}
	}
	return f
}

// restore puts the captured facts back after `selectPages`, each group only when nothing in it reaches a
// page the selection dropped. filesWithheld is true when the document had embedded files and they were NOT put
// back for that reason — the one group whose loss a caller may have another way to make good (`sameDocument`).
func (f documentFacts) restore(ctx *model.Context) (filesWithheld bool) {
	xt := ctx.XRefTable
	dropped := map[int]bool{}
	for nr := range f.pages {
		dropped[nr] = true
	}
	for _, pa := range pdfread.Pages(ctx) {
		if pa.Ref != nil {
			delete(dropped, pa.Ref.ObjectNumber.Value())
		}
	}
	ctx.ID = f.id
	infoKept := !reachesObject(xt, f.info, dropped, map[int]bool{}, 0)
	if ctx.Info != nil && len(f.info) > 0 && infoKept {
		if info := derefDict(xt, *ctx.Info); info != nil {
			for k, v := range f.info {
				info[k] = v
			}
		}
	}
	if f.metadata != nil && infoKept && !reachesObject(xt, f.metadata, dropped, map[int]bool{}, 0) {
		if root, err := xt.Catalog(); err == nil {
			root["Metadata"] = f.metadata
		}
	}
	if f.files == nil {
		return false
	}
	seen := map[int]bool{}
	safe := true
	if err := f.files.Process(context.Background(), xt, func(_ *model.XRefTable, _ string, v *types.Object) error {
		if reachesObject(xt, *v, dropped, seen, 0) {
			safe = false
		}
		return nil
	}); err != nil || !safe {
		return true
	}
	// The cache is what `BindNameTrees` writes onto the catalog, making `/Names` where the selection
	// left none.
	xt.Names["EmbeddedFiles"] = f.files
	return false
}
