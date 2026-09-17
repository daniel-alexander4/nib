package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The document outline, pruned onto the pages a selection kept.
//
// # What changed, and why the old decision was right when it was made
//
// `pdfops.go`'s subset header recorded dropping `/Outlines` as a DECISION rather than an oversight:
// an outline destination names a page OBJECT, and the old `api.Collect` built a fresh context in
// which every object number was new, so a copied destination "sends the reader to the wrong place —
// worse than not having one, because it is wrong rather than absent". That was exactly true of the
// implementation it described. Since P02.S04a the selection rewrites the page tree IN PLACE
// (`pageselect.go`), so a kept page's object number is the number it always had — and a destination
// naming a kept page by indirect reference is still correct after an arbitrary reorder, with no
// remap at all. The drop outlived its reason.
//
// It matters because the outline is not somebody else's metadata here: the user AUTHORS it in nib
// (`web/index.html:1363-1373`, `SetOutline`), reads it in a sidebar tab called "Jump to Section"
// (`web/index.html:193`), and exports through "Split by bookmarks" (`SplitByBookmarks`). Driven
// before this file existed: a four-bookmark document through `Collect("4","3","1")` came back with
// no `/Outlines` key and `Outline()` returning an empty list, and nothing anywhere told the user.
//
// # nib's own bookmarks are NAMED destinations, which is the whole ordering problem
//
// Measured, not assumed. pdfcpu's `AddBookmarks` — which is what `SetOutline` calls — writes each
// item's `/Dest` as a STRING keying the `/Names` `/Dests` name tree (`bookmark.go:429`), never as an
// explicit `[page /XYZ …]` array. So for the documents nib itself produces, every outline
// destination is one indirection away from the page, through the very tree `pruneNames` is
// separately pruning by the same rule.
//
// **The two are kept in agreement by sharing the predicate, not by ordering.** A name is resolved
// here to the exact object `pruneNames` will hand `destNamesAKeptPage`, and the same function
// decides both. Ordering alone could not do it: pruning names first would leave this code unable to
// tell "the page went" from "the name went", and pruning the outline first against a different
// predicate is how a bookmark whose destination survived gets dropped, or one whose destination went
// gets kept pointing at nothing.
//
// A name that resolves only through the catalog's PDF-1.1 `/Dests` dictionary (`xt.Dests`) is
// treated as GONE, because it is: `/Dests` is not on `catalogAllowlist` and `pruneNames` nils the
// cache, so the table the name needs does not survive this operation whatever the page did.
//
// # The keep rule is closed upward, and that is what makes the repair tractable
//
// **A node survives if it can still take the reader somewhere** — its own destination names a kept
// page, or one of its descendants' does. Because a node with a surviving descendant survives too,
// the kept set is subtree-closed: no node is ever orphaned and no node is ever RE-PARENTED, so
// repairing `/First`, `/Last`, `/Next`, `/Prev` and `/Parent` is filtering each node's own child
// list, never promoting a grandchild. The whole class of "the tree came out a different shape" bugs
// is closed by the rule rather than by careful code.
//
// Two consequences worth naming. A parent whose every child was dropped but whose own destination
// survives stays, as a leaf. A node with no destination and no surviving descendant is dropped; it
// navigates nowhere and contains nothing that does.
//
// # A parent kept for its children INHERITS a destination, and that is not a nicety
//
// The obvious answer for a parent whose own destination went but whose children survived is to keep
// it as a title with no destination. **Read pdfcpu's reader and it is wrong**: `bookmarksForOutlineItem`
// does `continue` on an item whose destination does not resolve (`bookmark.go:248-251`) — and the
// `continue` is BEFORE the descent into `/First`, so a destination-less item hides its entire
// surviving subtree from `Outline()` and therefore from "Jump to Section" and `SplitByBookmarks`.
// Keeping the parent politely would have re-created this item's own bug one level in: delete the
// page a chapter heading points at and its sections vanish from the panel.
//
// So such a node takes the destination of its first surviving descendant, which is the right answer
// on the merits as well as the working one: when a section's own opening page is gone, that section
// now starts at the first surviving thing inside it. The invariant this buys is worth more than the
// special case it removes — **every node that survives has a destination that resolves to a kept
// page**, so "survives" and "has somewhere to go" are the same predicate rather than two.
//
// What is never done is keep the stale one. An explicit destination names the dropped page by
// indirect reference, pdfcpu writes by reachability, and a kept node still holding one would put the
// removed page's dictionary and its `/Contents` back into the output — the identical hazard
// `unlinkDestinations` exists for one door over.
//
// # The surviving order is the SOURCE's, not the new page order
//
// A reorder can leave the outline descending: `Collect("4","3","1")` over four bookmarked pages
// gives Alpha→3, Beta→2, Gamma→1, each correct and the list upside down. Re-sorting is refused
// because the outline is an authored HIERARCHY — a sibling order that means chapter-then-section,
// and a child that may now precede its parent's page — and rewriting it is a claim about the
// document's structure that a page permutation does not make. The one consumer that needs ascending
// order already sorts for itself: `SplitByBookmarks` sorts by `PageFrom` (`pdfops.go:662`) and
// guards both an unresolved bookmark and an empty span, which is why it is safe against this and
// was before.
//
// # No action dictionary survives, and that is deliberate
//
// An outline item may carry its navigation in `/A` rather than `/Dest`, and an action can be
// `/Launch`, `/JavaScript`, `/SubmitForm` or `/GoToR` — and can CHAIN to one through `/Next`, which
// is the precise trick `eachAction`'s header records catching in annotations. Named search:
// `grep -n "Outlines\|Outline" internal/pdfops/scan.go` returns nothing, so neither `Scan` nor
// `StripActive` has ever walked the outline. Carrying `/A` would therefore widen an action surface
// nib's own scanner does not inspect, in a subset that dropped the whole thing for free yesterday.
//
// So a `/S /GoTo` action is read for its `/D` and REWRITTEN as a plain `/Dest`, and `/A` is dropped
// outright — chain and all. Navigation is preserved; the executable surface after this change is the
// one that existed before it.
//
// The item's other keys go the same way the catalog's do, and for the same reason: `outlineItemKeys`
// is an ALLOWLIST, so a key this code has never heard of is dropped rather than carried into every
// extract, split and redaction artifact. `/SE` is the sharp instance and is named below.
var outlineItemKeys = map[string]bool{
	"Title":  true,
	"Parent": true, // all four links are rewritten below, but the keys must be allowed to exist
	"First":  true,
	"Last":   true,
	"Prev":   true,
	"Next":   true,
	"Count":  true,
	"Dest":   true, // always rewritten below to the destination that survived
	"C":      true, // colour and flags are presentation: no reference, nothing page-indexed
	"F":      true,
	// Deliberately absent: /A (above), and /SE — a structure-element reference, which re-anchors an
	// element `carryStructure` may have pruned or that went with the tree when the carry was refused.
	// Dropped whether or not the tree came along, because whether it came along is decided after this
	// runs and a reference that is right in one case and re-anchoring in the other is not worth the
	// coupling. /SD (a PDF 2.0 structure destination on a /GoTo) goes with /A.
}

