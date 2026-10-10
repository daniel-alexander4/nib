package pdfread

import (
	"bytes"
	"reflect"
	"runtime"
	"sync"
	"weak"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The one door to pdfcpu's writer, and what it puts back first — ADR-129, `/pending 842`.
//
// pdfcpu's relaxed validator does not only judge a document, it CHANGES it: a font it cannot validate is not
// refused, it is taken out of the `/Font` dictionary that names it (`validate/font.go` `fixFontObjNr`, v0.13.0 —
// the entry is set to nil, or pointed at another font of the same `/BaseFont` where the dictionary holds one).
// The fonts it does this to: a Type 0 font whose descendant fails validation (`:756-760`), a font written
// directly in the dictionary, and a reference to an object that is not a font dictionary (`:1114`, `:1132`).
// pdfcpu's writer leaves a nil entry out, so every operation nib performed on such a document wrote it WITHOUT a
// font its content still names — a deletion nobody asked for — and `pdfops.ContentDigest`, which counts a
// resource dictionary's entries, moved under both of its rules. Measured on veraPDF's `7.21.3.2-t01-fail-a.pdf`:
// `<</C2_0 10 0 R/TT0 11 0 R>>` in the file, `<</TT0 11 0 R>>` after `AddNotes`.
//
// **So the entry is put back for the WRITE, and only for the write.** `validated` finds each `/Font` entry
// the validator changed (`rememberValidatorLosses`); `Write` restores it immediately before pdfcpu serialises the context. Everything
// between — the optimize pass, the operation, every reader — sees what it has always seen, the validator's
// reading: nothing here hands a font pdfcpu refused to code that reads fonts. That is what makes this safe where
// `ReadOptimizedOrRefuseSettingAside` is not for a written context: there, an entry nothing vouched for is read
// and acted on; here it is only carried, byte for byte what the file held, by a writer that does not interpret
// it. An entry the operation itself changed or removed is left as the operation left it, and so is one whose
// target is no longer the object it was.
//
// **A context whose objects are about to MOVE is restored before the move** (`PutBackValidatorLosses`): pdfcpu's
// merge renumbers a source's references as it appends them, `CutPage` copies pages into a new context and
// `NUpFromPDF` copies each page's resources into the form it builds, so an entry remembered from before would
// name another object afterwards, or sit in a dictionary nothing draws through. Left unrestored such a context is
// merely written as before — an entry is never restored into a numbering it did not come from.
//
// `TestEveryWriteRoutesThroughTheDoor` (in this package) is the guard: outside this package no source file may
// name pdfcpu's writers, and a function that moves a context's objects must restore them first.

// Write is `api.WriteContext` into memory, after putting back what pdfcpu's validator took out of ctx.
//
// **Declared gap, and it is the WRITER's, so nothing here restores it (`/pending 655`).** pdfcpu writes the catalog
// and each page dictionary whole, then follows a fixed list of their keys (`write.go:276-322`,
// `writePages.go:75-102`, v0.13.0) and writes nothing an unlisted key names. `/AF` — an associated file, ISO
// 32000-2 §14.13 — is on neither list: after ANY write through this door a catalog's or a page's `/AF` is still
// there and the file specification it names is not, unless /Names /EmbeddedFiles names the same object (PDF/A-3
// requires that, and then it is carried whole). A structure element's `/AF` is carried. The objects are in the
// context when this function is called; pdfcpu's own unvalidated read and write lose them identically. Measured
// and held by `pdfops`' `TestWhatARewriteDoesToAnAssociatedFile`. The same holds for any other catalog or page
// key off those lists — unmeasured which occur.
func Write(ctx *model.Context) ([]byte, error) {
	PutBackValidatorLosses(ctx)
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// lostFont is one `/Font` dictionary entry the validator changed: the dictionary, the name, what the document
// wrote there, what the validator left, and the cross-reference entry the document's own reference named (nil for
// a font written directly, or a reference to nothing).
type lostFont struct {
	dict   types.Dict
	key    string
	was    types.Object
	left   types.Object
	target *model.XRefTableEntry
}

// pageLoss is a lostFont as one page inherits or holds it. pdfcpu's optimize pass gives every page a `/Resources`
// of its own, built new (`optimizeResourceDicts`, optimize.go:1592): the nil entry is copied into a dictionary no
// read saw, so the page's entry is found again from the page, whose own dictionary the pass keeps.
type pageLoss struct {
	page types.Dict
	lostFont
}

// ctxLosses is what the validator changed in one context: by the dictionary it was in, and by page.
type ctxLosses struct {
	fonts []lostFont
	pages []pageLoss
}

// maxLossPageDepth bounds the page-tree walk below; the validator has already refused a deeper tree.
const maxLossPageDepth = 128

// losses holds, per context, what the validator changed. Keyed weakly: a context nobody writes is collected with
// its record (the record holds the context's dictionaries, never the context).
var losses = struct {
	sync.Mutex
	m map[weak.Pointer[model.Context]]ctxLosses
}{m: map[weak.Pointer[model.Context]]ctxLosses{}}

// maxFontDictNesting bounds how far eachFontDict follows DIRECT objects inside one another; every indirect object
// is its own starting point, so the bound is on nesting within one object and not on the document.
const maxFontDictNesting = 64

// eachFontDict calls visit with every dictionary a `/Font` key names in ctx and where it is: the object number
// and the path of keys and indexes from that object (none where the dictionary is its own object, and then
// once). Each object of the cross-reference table is a starting point and direct dictionaries and arrays inside
// it are followed. An object still held undecoded in an object stream is not looked into — nothing has read it,
// so nothing has changed it — and looking would decode it, which is not this walk's to do.
func eachFontDict(ctx *model.Context, visit func(objNr int, path []any, fd types.Dict)) {
	seen := map[int]bool{}
	// One path, pushed and popped: a visit that keeps it copies it (`rememberValidatorLosses`).
	var path []any
	var walk func(n int, o types.Object)
	walk = func(n int, o types.Object) {
		if len(path) > maxFontDictNesting {
			return
		}
		switch v := o.(type) {
		case types.Dict:
			if fonts, ok := v["Font"]; ok {
				if ref, isRef := fonts.(types.IndirectRef); isRef {
					m := ref.ObjectNumber.Value()
					if e := ctx.XRefTable.Table[m]; !seen[m] && e != nil && !e.Free {
						seen[m] = true
						if fd, ok := e.Object.(types.Dict); ok {
							visit(m, nil, fd)
						}
					}
				} else if fd, ok := fonts.(types.Dict); ok {
					path = append(path, "Font")
					visit(n, path, fd)
					path = path[:len(path)-1]
				}
			}
			for k, el := range v {
				switch el.(type) {
				case types.Dict, types.Array:
					path = append(path, k)
					walk(n, el)
					path = path[:len(path)-1]
				}
			}
		case types.StreamDict:
			walk(n, v.Dict)
		case *types.StreamDict:
			if v != nil {
				walk(n, v.Dict)
			}
		case types.Array:
			for i, el := range v {
				switch el.(type) {
				case types.Dict, types.Array:
					path = append(path, i)
					walk(n, el)
					path = path[:len(path)-1]
				}
			}
		}
	}
	for n, e := range ctx.XRefTable.Table {
		if e != nil && !e.Free && e.Object != nil {
			walk(n, e.Object)
		}
	}
}

// objectAt is the object at path inside object objNr of ctx, or nil.
func objectAt(ctx *model.Context, objNr int, path []any) types.Object {
	o, err := ctx.Dereference(*types.NewIndirectRef(objNr, 0))
	if err != nil {
		return nil
	}
	unwrap := func(o types.Object) types.Object {
		switch v := o.(type) {
		case types.StreamDict:
			return v.Dict
		case *types.StreamDict:
			if v != nil {
				return v.Dict
			}
		}
		return o
	}
	for _, step := range path {
		switch v := unwrap(o).(type) {
		case types.Dict:
			k, ok := step.(string)
			if !ok {
				return nil
			}
			o = v[k]
		case types.Array:
			i, ok := step.(int)
			if !ok || i >= len(v) {
				return nil
			}
			o = v[i]
		default:
			return nil
		}
	}
	return unwrap(o)
}

// rememberValidatorLosses records every `/Font` entry the validation of ctx changed, pdf being the bytes ctx was
// read from.
//
// **Looked for afterwards, and the document read again only where something shows.** The validator's two changes
// both leave a mark in the dictionary: an entry it emptied is nil, and one it pointed at another font shares its
// reference with that font's own entry. A dictionary with neither is as it was read, and that is every
// dictionary of nearly every document: the read pays one walk of the objects it already holds (measured in
// ADR-129). Where a dictionary shows either, the bytes are parsed a second time, unvalidated, and each of its
// entries compared with what the file wrote at the same place — a `null` the document wrote itself, and two
// names the document bound to one font, are told apart from a loss there.
func rememberValidatorLosses(ctx *model.Context, pdf []byte) {
	type suspect struct {
		objNr int
		path  []any
		fd    types.Dict
	}
	var suspects []suspect
	eachFontDict(ctx, func(objNr int, path []any, fd types.Dict) {
		refs := map[types.IndirectRef]bool{}
		for _, f := range fd {
			ref, isRef := f.(types.IndirectRef)
			if f == nil || (isRef && refs[ref]) {
				suspects = append(suspects, suspect{objNr, append([]any(nil), path...), fd})
				return
			}
			if isRef {
				refs[ref] = true
			}
		}
	})
	if len(suspects) == 0 {
		return
	}
	written, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return
	}
	var lost []lostFont
	for _, s := range suspects {
		od, ok := objectAt(written, s.objNr, s.path).(types.Dict)
		if !ok {
			continue
		}
		for key, cur := range s.fd {
			was, present := od[key]
			if !present {
				continue
			}
			if was == nil {
				continue // `null` as the document wrote it: nothing was lost
			}
			wasRef, wasIsRef := was.(types.IndirectRef)
			switch c := cur.(type) {
			case nil:
			case types.IndirectRef:
				if wasIsRef && c == wasRef {
					continue
				}
			default:
				continue
			}
			l := lostFont{dict: s.fd, key: key, was: was, left: cur}
			if wasIsRef {
				l.target = ctx.XRefTable.Table[wasRef.ObjectNumber.Value()]
			}
			lost = append(lost, l)
		}
	}
	if len(lost) == 0 {
		return
	}
	// Which page each belongs to, while the dictionaries are still the ones the document was read into: a page
	// draws with the nearest dictionary on its way to the root that holds the name.
	in := map[uintptr][]lostFont{}
	for _, l := range lost {
		id := reflect.ValueOf(l.dict).Pointer()
		in[id] = append(in[id], l)
	}
	var pages []pageLoss
	var walk func(o types.Object, chain []types.Dict, depth int)
	walk = func(o types.Object, chain []types.Dict, depth int) {
		d, err := ctx.DereferenceDict(o)
		if err != nil || d == nil || depth > maxLossPageDepth {
			return
		}
		if res, _ := ctx.DereferenceDict(d["Resources"]); res != nil {
			if fd, _ := ctx.DereferenceDict(res["Font"]); fd != nil {
				chain = append(chain[:len(chain):len(chain)], fd)
			}
		}
		if kids, _ := ctx.DereferenceArray(d["Kids"]); kids != nil {
			for _, k := range kids {
				walk(k, chain, depth+1)
			}
			return
		}
		nearer := map[string]bool{}
		for i := len(chain) - 1; i >= 0; i-- {
			for _, l := range in[reflect.ValueOf(chain[i]).Pointer()] {
				if !nearer[l.key] {
					pages = append(pages, pageLoss{d, l})
				}
			}
			for k := range chain[i] {
				nearer[k] = true
			}
		}
	}
	if root, err := ctx.XRefTable.Catalog(); err == nil && root != nil {
		walk(root["Pages"], nil, 0)
	}
	key := weak.Make(ctx)
	losses.Lock()
	losses.m[key] = ctxLosses{lost, pages}
	losses.Unlock()
	runtime.AddCleanup(ctx, func(k weak.Pointer[model.Context]) {
		losses.Lock()
		delete(losses.m, k)
		losses.Unlock()
	}, key)
}

// ValidatorLosses is how many `/Font` entries pdfcpu's validator changed in ctx that have not been put back.
func ValidatorLosses(ctx *model.Context) int {
	losses.Lock()
	defer losses.Unlock()
	return len(losses.m[weak.Make(ctx)].fonts)
}

// PutBackValidatorLosses restores, in ctx, every `/Font` entry pdfcpu's validator changed as it read it, where the
// entry is still as the validator left it and its target is still the object the document named. `Write` calls
// it; a caller does only where ctx's objects are about to be renumbered or copied into another context (the
// file comment). Once: what is put back is forgotten.
func PutBackValidatorLosses(ctx *model.Context) {
	key := weak.Make(ctx)
	losses.Lock()
	lost := losses.m[key]
	delete(losses.m, key)
	losses.Unlock()
	// A page first, where the optimize pass gave it a dictionary of its own. A dictionary on a page-tree node
	// that every page under it has been given a copy of is then NOT restored: no page draws through it any
	// more, and a second reference to the font would be one the document never had — pdfcpu's validator takes a
	// font out only where it meets it first, so the next read would lose it on one path and keep it on the other.
	type lossID struct {
		dict uintptr
		key  string
	}
	idOf := func(l lostFont) lossID { return lossID{reflect.ValueOf(l.dict).Pointer(), l.key} }
	superseded := map[lossID]bool{}
	for _, p := range lost.pages {
		id := idOf(p.lostFont)
		if _, met := superseded[id]; !met {
			superseded[id] = true
		}
		res, _ := ctx.DereferenceDict(p.page["Resources"])
		if res == nil {
			superseded[id] = false // the page still inherits it
			continue
		}
		fd, _ := ctx.DereferenceDict(res["Font"])
		if fd != nil && reflect.ValueOf(fd).Pointer() == id.dict {
			superseded[id] = false // the page's own dictionary is the one the document wrote
			continue
		}
		if fd != nil {
			p.putBack(ctx, fd)
		}
	}
	for _, l := range lost.fonts {
		if !superseded[idOf(l)] {
			l.putBack(ctx, l.dict)
		}
	}
}

// putBack restores l's entry in fd where it is still as the validator left it.
func (l lostFont) putBack(ctx *model.Context, fd types.Dict) {
	cur, present := fd[l.key]
	if !present {
		return // the operation removed the entry, or the page's content never names it
	}
	if curRef, ok := cur.(types.IndirectRef); ok {
		if leftRef, ok := l.left.(types.IndirectRef); !ok || leftRef != curRef {
			return // the operation rebound the name (or it is already back)
		}
	} else if cur != nil || l.left != nil {
		return
	}
	if was, ok := l.was.(types.IndirectRef); ok {
		e := ctx.XRefTable.Table[was.ObjectNumber.Value()]
		if e != l.target || (e != nil && e.Free) {
			return // that number is no longer the object the document named
		}
	}
	fd[l.key] = l.was
}
