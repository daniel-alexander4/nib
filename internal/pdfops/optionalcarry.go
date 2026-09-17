package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The optional-content properties, carried through a page selection — `/pending 525`.
//
// # Dropping this key does not lose a layer, it REVEALS one, and that is the whole reason
//
// Every other entry on `pageselect.go`'s drop list costs the reader something: a bookmark, a page
// label, a name. `/OCProperties` is the one that costs the AUTHOR something, and in the direction
// nobody checks. A layer's content lives in the page's own content stream, bracketed
// `/OC /L0 BDC … EMC`, and the page's `/Resources /Properties` names the group — all of which a
// selection carries verbatim, because it carries the page dictionary whole. The only thing that
// says the group is switched OFF is `/OCProperties /D /OFF` in the catalog. Take the catalog key
// away and, per ISO 32000-1 8.11, a reader has no optional content to recognise, so it draws the
// marked content like any other.
//
// **Measured, not argued** (2026-09-16, a three-page fixture whose page 1 carries a 300×200pt black
// box inside a group listed in `/OFF`, subset to pages 1 and 3, page 1 rasterised at 40 dpi):
//
//	                        source      after Collect("1","3")
//	  Ghostscript 10.02.1    39 dark px   18,855 dark px
//	  poppler pdftoppm        6 dark px   18,598 dark px
//
// Two independent renderers, the same answer: the box the author hid is printed on the page the
// user is about to hand over. nib's own `StripActive` deletes this key for exactly that effect and
// calls it "the intended hidden-content reveal" (`scan.go:270-273`) — intended THERE, on a door the
// user reached by asking to strip active content. `Collect` is "keep pages 1 and 3", and
// `RedactPages` builds its runs of untouched pages through `collectWithoutStructure`, so today a
// redaction reveals a hidden layer on every page it did not touch.
//
// So the carry is NOT gated on the structure carry the way the outline's is. `/Outlines` is gated
// because an outline title names content a raster destroyed; this key hides content, and the doors
// that must not carry a description of destroyed content are precisely the doors where revealing it
// is worst.
//
// # What it carries is the source's own subtree, and the refusal is reachability, not a key list
//
// `outlineItemKeys` is an allowlist because an outline item can hold a `/A` action or a `/SE`
// structure reference — a whole family of things that could ride out. `/OCProperties` can do exactly
// one harmful thing, and it is checkable directly: an indirect reference in it that resolves to a
// page this selection DROPPED puts that page's dictionary, and therefore its `/Contents`, back into
// the output, because pdfcpu writes by reachability. That is `pageselect.go`'s `/StructTreeRoot`
// hazard one key over, and the document it is written against is a crafted one rather than an
// ordinary one: measured over veraPDF's PDF_UA-1 corpus, the six files carrying `/OCProperties`
// hold `{OCGs, D{Name, Order, ON, AS, RBGroups}}` and their groups hold `{Type, Name, Usage}` —
// not one page reference anywhere.
//
// So the whole subtree is carried and the WALK is the guard. A subtree that reaches a dropped page
// is refused entire, which leaves exactly the document this primitive wrote before this change; a
// key list would have to enumerate `/Usage`'s sub-dictionaries and every future addition to them to
// say the same thing less completely.
//
// # What the carry leaves behind, named
//
// A group whose content lived only on dropped pages survives as an entry in the layers panel that
// toggles nothing. That is a dead label, not a disclosure, and pruning it is refused: deciding
// which groups are still referenced means finding every `/OC` in the kept pages — content-stream
// `BDC` operands, form XObject and annotation `/OC` keys, and membership dictionaries nested
// arbitrarily deep — and a walk that misses one drops the group that was hiding it, which is the
// defect this file exists to prevent, reintroduced by the cleanup.
//
// `/AS` is carried, and that is deliberately not what `correctOptionalContent` does. There it is
// removed from a configuration nib itself just wrote, where it was measured redundant with the
// `/ON` beside it (`optionalcontent.go:24-31`). Here it belongs to somebody else's document and a
// usage-application entry is one of the two ways a layer is hidden on screen, so removing it is the
// reveal again.

// carryOptionalContent returns the `/OCProperties` object a selection may keep, or nil where the
// document has none or its subtree reaches a page the selection dropped.
func carryOptionalContent(xt *model.XRefTable, root types.Dict, dropped map[int]bool) types.Object {
	o, ok := root["OCProperties"]
	if !ok {
		return nil
	}
	// A key that is not a readable dictionary is dropped rather than passed through: the allowlist's
	// default-deny applies to a malformed value the same as to an unknown key. A dictionary that IS
	// readable but incomplete cannot arrive here at all — every subset door reads through
	// `api.ReadValidateAndOptimize` (`scan.go:482`), and pdfcpu's validator requires `/D`
	// (`validate/optionalContent.go`, `optContentPropertiesDict required entry=D missing`), which is
	// what a red-proof that deleted `/D` from the carried subtree hit instead of its own assertion.
	if derefDict(xt, o) == nil {
		return nil
	}
	if reachesObject(xt, o, dropped, map[int]bool{}, 0) {
		return nil
	}
	return o
}

// reachesObject reports whether o, followed through every indirect reference, array and dictionary,
// arrives at an object number in want.
//
// **Every way of not knowing answers YES**, because the caller's question is "is this safe to
// carry" and the safe answer to "I could not tell" is the document this primitive wrote yesterday.
// So a reference that will not dereference, and a subtree deeper than the cap, both refuse the
// carry rather than allow it.
//
// The visited set is keyed on the object number and is shared across the whole walk: a
// configuration dictionary that names the same group in `/ON`, `/Order` and `/RBGroups` reaches it
// three times, and a malformed document can point two objects at each other.
func reachesObject(xt *model.XRefTable, o types.Object, want, seen map[int]bool, depth int) bool {
	if depth > 32 {
		return true
	}
	switch v := o.(type) {
	case types.IndirectRef:
		nr := v.ObjectNumber.Value()
		if want[nr] {
			return true
		}
		if seen[nr] {
			return false
		}
		seen[nr] = true
		d, err := xt.Dereference(v)
		if err != nil {
			return true
		}
		return reachesObject(xt, d, want, seen, depth+1)
	case types.Dict:
		for _, e := range v {
			if reachesObject(xt, e, want, seen, depth+1) {
				return true
			}
		}
	case types.StreamDict:
		return reachesObject(xt, v.Dict, want, seen, depth+1)
	case types.Array:
		for _, e := range v {
			if reachesObject(xt, e, want, seen, depth+1) {
				return true
			}
		}
	}
	return false
}