// outlineRootKeys is the same allowlist for the `/Outlines` dictionary itself. It has no /Parent,
// no /Prev, no /Next and no destination of its own.
var outlineRootKeys = map[string]bool{
	"Type":  true,
	"First": true,
	"Last":  true,
	"Count": true,
}

// outlineNode is one item of the source outline, read once into a shape that can be filtered and
// re-threaded without walking the document again.
type outlineNode struct {
	ref  types.IndirectRef
	dic  types.Dict
	kids []*outlineNode
	// open is the SOURCE item's open/closed state, taken from the SIGN of its /Count. The sign is
	// the only place that state is recorded, and it survives the prune even though the magnitude
	// cannot: the magnitude counts descendants, and this operation changes how many there are.
	open bool
	// dest is the destination that survived — the node's own, or the one it inherited from its first
	// surviving descendant. nil means the node navigates nowhere and does not survive.
	dest types.Object
}

// carryOutline prunes the document outline onto keptPages and returns the `/Outlines` object to
// re-add to the catalog, or nil when nothing survived.
//
// It reports nil rather than an empty `/Outlines` dictionary. An outline root with no `/First` is a
// document that claims to have a contents page and shows an empty one, which reads as a defect
// rather than as the absence it is — and `Outline()` and `SplitByBookmarks` both already handle a
// document with no outline, so the honest shape is the one they were written for.
func carryOutline(xt *model.XRefTable, root types.Dict, keptPages map[int]bool) types.Object {
	o, has := root["Outlines"]
	if !has {
		return nil
	}
	rootDic := derefDict(xt, o)
	if rootDic == nil {
		return nil
	}
	// One visited set for the whole walk, not one per chain. A malformed document can loop a /Next
	// back on itself or reach one item from two parents, and either would spin or duplicate. These
	// are documents chosen for being irregular — every other walk in this package is bounded the
	// same way.
	seen := map[int]bool{}
	kids := readOutlineChain(xt, rootDic["First"], seen, 0)

	live := make([]*outlineNode, 0, len(kids))
	for _, k := range kids {
		if markOutline(xt, k, keptPages) {
			live = append(live, k)
		}
	}
	if len(live) == 0 {
		return nil
	}
	// The catalog holding the outline dictionary DIRECTLY rather than by reference. Nothing can point
	// a /Parent at it, so the tree cannot be re-threaded; the honest answer is the same one an empty
	// outline gets. Checked before anything below is rewritten, so a refusal leaves the source
	// untouched rather than half-pruned.
	ref, isRef := o.(types.IndirectRef)
	if !isRef {
		return nil
	}

	for k := range rootDic {
		if !outlineRootKeys[k] {
			delete(rootDic, k)
		}
	}
	rootDic["Type"] = types.Name("Outlines")
	// The root's /Count is the number of items VISIBLE with the outline expanded, and is
	// non-negative: the root is not itself an item and cannot be closed (ISO 32000-1 Table 152).
	rootDic["Count"] = types.Integer(threadOutline(rootDic, ref, live))
	return o
}

