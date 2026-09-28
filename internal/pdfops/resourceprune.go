package pdfops

import (
	"fmt"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Resource pruning — a page a selection KEEPS carries only the resources its drawing can name
// (`/pending 688`).
//
// # The leak this closes
//
// `selectPages` materializes inherited `/Resources` onto every kept page, and a producer that shares
// one resource dictionary across pages gives every page the same dictionary without inheritance at
// all. Either way the kept page NAMED every form, image and font any page of the document drew — and
// pdfcpu writes by reachability, so the form and image only the dropped page drew were written into
// the output. Measured on redaction (R5-1 of the P07 phase-close review): the raster replaced page 1
// and page 1's form text was still in the file, decodable by anyone. It is `pageselect.go`'s own
// header hazard — "delete page 4 ships page 4's text" — reached through the resource dictionary
// instead of the structure tree, so it is closed for every subset door and not for redaction alone.
// The form's own `/AcroForm /DR` is the same road from the catalog side, and a producer points it at
// the pages' shared dictionary too: `pruneDefaultResources` rebuilds it (`/pending 705`).
//
// # What "can name" means, and why it over-approximates
//
// A resource is named from a content stream by a name operand: `Do`, `Tf`, `gs`, `sh`, `cs`/`scn`,
// `BDC`'s property list, and an inline image's `/CS`. Rather than model each operator, EVERY name
// token in the stream is collected, and a resource entry survives when its key is one of them. That
// can keep an entry the page does not strictly draw (`/Fm1` appearing as a `BDC` tag, say) and can
// never drop one it does, which is the direction that matters to a user: an over-kept entry is a
// resource that was already in the file, while an under-kept one blanks part of a page nobody asked
// to change. The tokenizer skips strings, so text that merely SPELLS a name keeps nothing.
//
// Names reach a resource dictionary from more places than the page's own content, and each is walked:
//
//   - a form XObject, tiling pattern or Type 3 font with no `/Resources` of its own reads its
//     CALLER's, so its names are the caller's names too (to a fixed point);
//   - one WITH `/Resources` of its own is an owner in its own right and is pruned against its own
//     content, once, whichever page reached it — the shape where a producer points every form's
//     `/Resources` at the page's shared dictionary is the same leak one level down;
//   - an annotation appearance stream, and an ExtGState's soft-mask group, are owners the same way.
//
// # Default-deny, like the catalog
//
// Only the eight resource categories ISO 32000-1 Table 33 defines survive. `/ProcSet` names no
// resource and is kept whole; the other seven are filtered; a key outside the table is dropped, for
// `catalogAllowlist`'s reason — a key nobody enumerated must not carry a reference out.
//
// # When a stream cannot be read
//
// Its names are unknowable, so its owner cannot be pruned. `refuseUnreadable` — redaction — refuses
// the operation, because keeping the owner's dictionary keeps whatever the redacted page shared with
// it. `keepUnreadable` — every other door — leaves that owner's dictionary as it was, which is what
// every subset did before this existed; a page whose content no reader can decode is not made
// unremovable by it.

// unreadablePolicy says what a selection does with a kept page whose drawing it cannot read.
type unreadablePolicy int

const (
	// keepUnreadable leaves that page's resources unpruned: the behaviour before `/pending 688`.
	keepUnreadable unreadablePolicy = iota
	// refuseUnreadable fails the operation. Redaction's policy: a prune it cannot perform is a leak
	// it cannot rule out.
	refuseUnreadable
)

// resourceCategories are Table 33's keys. ProcSet is the one kept whole.
var resourceCategories = map[string]bool{
	"ExtGState": true, "ColorSpace": true, "Pattern": true, "Shading": true,
	"XObject": true, "Font": true, "Properties": true, "ProcSet": true,
}

// resourcePruner prunes the resource dictionaries of one selection's kept pages. Its memos make the
// work proportional to the distinct streams reached, not to the pages naming them.
type resourcePruner struct {
	xt     *model.XRefTable
	policy unreadablePolicy
	// names memoizes each indirect stream's name set (nil value: unreadable).
	names map[int]map[string]bool
	// owned records the owners (by object number) whose own /Resources were already pruned, or are
	// being pruned — the second makes a form whose resources reach itself terminate.
	owned map[int]bool
}

// pruneKeptResources gives every kept page a fresh, direct `/Resources` holding only what that page's
// drawing can name. Pages are distinct dictionaries by the time this runs (a repeat is a clone), and
// a fresh dictionary per page is what lets two pages that shared one keep different entries.
//
// source[i] is the source page number of pages[i], so a refusal names the page the user knows.
func pruneKeptResources(xt *model.XRefTable, pages []types.Dict, source []int, policy unreadablePolicy) error {
	p := &resourcePruner{xt: xt, policy: policy, names: map[int]map[string]bool{}, owned: map[int]bool{}}
	for i, page := range pages {
		if err := p.page(page, source[i]); err != nil {
			return err
		}
	}
	return nil
}

func (p *resourcePruner) unreadable(what string) error {
	if p.policy == refuseUnreadable {
		return fmt.Errorf("pdfops: %s cannot be read, so nib cannot tell which of the document's "+
			"shared images, forms and fonts it draws — refusing rather than keeping them all", what)
	}
	return nil
}

func (p *resourcePruner) page(page types.Dict, n int) error {
	used, ok := p.pageContentNames(page)
	if !ok {
		return p.unreadable(fmt.Sprintf("page %d", n))
	}
	// Appearance streams: one without its own /Resources reads the page's (the reading readers apply);
	// one with its own is an owner.
	for _, a := range derefArray(p.xt, page["Annots"]) {
		ad := derefDict(p.xt, a)
		if ad == nil {
			continue
		}
		for _, s := range appearanceStreams(p.xt, ad) {
			if !p.child(s, used) {
				return p.unreadable(fmt.Sprintf("an annotation appearance on page %d", n))
			}
		}
	}
	res := derefDict(p.xt, page["Resources"])
	if res == nil {
		return nil
	}
	pruned, ok := p.prune(res, used)
	if !ok {
		return p.unreadable(fmt.Sprintf("something page %d draws", n))
	}
	page["Resources"] = pruned
	return nil
}

// prune returns a fresh dictionary holding the entries of res that used names, after closing used
// over the resource-less children those entries reach. ok is false when a child could not be read.
func (p *resourcePruner) prune(res types.Dict, used map[string]bool) (types.Dict, bool) {
	// Close over children: a newly used name can reach a resource-less form that names more.
	walked := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, cat := range []string{"XObject", "Pattern", "Font", "ExtGState"} {
			sub := derefDict(p.xt, res[cat])
			for name, obj := range sub {
				key := cat + "/" + name
				if !used[name] || walked[key] {
					continue
				}
				walked[key] = true
				before := len(used)
				if !p.child(obj, used) {
					return nil, false
				}
				if len(used) != before {
					changed = true
				}
			}
		}
	}
	out := types.Dict{}
	for k, v := range res {
		if !resourceCategories[k] {
			continue
		}
		if k == "ProcSet" {
			out[k] = v
			continue
		}
		sub := derefDict(p.xt, v)
		if sub == nil {
			continue
		}
		kept := types.Dict{}
		for name, obj := range sub {
			if used[name] {
				kept[name] = obj
			}
		}
		if len(kept) > 0 {
			out[k] = kept
		}
	}
	return out, true
}

