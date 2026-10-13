package pdfread

import (
	"bytes"
	"errors"
	"runtime"
	"sync"
	"weak"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// An XFA form is not kept, and it is not dropped by half — ADR-145, `/pending 866`, `/pending 843`.
//
// A PDF may carry its form twice: as fields on its pages, and as an XFA packet in the catalog's `/AcroForm /XFA`
// that some readers draw INSTEAD of the pages. nib shows and fills only the pages. Before this door two things
// happened to the packet, neither of them asked for. pdfcpu's validator deletes the whole `/AcroForm` of a
// document whose form has no fields — an XFA-only form — before any of nib's code has the context
// (`validate/form.go` `validateAcroForm`, v0.13.0), and every write then left the catalog's `/NeedsRendering true`
// behind, telling a reader to build the pages from a form that was gone. And a form holding both kept its packet
// through every change, so an XFA-aware reader went on drawing what the packet said while nib changed the pages.
//
// **So every write drops the two keys together** (`dropXFA`, called by `Write` and by nothing else): `/XFA` off the
// form and `/NeedsRendering` off the catalog. `Write` is the one door to pdfcpu's writer
// (`TestEveryWriteRoutesThroughTheDoor`), so no operation can write a document that keeps either.
//
// **And a document whose pages are only a placeholder is not written at all** (`ErrDynamicXFA`). `/NeedsRendering
// true` says the pages in the file are not the document — Acrobat builds them from the XFA template, and what the
// file holds is one page reading "Please wait…". Dropping the form from that leaves the placeholder and nothing
// else, so it is refused, naming its cause. The refusal keys on that one key and not on "XFA-only": a static XFA
// form's pages are the document, and losing its packet loses nothing nib could have shown.
//
// **The reading is taken BEFORE validation**, because afterwards an XFA-only form is not there to read
// (`rememberXFA`), and kept against the context, weakly, as `afOnly` is. An operation that rebuilds the catalog
// — a page selection keeps only an allow-list of its keys — would otherwise take `/NeedsRendering` away and make a
// placeholder writable. The mark does not follow objects into ANOTHER context, so a function that moves them (a
// merge source, a cut, an n-up) asks `RefuseDynamicXFA` first, beside `PutBackValidatorLosses`.

// What XFAOf answers.
const (
	XFAStatic  = "static"  // an XFA packet beside pages that are the document
	XFADynamic = "dynamic" // `/NeedsRendering true`: the pages are a placeholder for the form
)

// DynamicXFARefusal is the sentence a user reads; the server's error door recognises it to answer 422 with a cause.
const DynamicXFARefusal = "this document's pages are only a placeholder for a dynamic XFA form, which Nib does not show or fill, and a changed copy would keep the placeholder and lose the form — so nothing was changed"

// ErrDynamicXFA is the refusal of a document read with `/NeedsRendering true`.
var ErrDynamicXFA = errors.New(DynamicXFARefusal)

// xfaRead holds, per context, what the document it was read from said about XFA before validation.
var xfaRead = struct {
	sync.Mutex
	m map[weak.Pointer[model.Context]]string
}{m: map[weak.Pointer[model.Context]]string{}}

// xfaIn reads the catalog as it stands in ctx: XFADynamic for `/NeedsRendering true` (with or without a packet —
// the key alone says the pages are not the document), XFAStatic for an `/AcroForm` holding `/XFA`, else "".
//
// `ctx.Dereference` and never a typed dereference: this runs before the validator, and pdfcpu's
// `DereferenceStreamDict` marks what it returns as validated (ADR-116). The packet itself is not looked at.
func xfaIn(ctx *model.Context) string {
	if ctx == nil || ctx.XRefTable == nil {
		return ""
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil || root == nil {
		return ""
	}
	if o, err := ctx.Dereference(root["NeedsRendering"]); err == nil {
		if b, ok := o.(types.Boolean); ok && bool(b) {
			return XFADynamic
		}
	}
	if o, err := ctx.Dereference(root["AcroForm"]); err == nil {
		if form, ok := o.(types.Dict); ok {
			if _, has := form["XFA"]; has {
				return XFAStatic
			}
		}
	}
	return ""
}

// rememberXFA records against ctx what xfaIn read before validation. Nothing is stored for a document with no XFA,
// which is nearly every document: the read pays two catalog lookups.
func rememberXFA(ctx *model.Context, read string) {
	if read == "" {
		return
	}
	key := weak.Make(ctx)
	xfaRead.Lock()
	xfaRead.m[key] = read
	xfaRead.Unlock()
	runtime.AddCleanup(ctx, func(k weak.Pointer[model.Context]) {
		xfaRead.Lock()
		delete(xfaRead.m, k)
		xfaRead.Unlock()
	}, key)
}

// XFAOf is what the document ctx was read from said about XFA — XFAStatic, XFADynamic or "" — as it was written,
// before pdfcpu's validator deleted an XFA-only form. It is what the document ARRIVED with; an operation that has
// since removed the form does not change the answer.
func XFAOf(ctx *model.Context) string {
	xfaRead.Lock()
	defer xfaRead.Unlock()
	return xfaRead.m[weak.Make(ctx)]
}

// ReadXFA is XFAOf for bytes: one unvalidated parse (`api.ReadContext`), so it answers for a document the validator
// would refuse too. It is the reading behind the notice a window shows when such a document is opened, and behind
// a ceremony's refusal to be convened over one.
func ReadXFA(pdf []byte) (read string, err error) {
	defer fault.Catch(&err)
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return "", err
	}
	return xfaIn(ctx), nil
}

// RefuseDynamicXFA is ErrDynamicXFA where ctx was read from a document whose pages are a placeholder, or whose
// catalog says so now. `Write` asks it; so must any function about to move ctx's objects into another context,
// where the mark does not follow (`TestEveryWriteRoutesThroughTheDoor`).
func RefuseDynamicXFA(ctx *model.Context) error {
	if XFAOf(ctx) == XFADynamic || xfaIn(ctx) == XFADynamic {
		return ErrDynamicXFA
	}
	return nil
}

// dropXFA takes `/XFA` off ctx's form and `/NeedsRendering` off its catalog — together, so neither is left
// describing the other. `Write` is its one caller. The packet's objects are not freed: pdfcpu's writer writes only
// what the catalog reaches, and nothing reaches them once the key is gone.
func dropXFA(ctx *model.Context) {
	root, err := ctx.XRefTable.Catalog()
	if err != nil || root == nil {
		return
	}
	delete(root, "NeedsRendering")
	if form, _ := ctx.DereferenceDict(root["AcroForm"]); form != nil {
		delete(form, "XFA")
	}
}