// readOutlineChain reads a sibling chain and everything below it, following /Next across and /First
// down.
func readOutlineChain(xt *model.XRefTable, first types.Object, seen map[int]bool, depth int) []*outlineNode {
	if depth > 50 {
		return nil
	}
	var out []*outlineNode
	for cur := first; cur != nil; {
		ref, isRef := cur.(types.IndirectRef)
		if !isRef {
			return out
		}
		nr := ref.ObjectNumber.Value()
		if seen[nr] {
			return out
		}
		seen[nr] = true
		dic := derefDict(xt, ref)
		if dic == nil {
			return out
		}
		n := &outlineNode{ref: ref, dic: dic}
		if c, ok := intVal(xt, dic["Count"]); ok && c > 0 {
			n.open = true
		}
		n.kids = readOutlineChain(xt, dic["First"], seen, depth+1)
		out = append(out, n)
		cur = dic["Next"]
	}
	return out
}

// intVal resolves an object to an integer.
func intVal(xt *model.XRefTable, o types.Object) (int, bool) {
	d, err := xt.Dereference(o)
	if err != nil || d == nil {
		return 0, false
	}
	i, ok := d.(types.Integer)
	if !ok {
		return 0, false
	}
	return i.Value(), true
}

// markOutline resolves a node's destination, filters its children to the ones that survived, and
// reports whether the node itself survives.
//
// Every child is visited whatever the node's own destination did: a node kept on its own
// destination still has to drop the children that lost theirs, and short-circuiting on the parent
// is how a dropped page stays reachable through a child nobody looked at.
//
// The inheritance is one line because the recursion has already earned it — a child is in `live`
// only if it survived, and a node that survives has a destination, so `live[0].dest` is never nil.
// That is the induction that makes "survives" and "has a destination" one predicate (see header).
func markOutline(xt *model.XRefTable, n *outlineNode, keptPages map[int]bool) bool {
	n.dest = outlineDestination(xt, n.dic, keptPages)
	live := make([]*outlineNode, 0, len(n.kids))
	for _, k := range n.kids {
		if markOutline(xt, k, keptPages) {
			live = append(live, k)
		}
	}
	n.kids = live
	if n.dest == nil && len(live) > 0 {
		n.dest = live[0].dest
	}
	return n.dest != nil
}