// child handles one object a used resource entry (or an annotation) reaches. A drawing object with no
// /Resources adds its names to the caller's set; one with /Resources is pruned as an owner. Reports
// false when a stream it had to read could not be.
func (p *resourcePruner) child(obj types.Object, used map[string]bool) bool {
	o, err := p.xt.Dereference(obj)
	if err != nil || o == nil {
		return true
	}
	switch v := o.(type) {
	case types.StreamDict:
		// Every stream reached here that is not an image is a drawing: a form XObject, a tiling
		// pattern, an appearance stream, a soft-mask group. An image names nothing.
		if nameVal(v.Dict, "Subtype") == "Image" {
			return true
		}
		names, ok := p.streamNames(obj)
		if !ok {
			return false
		}
		return p.ownerOrMerge(obj, v.Dict, names, used)
	case types.Dict:
		// A Type 3 font draws through its CharProcs.
		if nameVal(v, "Subtype") == "Type3" {
			names := map[string]bool{}
			for _, proc := range derefDict(p.xt, v["CharProcs"]) {
				pn, ok := p.streamNames(proc)
				if !ok {
					return false
				}
				for n := range pn {
					names[n] = true
				}
			}
			return p.ownerOrMerge(obj, v, names, used)
		}
		// An ExtGState's soft mask is a transparency group drawn from its own content.
		if sm := derefDict(p.xt, v["SMask"]); sm != nil {
			if g, has := sm["G"]; has {
				return p.child(g, used)
			}
		}
		return true
	}
	return true
}

