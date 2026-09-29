package pdfread

import (
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Page is one page exactly as `ctx.PageDict(Nr, false)` answers for it: the dictionary, its reference, the
// attributes in effect on it, or the error pdfcpu returned. Dict is nil when Err is not.
type Page struct {
	Nr    int
	Dict  types.Dict
	Ref   *types.IndirectRef
	Attrs *model.InheritedPageAttrs
	Err   error
}

// Pages answers `ctx.PageDict(p, false)` for every page 1..PageCount, in order, from ONE walk of the page tree
// (/pending 753). It is the door every per-page sweep goes through (ADR-009).
//
// # Why: `PageDict` walks the page tree from the root at every call
//
// pdfcpu skips a subtree by its `/Count`, so on a balanced tree one call is cheap — but on a FLAT tree, which is
// what pdfcpu's own writer and nib's Markdown conversion produce (one `/Pages` holding every page), each call
// dereferences every kid before the one it wants. A loop over the pages is then quadratic: preparing a 7,059-page
// document for a ceremony took ~50 s and 13 GiB, most of it in `processPageTreeForPageDict`.
//
// # It answers what pdfcpu answers, or it asks pdfcpu
//
// Every caller was written against `PageDict`'s answer, and several write what they read (a crop, a tab order, a
// language), so this must never re-decide what a page is. The walk mirrors pdfcpu's own
// (`processPageTreeForPageDictDepth`, `model/xreftable.go`, v0.13.0) for every page at once, and wherever the two
// could part company it gives up and asks `PageDict` page by page, at the old cost:
//
//   - a node's `/Count` (as `IntEntry` reads it) that is absent or disagrees with the pages beneath it. pdfcpu
//     SKIPS a subtree by its count, so a wrong one moves which page a number reaches; and where it is absent it
//     ENTERS a subtree that does not hold the page asked for, letting that subtree's attributes leak into the
//     answer for a later page. Neither is something one walk can reproduce for every page at once.
//   - a node typed neither `/Pages` nor `/Page` (pdfcpu repairs, skips or refuses it by validation mode);
//   - a `/Page` that carries `/Kids`, or a `/Pages` without a direct `/Kids` array (pdfcpu answers such a node
//     itself as the page);
//   - a kid that is not a reference, a cycle, a tree deeper than pdfcpu allows, an attribute pdfcpu would refuse;
//   - a count of pages that is not `ctx.PageCount`.
//
// Attributes are resolved along the path exactly as `checkInheritedPageAttrs` does with `consolidateRes` false —
// `/MediaBox` and `/CropBox` rebuilt as a fresh rectangle per page (pdfcpu hands every call a new one, and a caller
// may keep it), `/Rotate`, and `/Resources` dereferenced, not merged.
func Pages(ctx *model.Context) []Page {
	out, _ := PagesWalked(ctx)
	return out
}

// PagesWalked is Pages, also saying whether the one walk answered (true) or pdfcpu was asked page by page (false).
// Only an instrument reads the flag; the pages are the same answer either way.
func PagesWalked(ctx *model.Context) ([]Page, bool) {
	if out, ok := walkPages(ctx.XRefTable); ok {
		return out, true
	}
	out := make([]Page, 0, max(ctx.PageCount, 0))
	for p := 1; p <= ctx.PageCount; p++ {
		d, ref, attrs, err := ctx.PageDict(p, false)
		out = append(out, Page{Nr: p, Dict: d, Ref: ref, Attrs: attrs, Err: err})
	}
	return out, false
}

// pathAttrs is what a node's ancestors (and the node) set, kept as the source objects so each page gets its own
// rectangles.
type pathAttrs struct {
	mediaBox, cropBox types.Array
	rotate            int
	resources         types.Dict
}

func (a pathAttrs) resolve(xt *model.XRefTable) (*model.InheritedPageAttrs, bool) {
	out := &model.InheritedPageAttrs{Resources: a.resources, Rotate: a.rotate}
	var ok bool
	if a.mediaBox != nil {
		if out.MediaBox, ok = rectOf(xt, a.mediaBox); !ok {
			return nil, false
		}
	}
	if a.cropBox != nil {
		if out.CropBox, ok = rectOf(xt, a.cropBox); !ok {
			return nil, false
		}
	}
	return out, true
}

// rectOf is pdfcpu's `rect`, which indexes four entries unchecked; a shorter array is left to pdfcpu.
func rectOf(xt *model.XRefTable, a types.Array) (*types.Rectangle, bool) {
	if len(a) < 4 {
		return nil, false
	}
	var v [4]float64
	for i := range v {
		f, err := xt.DereferenceNumber(a[i])
		if err != nil {
			return nil, false
		}
		v[i] = f
	}
	return types.NewRectangle(v[0], v[1], v[2], v[3]), true
}

// apply is `checkInheritedPageAttrs(d, pAttrs, false)`. An array that will not dereference, a rectangle that will
// not read, a /Rotate pdfcpu refuses or a /Resources that is not a dictionary all return false: pdfcpu's answer
// there is an error, and it is pdfcpu's to give.
func (a *pathAttrs) apply(xt *model.XRefTable, d types.Dict) bool {
	for _, key := range []string{"MediaBox", "CropBox"} {
		obj, found := d.Find(key)
		if !found {
			continue
		}
		arr, err := xt.DereferenceArray(obj)
		if err != nil {
			return false
		}
		if _, ok := rectOf(xt, arr); !ok {
			return false
		}
		if key == "MediaBox" {
			a.mediaBox = arr
		} else {
			a.cropBox = arr
		}
	}
	if obj, found := d.Find("Rotate"); found {
		obj, err := xt.Dereference(obj)
		if err != nil {
			return false
		}
		switch r := obj.(type) {
		case types.Integer:
			a.rotate = r.Value()
		case types.Float:
			if xt.ValidationMode == model.ValidationStrict {
				return false
			}
			a.rotate = int(math.Round(r.Value()))
		default:
			return false
		}
	}
	if obj, found := d.Find("Resources"); found {
		res, err := xt.DereferenceDict(obj)
		if err != nil {
			return false
		}
		a.resources = res
	}
	return true
}

// walkPages is the one walk. false means "ask pdfcpu page by page" — see Pages for every case that says it.
func walkPages(xt *model.XRefTable) ([]Page, bool) {
	rootRef, err := xt.Pages()
	if err != nil || rootRef == nil {
		return nil, false
	}
	out := make([]Page, 0, max(xt.PageCount, 0))
	visit := model.NewPageTreeVisit()

	// node walks a /Pages node and returns how many pages it held.
	var node func(ref types.IndirectRef, d types.Dict, attrs pathAttrs, depth int) (int, bool)
	node = func(ref types.IndirectRef, d types.Dict, attrs pathAttrs, depth int) (int, bool) {
		if xt.CheckRecursionDepth("page tree", depth) != nil {
			return 0, false
		}
		if !attrs.apply(xt, d) {
			return 0, false
		}
		kids := d.ArrayEntry("Kids")
		if kids == nil {
			return 0, false // pdfcpu would answer this /Pages node itself as a page
		}
		objNr := ref.ObjectNumber.Value()
		if visit.Enter(objNr) != nil {
			return 0, false
		}
		defer visit.Leave(objNr)
		held := 0
		for _, o := range kids {
			if o == nil {
				continue
			}
			kr, isRef := o.(types.IndirectRef)
			if !isRef {
				return 0, false
			}
			kd, err := xt.DereferenceDict(kr)
			if err != nil || kd == nil {
				return 0, false
			}
			t := kd.Type()
			if t == nil {
				return 0, false
			}
			switch *t {
			case "Pages":
				c := kd.IntEntry("Count")
				if c == nil {
					return 0, false
				}
				n, ok := node(kr, kd, attrs, depth+1)
				if !ok || n != *c {
					return 0, false
				}
				held += n
			case "Page":
				if xt.CheckRecursionDepth("page tree", depth+1) != nil {
					return 0, false
				}
				if c := kd.IntEntry("Count"); c != nil && *c < 0 {
					return 0, false // pdfcpu would skip its own target
				}
				if kd.ArrayEntry("Kids") != nil {
					return 0, false // pdfcpu descends past a /Page with a direct /Kids array
				}
				leaf := attrs
				if !leaf.apply(xt, kd) {
					return 0, false
				}
				resolved, ok := leaf.resolve(xt)
				if !ok {
					return 0, false
				}
				r := kr
				out = append(out, Page{Nr: len(out) + 1, Dict: kd, Ref: &r, Attrs: resolved})
				held++
			default:
				return 0, false
			}
		}
		return held, true
	}

	root, err := xt.DereferenceDict(*rootRef)
	if err != nil || root == nil {
		return nil, false
	}
	n, ok := node(*rootRef, root, pathAttrs{}, 0)
	if !ok || n != xt.PageCount || len(out) != xt.PageCount {
		return nil, false
	}
	if c := root.IntEntry("Count"); c != nil && *c != n {
		return nil, false
	}
	return out, true
}