// threadOutline rewrites one node's children into a correctly linked chain and returns how many
// items become visible when that node is expanded.
//
// The returned count is what both /Count shapes are built from (ISO 32000-1 Tables 152 and 153): a
// child always contributes itself, and contributes its own descendants only when it is OPEN, since
// a closed item's subtree is not visible. An item's /Count carries that number NEGATED when the item
// is closed, which is the only place open/closed is recorded; the root's is never negative.
func threadOutline(dic types.Dict, ref types.IndirectRef, kids []*outlineNode) int {
	if len(kids) == 0 {
		delete(dic, "First")
		delete(dic, "Last")
		// A leaf carries no /Count at all. Writing 0 would say "open with nothing in it", which is a
		// different claim from "has no children" and is the one pdfcpu's own writer does not make.
		delete(dic, "Count")
		return 0
	}
	dic["First"] = kids[0].ref
	dic["Last"] = kids[len(kids)-1].ref
	visible := 0
	for i, k := range kids {
		for key := range k.dic {
			if !outlineItemKeys[key] {
				delete(k.dic, key)
			}
		}
		// Unconditional: a node reaches this list only by surviving, and a node survives only with a
		// destination. Writing it back is also what REPLACES an inherited one and what erases a
		// `/GoTo` action's, both of which arrive as `k.dest` rather than in the dictionary.
		k.dic["Dest"] = k.dest
		k.dic["Parent"] = ref
		if i == 0 {
			delete(k.dic, "Prev")
		} else {
			k.dic["Prev"] = kids[i-1].ref
		}
		if i == len(kids)-1 {
			delete(k.dic, "Next")
		} else {
			k.dic["Next"] = kids[i+1].ref
		}
		sub := threadOutline(k.dic, k.ref, k.kids)
		visible++
		if k.open {
			visible += sub
			if sub > 0 {
				k.dic["Count"] = types.Integer(sub)
			}
			continue
		}
		if sub > 0 {
			k.dic["Count"] = types.Integer(-sub)
		}
	}
	return visible
}

// outlineDestination returns the destination an outline item should keep, or nil when the page it
// named is not in this subset.
//
// A `/S /GoTo` action's `/D` is read and returned as a plain destination — see the header for why
// the action dictionary itself never survives.
func outlineDestination(xt *model.XRefTable, d types.Dict, keptPages map[int]bool) types.Object {
	dest, has := d["Dest"]
	if !has {
		act := derefDict(xt, d["A"])
		if act == nil || nameVal(act, "S") != "GoTo" {
			return nil
		}
		if dest, has = act["D"]; !has {
			return nil
		}
	}
	if !destReachesAKeptPage(xt, dest, keptPages) {
		return nil
	}
	return dest
}

// destReachesAKeptPage is `destNamesAKeptPage` extended with the one shape an outline uses that an
// annotation's /Dest does not: a NAME, resolved through the /Dests name tree.
//
// It resolves the name to the very object `pruneNames` tests and then asks the same question of it,
// so the two cannot disagree about a destination — which is the failure this indirection invites:
// a bookmark dropped although its named destination survived, or kept although it went.
func destReachesAKeptPage(xt *model.XRefTable, o types.Object, keptPages map[int]bool) bool {
	r, err := xt.Dereference(o)
	if err != nil || r == nil {
		return false
	}
	switch r.(type) {
	case types.Array, types.Dict:
		return destNamesAKeptPage(xt, o, keptPages)
	}
	key, err := xt.DestName(o)
	if err != nil || key == "" {
		return false
	}
	// `xt.Names["Dests"]` only. A name carried by the catalog's PDF-1.1 /Dests dictionary resolves
	// today and cannot resolve in the output — /Dests is not on `catalogAllowlist` — so the honest
	// answer for it is the same as for a page that went.
	node := xt.Names["Dests"]
	if node == nil {
		return false
	}
	v, ok := node.Value(key)
	if !ok {
		return false
	}
	return destNamesAKeptPage(xt, v, keptPages)
}