// ownerOrMerge prunes the holder's own /Resources against names, or, where it has none, adds names to
// the caller's set.
func (p *resourcePruner) ownerOrMerge(ref types.Object, holder types.Dict, names, used map[string]bool) bool {
	res := derefDict(p.xt, holder["Resources"])
	if res == nil {
		for n := range names {
			used[n] = true
		}
		return true
	}
	if ir, ok := ref.(types.IndirectRef); ok {
		nr := ir.ObjectNumber.Value()
		if p.owned[nr] {
			return true
		}
		p.owned[nr] = true
	}
	own := make(map[string]bool, len(names))
	for n := range names {
		own[n] = true
	}
	pruned, ok := p.prune(res, own)
	if !ok {
		return false
	}
	holder["Resources"] = pruned
	return true
}

// pageContentNames is the name set of a page's /Contents — one stream or an array of them. A page
// with no /Contents draws nothing and names nothing.
func (p *resourcePruner) pageContentNames(page types.Dict) (map[string]bool, bool) {
	c, has := page["Contents"]
	if !has || c == nil {
		return map[string]bool{}, true
	}
	o, err := p.xt.Dereference(c)
	if err != nil {
		return nil, false
	}
	if o == nil {
		return map[string]bool{}, true
	}
	arr, isArr := o.(types.Array)
	if !isArr {
		return p.streamNames(c)
	}
	all := map[string]bool{}
	for _, s := range arr {
		names, ok := p.streamNames(s)
		if !ok {
			return nil, false
		}
		for n := range names {
			all[n] = true
		}
	}
	return all, true
}

// streamNames decodes one stream and collects its name tokens, memoized per object number.
func (p *resourcePruner) streamNames(obj types.Object) (map[string]bool, bool) {
	nr := 0
	if ir, ok := obj.(types.IndirectRef); ok {
		nr = ir.ObjectNumber.Value()
		if names, seen := p.names[nr]; seen {
			return names, names != nil
		}
	}
	sd, _, err := p.xt.DereferenceStreamDict(obj)
	var names map[string]bool
	if err == nil && sd != nil {
		switch {
		case len(sd.FilterPipeline) == 0:
			names = contentNames(sd.Raw)
		case sd.Decode() == nil:
			names = contentNames(sd.Content)
		}
	}
	if nr != 0 {
		p.names[nr] = names
	}
	return names, names != nil
}

// contentNames is every name token in a decoded content stream, decoded as pdfcpu decodes a
// dictionary key (`#xx` escapes), so the two compare. An inline image's dictionary is read up to its
// `ID`: its `/CS` may name a colour space resource, and the payload after `ID` is binary.
func contentNames(src []byte) map[string]bool {
	out := map[string]bool{}
	var collect func(b []byte, stopAtID bool)
	collect = func(b []byte, stopAtID bool) {
		for _, tk := range contentstream.Tokenize(b) {
			raw := tk.Bytes(b)
			switch tk.Kind {
			case contentstream.Operand:
				if len(raw) > 1 && raw[0] == '/' {
					n := string(raw[1:])
					if d, err := types.DecodeName(n); err == nil {
						n = d
					}
					out[n] = true
				}
			case contentstream.Operator:
				if stopAtID && string(raw) == "ID" {
					return
				}
			case contentstream.InlineImage:
				if len(raw) > 2 {
					collect(raw[2:], true)
				}
			}
		}
	}
	collect(src, false)
	return out
}

// appearanceStreams is every stream an annotation's /AP holds: /N, /R and /D, each a stream or a
// dictionary of appearance states.
func appearanceStreams(xt *model.XRefTable, annot types.Dict) []types.Object {
	ap := derefDict(xt, annot["AP"])
	if ap == nil {
		return nil
	}
	var out []types.Object
	for _, k := range []string{"N", "R", "D"} {
		v, has := ap[k]
		if !has || v == nil {
			continue
		}
		o, err := xt.Dereference(v)
		if err != nil || o == nil {
			continue
		}
		switch s := o.(type) {
		case types.StreamDict:
			out = append(out, v)
		case types.Dict:
			for _, st := range s {
				out = append(out, st)
			}
		}
	}
	return out
}
