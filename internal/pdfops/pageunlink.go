package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// unlinkDroppedPages is the ONE door that cuts every road from what a page selection keeps to a page it
// dropped (ADR-009, `/pending 726`, `/pending 765`). It runs LAST in `selectPages`, once every key that
// can reach a page — the allowlisted catalog, the carried outline, open action, labels, optional
// content and structure tree — is back in place, so nothing is re-added after it has looked.
//
// # Why one door and not one key more
//
// pdfcpu writes by reachability, so a dropped page's dictionary — and its `/Contents`, which is to say
// its TEXT — goes into the output whenever anything kept still names it. That is a redaction-class
// leak: "removed" meaning "hidden". It was found one key at a time — a field's `/Kids`, `/Dest`, a
// GoTo's `/D`, `/IRT` and `/Popup` (`/pending 709` R2-3), and then a kept popup's own `/P`, a `/Next`
// chain, `/AA` and a `/Hide` target (`/pending 726`, `/pending 765`). Each fix was right and each
// left the next key open, because the question it asked was "does THIS key name a dropped page".
// This door asks it of every reference the output holds.
//
// # Two layers, and why the first is not redundant with the second
//
// The semantic layer repairs what has a correct repair: a kept annotation whose `/P` names a dropped
// page is re-pointed at the kept page it sits on (it is a malformed source, but the annotation is
// real); a `/Dest` that does not reach a kept page goes; a GoTo action that does not is SPLICED out of
// its chain — `/A`, `/Next` (a dictionary or an array) and every `/AA` trigger — so a URI action
// whose `/Next` jumped to the removed page still opens its URI; and `unlinkAnnotationThreads` cuts a
// reply or popup thread to an annotation not on a kept page. Only `/S /GoTo` is judged: a `/GoToR`'s
// `/D` names a page of ANOTHER file, and resolving it against this one's name tree would delete a
// working link.
//
// The net is the completeness half. It walks everything the catalog reaches and cuts any reference to
// a POISONED object: a dropped page; an annotation that is on a dropped page and not on a kept one (or,
// listed nowhere, names a dropped page as its `/P`), because its `/Contents` is that page's comment
// text; and a destination array whose page is a dropped one. In a dictionary the key goes; in an array
// the element goes. That repair is crude by design — a `/Hide` action losing its `/T`, a bead losing its
// `/P` — and it only ever fires on a reference that would otherwise be either a leak or, where pdfcpu
// happens not to follow the key, a dangling object number in the file.
//
// **Destinations are judged by `destReachesAKeptPage`, never a narrower question** (ADR-009,
// `/pending 709` R2-4): a name — how Word and hyperref write internal links — resolves through the
// `/Dests` tree `pruneNames` has already cut, so a name still there names a kept page. pdfcpu's own
// migration is no help: it patches a reference through a lookup the dropped page is absent from,
// which yields `0 0 R`.
//
// The net needs no list of keys and so cannot go stale as keys are added; the semantic layer exists
// because the net's repair of a destination or an action would leave a malformed one in its place.
// `orphanPageObjects` asks the same question after the write and only for a carried tree; this asks it
// before the write, for every subset, and answers it by cutting rather than by refusing a tag claim.
func unlinkDroppedPages(xt *model.XRefTable, root types.Dict, pages []keptPage, dropped []types.Dict, droppedNr map[int]bool, kept map[int]bool) {
	u := &pageUnlink{
		xt:      xt,
		kept:    kept,
		dropped: droppedNr,
		onKept:  map[int]bool{},
		onGone:  map[int]bool{},
		poison:  map[int]bool{},
		seen:    map[int]bool{},
	}
	for _, p := range pages {
		for _, a := range derefArray(xt, p.dic["Annots"]) {
			if ar, ok := a.(types.IndirectRef); ok {
				u.onKept[ar.ObjectNumber.Value()] = true
			}
		}
	}
	for _, d := range dropped {
		for _, a := range derefArray(xt, d["Annots"]) {
			if ar, ok := a.(types.IndirectRef); ok && !u.onKept[ar.ObjectNumber.Value()] {
				u.onGone[ar.ObjectNumber.Value()] = true
			}
		}
	}
	for nr := range droppedNr {
		u.poison[nr] = true
	}

	// A kept annotation's own /P is the one reference whose repair is known exactly: the page it is on.
	for _, p := range pages {
		for _, a := range derefArray(xt, p.dic["Annots"]) {
			ad := derefDict(xt, a)
			if ad == nil {
				continue
			}
			if pr, ok := ad["P"].(types.IndirectRef); ok && droppedNr[pr.ObjectNumber.Value()] {
				ad["P"] = p.ref
			}
		}
	}
	keptDicts := make([]types.Dict, 0, len(pages))
	for _, p := range pages {
		keptDicts = append(keptDicts, p.dic)
	}
	unlinkAnnotationThreads(xt, keptDicts)

	u.dict(root, 0)
}

// maxDirectDepth bounds the net's recursion through DIRECT objects; an indirect one is visited once
// through `seen`, so only a literal nesting this deep can reach the bound. pdfcpu's own parser nests
// no deeper in practice, and a structure past it is left as it is rather than walked.
const maxDirectDepth = 200

type pageUnlink struct {
	xt      *model.XRefTable
	kept    map[int]bool // kept page objects, for `destReachesAKeptPage`
	dropped map[int]bool // dropped page objects
	onKept  map[int]bool // annotations in a kept page's /Annots
	onGone  map[int]bool // annotations in a dropped page's /Annots and no kept one's
	poison  map[int]bool // objects every reference to which is cut
	seen    map[int]bool
}

// cut reports whether a reference to r must go, visiting r first if it has not been visited. Whether
// an object is poisoned is decided from the object itself before its children are walked, so a cycle
// back to it reads the settled answer.
func (u *pageUnlink) cut(r types.IndirectRef, depth int) bool {
	nr := r.ObjectNumber.Value()
	if u.poison[nr] {
		return true
	}
	if u.seen[nr] {
		return false
	}
	u.seen[nr] = true
	o, err := u.xt.Dereference(r)
	if err != nil || o == nil {
		return false
	}
	if u.poisoned(nr, o) {
		u.poison[nr] = true
		return true
	}
	switch v := o.(type) {
	case types.Dict:
		u.dict(v, depth)
	case types.StreamDict:
		u.dict(v.Dict, depth)
	case types.Array:
		if out, changed := u.array(v, depth); changed {
			if e, ok := u.xt.Table[nr]; ok && e != nil {
				e.Object = out
			}
		}
	}
	return false
}

// poisoned is the net's whole judgement of an object on its own.
func (u *pageUnlink) poisoned(nr int, o types.Object) bool {
	switch v := o.(type) {
	case types.Dict:
		if u.onKept[nr] {
			return false
		}
		if u.onGone[nr] {
			return true
		}
		// An annotation listed on no page at all that names a dropped page as its own.
		if _, isAnnot := v["Subtype"]; isAnnot {
			if _, hasRect := v["Rect"]; hasRect {
				if pr, ok := v["P"].(types.IndirectRef); ok && u.dropped[pr.ObjectNumber.Value()] {
					return true
				}
			}
		}
	case types.Array:
		return u.deadDestArray(v)
	}
	return false
}

// deadDestArray is an explicit destination whose page is a dropped one — `[page /XYZ …]` and kin.
func (u *pageUnlink) deadDestArray(a types.Array) bool {
	if len(a) < 2 {
		return false
	}
	pr, ok := a[0].(types.IndirectRef)
	if !ok || !u.dropped[pr.ObjectNumber.Value()] {
		return false
	}
	_, isName := a[1].(types.Name)
	return isName
}

// dict walks one dictionary, cutting, splicing or recursing per key.
func (u *pageUnlink) dict(d types.Dict, depth int) {
	if d == nil || depth > maxDirectDepth {
		return
	}
	if dest, has := d["Dest"]; has && !destReachesAKeptPage(u.xt, dest, u.kept) {
		delete(d, "Dest")
	}
	// `/Next` is not read here: it is an action key only inside an action, which `chain` walks, and an
	// outline item's `/Next` is its sibling.
	if v, has := d["A"]; has {
		if live := u.chain(v, map[int]bool{}, 0); live == nil {
			delete(d, "A")
		} else {
			d["A"] = live
		}
	}
	if aa := derefDict(u.xt, d["AA"]); aa != nil {
		for trig, v := range aa {
			if live := u.chain(v, map[int]bool{}, 0); live == nil {
				delete(aa, trig)
			} else {
				aa[trig] = live
			}
		}
		if len(aa) == 0 {
			delete(d, "AA")
		}
	}
	for k, v := range d {
		switch vv := v.(type) {
		case types.IndirectRef:
			if u.cut(vv, depth+1) {
				delete(d, k)
			}
		case types.Dict:
			u.dict(vv, depth+1)
		case types.Array:
			if u.deadDestArray(vv) {
				delete(d, k)
				continue
			}
			if out, changed := u.array(vv, depth+1); changed {
				d[k] = out
			}
		}
	}
}

// array walks one array, dropping every element that is cut.
func (u *pageUnlink) array(a types.Array, depth int) (types.Array, bool) {
	if depth > maxDirectDepth {
		return a, false
	}
	var out types.Array
	changed := false
	for i, e := range a {
		drop := false
		switch v := e.(type) {
		case types.IndirectRef:
			drop = u.cut(v, depth+1)
		case types.Dict:
			u.dict(v, depth+1)
		case types.Array:
			if u.deadDestArray(v) {
				drop = true
			} else if inner, ch := u.array(v, depth+1); ch {
				a[i] = inner
				e = inner
			}
		}
		if drop {
			if !changed {
				out = append(types.Array{}, a[:i]...)
				changed = true
			}
			continue
		}
		if changed {
			out = append(out, e)
		}
	}
	if !changed {
		return a, false
	}
	return out, true
}

// chain returns an action (a dictionary, or an array of them, under `/A`, `/Next` or an `/AA` trigger)
// with every GoTo that does not reach a kept page spliced out, its own `/Next` taking its place. nil
// means nothing in the chain survived. A dictionary already on this chain is a cycle and is returned
// as it is.
func (u *pageUnlink) chain(o types.Object, onChain map[int]bool, depth int) types.Object {
	if depth > maxDirectDepth {
		return o
	}
	if r, ok := o.(types.IndirectRef); ok {
		if onChain[r.ObjectNumber.Value()] {
			return o
		}
		onChain[r.ObjectNumber.Value()] = true
	}
	v, err := u.xt.Dereference(o)
	if err != nil || v == nil {
		return o
	}
	switch act := v.(type) {
	case types.Array:
		out := make(types.Array, 0, len(act))
		for _, e := range act {
			if live := u.chain(e, onChain, depth+1); live != nil {
				out = append(out, live)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case types.Dict:
		var next types.Object
		if n, has := act["Next"]; has {
			if next = u.chain(n, onChain, depth+1); next == nil {
				delete(act, "Next")
			} else {
				act["Next"] = next
			}
		}
		if nameVal(act, "S") == "GoTo" {
			if d, has := act["D"]; has && !destReachesAKeptPage(u.xt, d, u.kept) {
				return next
			}
		}
		return o
	}
	return o
}
