package pdfops

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A subset carries the structure of the pages it keeps — `PLAN-ua-coverage.md` P02.S04b, D5.
//
// # Why the prune is the whole slice, measured rather than argued
//
// Keeping the catalog's structure keys and pruning NOTHING measures **`carried`** on the census
// document — the census's best verdict — while being a lie. `Collect(census, ["1"])` that way comes
// out with all 364 elements, 217 completeness defects (7 `/ParentTree` rows nothing owns, ~210
// elements naming a page that has gone) and **229,859 decoded stream bytes in a one-page output**
// against 91,481 bytes today: the seven dropped pages' content streams are still in the file,
// reachable through the surviving elements' `/Pg`. pdfcpu writes by reachability, so that is not
// untidiness — it is "delete page 4" shipping page 4's text, the same re-anchoring hazard
// `pageselect.go` already records three instances of, arriving through the tree.
//
// **And `fate()`/`orphaned()` cannot see any of it.** They ask whether the tree anchors to *anything*,
// which a tree anchoring to most of what it describes passes. `structureCarriedCompletely` (P02.S01)
// is the reader that can fail here.
//
// # An element's own `/Pg` is NOT a death sentence, and believing it was would have destroyed the tree
//
// The first cut of this read *"an element whose own `/Pg` names a page that has gone is dead"*, and
// that annihilates real documents. Measured: LibreOffice writes `/Pg` on **523 of 523** elements,
// including the root `/Document` element (`/Pg` = page 1) and a `/Table` whose `/Pg` is page 3 while
// its rows are on pages 3–5. Dropping page 1 under that rule leaves **0 elements** of 523 — and it
// does so on **5 of 7** real multi-page tagged documents available here (two LibreOffice conversions
// and the veraPDF UA-1 corpus's five multi-page files), while nib's own census document is
// bit-for-bit identical under both rules in every selection. The one fixture this repo had could not
// see it.
//
// So death is decided by KIDS: an element dies when it has none left. A survivor whose own `/Pg` died
// simply loses that key — its MCID kids went with the page, and an MCR or OBJR kid carries its own
// `/Pg` and may be on a page that survived, which is the only reason such an element is worth
// keeping.
//
// # It refuses more often than it succeeds, and that is the design
//
// Every path that does not end in a complete carry ends in the honest loss: the catalog's structure
// keys are simply not restored, the tree becomes unreachable, and pdfcpu leaves it out. So a refusal
// costs nothing and needs no rollback — which is why the refusals below can be as conservative as
// they like.

// keptPage is one page of the OUTPUT, as `selectPages` placed it.
//
// `annotOf` is non-nil only for a repeat: `clonePage` gives a duplicated page its own annotation
// objects, and an OBJR copied for that page has to name the copy rather than the original's.
type keptPage struct {
	ref     types.IndirectRef
	dic     types.Dict
	clone   bool
	annotOf map[int]types.IndirectRef
}

// carryStructure prunes the source structure tree onto the pages the subset kept, and reports whether
// the output has earned the catalog's structure keys.
//
// **It never fails the operation.** Anything it does not fully understand — an unreadable tree, a
// nested `/ParentTree`, a prune that leaves nothing anchored — returns `false`, and the page operation
// goes on to produce the honest loss it produced before this slice. An error comes back only where the
// failure is nib's own (an xref insert).
func carryStructure(ctx *model.Context, root types.Dict, kept []keptPage) (bool, error) {
	xt := ctx.XRefTable
	if _, has := root["StructTreeRoot"]; !has {
		return false, nil
	}
	live := map[int]bool{}
	keptAnnots := map[int]bool{}
	for _, k := range kept {
		live[k.ref.ObjectNumber.Value()] = true
		for _, a := range derefArray(xt, k.dic["Annots"]) {
			if ar, ok := a.(types.IndirectRef); ok {
				keptAnnots[ar.ObjectNumber.Value()] = true
			}
		}
	}

	// **A tree the model cannot read is not carried**, and that buys the prune an invariant it relies
	// on: `readStructTree` refuses a `/K` entry it cannot classify and an element with no `/S`, so
	// past this line every element has a type the walk below understands.
	tree, err := readStructTree(ctx, live)
	if err != nil {
		return false, nil
	}
	pt := derefDict(xt, tree.root["ParentTree"])
	if pt == nil {
		// No reverse linkage at all. Nothing to prune and nothing a reader could walk back through,
		// so there is no carry to make. Asked by reading the key rather than through
		// `parentTreeDict`, which CREATES an empty one as a side effect of being asked
		// (`structwrite.go:208-217`) — a predicate may not author anything.
		return false, nil
	}
	if _, nested := pt["Kids"]; nested {
		// A nested number tree is refused rather than rebalanced, the same refusal the write half
		// already makes and for the same reason (`parentTreeDict`). Measured: **0 of 294** tagged
		// files in veraPDF's own corpus and neither LibreOffice conversion nests one — LibreOffice
		// keeps 682 flat `/Nums` entries across 142 pages — so the refusal costs nothing measurable.
		// Flattening it instead is `/pending 528`.
		return false, nil
	}

	p := &prune{ctx: ctx, live: live, annots: keptAnnots, decided: map[elemCtx]bool{}}
	kids := p.kids(tree.root["K"], 0, 0)
	if len(kids) == 0 {
		return false, nil // nothing described a page that survived
	}
	tree.root["K"] = kids

	// **`/IDTree` is dropped, not pruned, and that is a declared loss.** An entry naming an element
	// this prune removed keeps that element alive, and the element's `/Pg` keeps a DROPPED page
	// dictionary and its `/Contents` alive — "delete page 4 ships page 4's text" through a key no
	// reading of `/K` can see, because an unreachable element is one `readStructTree` never visits
	// and so one completeness condition 2 cannot fire on. Measured on a fixture: 73 decoded content
	// bytes of a dropped page in the output, with `carryIsComplete` answering true.
	//
	// Pruning it properly would mean walking and rebalancing a name tree; dropping it costs a
	// cross-reference feature that **nothing in this repo reads or writes** (`grep -rn "IDTree"
	// --include=*.go .` finds one comment, `tagsource.go:20`) and that 29 of 294 corpus files carry.
	// An `/IDTree` naming elements a subset removed is wrong whatever else is true of it.
	delete(tree.root, "IDTree")
	delete(tree.root, "AF")

	// **The rows are cleaned of removed elements BEFORE anything is cloned, and the order is the
	// fix for a measured leak.** `carryOntoClone` copies the elements a `/ParentTree` row names, and
	// a row still naming an element the prune took out hands it a REMOVED element to deep-copy — one
	// that kept its `/AF`, its `/T` and whatever else the prune strips only from survivors. Measured:
	// `Collect(src, ["1","1"])` on a document whose row named such an element shipped the source's
	// embedded payload and its filename, with `fate=carried`, **0** completeness defects and **0**
	// orphan pages, so `carryIsComplete` certified it — and `pruneNames` had already deleted
	// `/EmbeddedFiles`, so `Attachments()` reported none and the user could neither see nor remove
	// it. Cleaning first makes the clone path unable to reach a removed element at all.
	clearRemovedFromRows(ctx, tree, p.gone())

	for _, k := range kept {
		if !k.clone {
			continue
		}
		if err := carryOntoClone(ctx, tree, k); err != nil {
			return false, err
		}
	}

	// **The refusal the completeness predicate cannot make.** A prune may legitimately leave the tree
	// with no anchor — and `structureCarriedCompletely` is then vacuously clean, because it walks
	// elements and owned keys and there are none of either — while `orphaned()` still answers false,
	// since a kept page's stale `/StructParents` counts as a surviving linkage
	// (`tagfate.go:132-140`). That combination ships `/MarkInfo /Marked true` over content nothing
	// describes: ADR-031 law 1's substance getting past both codified predicates. It is asked HERE
	// and not added to `structureCarriedCompletely`, which `NUp` shares (ADR-009).
	//
	// With it, an orphaned carried output is unreachable rather than merely unobserved — completeness
	// condition 3 requires every `/ParentTree` key to have a live owner, so a carried tree with any
	// entry has `pagesSP > 0`, and this line forbids `anchored == 0`. Asserted, not assumed:
	// `TestACarriedOutputIsNeverOrphaned`.
	after, aerr := readStructTree(ctx, live)
	if aerr != nil || after.anchored() == 0 {
		return false, nil
	}

	// **Renumbering is LAST, after the final refusal, because it is the only step that writes
	// outside the tree.** It rewrites `/StructParents` on page dictionaries, `/StructParent` on
	// annotations and both spellings on form XObjects — objects that survive the allowlist, so a
	// refusal reached after it would leave those keys rewritten in a document whose tree was then
	// dropped. Measured before the move: a refused carry emitted page 1 with `/StructParents 0`
	// where the non-carrying door left the source's `5`. Nothing observable broke, and the
	// invariant this file's header rests on — *"a refusal costs nothing and needs no rollback"* —
	// was not the one the code had.
	if err := renumberParentTree(ctx, tree); err != nil {
		return false, err
	}
	return true, nil
}

// prune is one prune's working state.
type prune struct {
	ctx    *model.Context
	live   map[int]bool // page object numbers still in the page tree
	annots map[int]bool // annotation object numbers still on a kept page
	// decided memoizes one element's fate, keyed on the object AND the page it inherited, so an
	// element reachable from two parents is judged once PER CONTEXT rather than once.
	//
	// **Keying on the object alone was order-dependent loss of the whole carry.** An element with no
	// `/Pg` of its own owns MCIDs on whichever page reached it, so its fate differs by parent —
	// measured on a fixture where one such element hangs under a page-1 parent and a page-2 parent:
	// `Collect(["1"])` came out `carried` with four elements, and `Collect(["2"])` came out
	// **`dropped`**, because the first walk judged it under the dead page-1 context and the memo
	// then killed both parents.
	decided map[elemCtx]bool
}

// elemCtx identifies one judgment: an element as reached from one inherited page.
type elemCtx struct {
	obj   int
	inhPg int
}

// gone returns the object numbers of the elements this prune took out of the tree.
func (p *prune) gone() map[int]bool {
	out := map[int]bool{}
	for k, kept := range p.decided {
		if !kept && k.obj != 0 {
			out[k.obj] = true
		}
	}
	// An element judged kept under one parent and removed under another is KEPT: it is still in the
	// tree, so its `/ParentTree` slot must still name it.
	for k, kept := range p.decided {
		if kept {
			delete(out, k.obj)
		}
	}
	return out
}

// kids filters one `/K` and returns what survives. inheritPg is the effective `/Pg` of the element
// whose `/K` this is — an integer MCID lives on that page and nowhere else.
func (p *prune) kids(o types.Object, inheritPg, depth int) types.Array {
	if o == nil || depth > maxStructDepth {
		return nil
	}
	xt := p.ctx.XRefTable
	entries := derefArray(xt, o)
	if entries == nil {
		// `/K` takes three shapes and only one is an array. Measured over veraPDF's corpus: **173 of
		// 294** tagged files write the root's `/K` as a single INDIRECT element and 121 as a direct
		// array; inside elements, 200 files have a bare integer and 36 a bare dictionary. A cast to
		// `types.Array` panics on the majority — and `fault.Catch` re-panics anything that is not a
		// `fault.Panic`, so the CLI would die rather than report.
		entries = types.Array{o}
	}
	out := make(types.Array, 0, len(entries))
	for _, raw := range entries {
		if _, isInt := raw.(types.Integer); isInt {
			// An MCID is marked content on the element's own page. If that page has gone, so has the
			// content — there is nothing to keep and nothing to repoint it at.
			if p.live[inheritPg] {
				out = append(out, raw)
			}
			continue
		}
		d := derefDict(xt, raw)
		if d == nil {
			continue
		}
		switch nameVal(d, "Type") {
		case "MCR":
			if p.live[kidPage(xt, d, inheritPg)] {
				out = append(out, raw)
			}
		case "OBJR":
			// **The liveness of an OBJR is a property of its `/Obj`, not of its `/Pg`** — and that
			// is the correction, not a nicety. `/Pg` is OPTIONAL on an OBJR, so one naming no page
			// passes a `/Pg`-only test, and completeness condition 2 skips it too
			// (`structcomplete.go:94` guards on `k.pgObj != 0`). Measured on a fixture: a widget on
			// a dropped page re-anchored through such an OBJR, putting the page's content and the
			// field's value in the output with the gate answering clean.
			//
			// It is also what keeps `dropSignature`'s contract true. That function strips a
			// signature widget from `/Annots` and its field from `/Fields` but removes no objects;
			// a `/Form` element's OBJR still naming the widget re-anchors it, and with it the `/V`
			// signature dictionary — signer name, `/M`, and the PKCS#7 blob carrying the signer's
			// certificate. Measured: `ZZSIGNERNAME`, the field name and `adbe.pkcs7.detached` all
			// present in a subset's output bytes. `keptAnnots` is built AFTER `dropSignature` has
			// run, so one condition covers both cases.
			obj := 0
			if ir, ok := d["Obj"].(types.IndirectRef); ok {
				obj = ir.ObjectNumber.Value()
			}
			// **A page is checked only where one is NAMED.** Requiring both unconditionally
			// destroyed the one shape PDF/UA asks for: a grouping `/Form` element with no `/Pg`
			// whose OBJR also has none inherits page 0, which is never live, so the element lost
			// its only kid and went — even with its annotation on a kept page. That is exactly what
			// `anchored()`'s kid-walk was added for at P06.S07 (`structtree.go:153-161`:
			// *"the only correct way to describe an annotation is a `/Form` element whose `OBJR`
			// kid names the page"*). Measured: `Collect(["1"])` left 2 elements and no `/Form`.
			pg := kidPage(xt, d, inheritPg)
			if p.annots[obj] && (pg == 0 || p.live[pg]) {
				out = append(out, raw)
			}
		default:
			// `/Type` is OPTIONAL on a structure element (ISO 32000-1 table 323), so "" lands here
			// too; `readStructTree` has already refused every other value.
			if p.element(raw, d, inheritPg, depth) {
				out = append(out, raw)
			}
		}
	}
	return out
}

// element decides one structure element's fate and rewrites its `/K` to what survived.
func (p *prune) element(raw types.Object, d types.Dict, inheritPg, depth int) bool {
	objNr := 0
	if ir, isRef := raw.(types.IndirectRef); isRef {
		objNr = ir.ObjectNumber.Value()
		if v, seen := p.decided[elemCtx{objNr, inheritPg}]; seen {
			return v // reachable twice from the same page, or a tree that points back at itself
		}
	}
	// **An element written inline has object number 0 and must not be recorded under it**: every
	// inline element in the document would collide on that key, and `removed[0]` would empty the
	// `/ParentTree` slot of whatever object 0 is not. Measured over veraPDF's corpus, no tagged file
	// writes an inline element — but `readStructTree` models them, so the guard is cheaper than the
	// assumption.
	xt := p.ctx.XRefTable
	own, hasOwn := 0, false
	if ir, isRef := d["Pg"].(types.IndirectRef); isRef {
		own, hasOwn = ir.ObjectNumber.Value(), true
	}
	effPg := inheritPg
	if hasOwn {
		effPg = own
	}
	srcKids := derefArray(xt, d["K"])
	wasArray := srcKids != nil
	if srcKids == nil && d["K"] != nil {
		srcKids = types.Array{d["K"]}
	}
	newKids := p.kids(d["K"], effPg, depth+1)

	// An own `/Pg` naming a page that has gone cannot STAY — that is completeness condition 2 — but
	// it does not kill the element either (see this file's header, and the 523-of-523 measurement).
	if hasOwn && !p.live[own] {
		delete(d, "Pg")
	}

	// An element that LOST every kid describes nothing and goes. One that never had a kid is left
	// alone: a pure reorder must not quietly restructure a tree it is not otherwise touching.
	// An element whose `/K` is an explicitly EMPTY array describes nothing and goes with the rest:
	// "left alone" was written for an element with no `/K` at all, and `derefArray` returns a
	// non-nil zero-length array for `/K []`, which would otherwise take that branch.
	hadNoKids := d["K"] == nil
	keep := len(newKids) > 0 || (hadNoKids && (!hasOwn || p.live[own]))
	if objNr != 0 {
		p.decided[elemCtx{objNr, inheritPg}] = keep
	}
	if !keep {
		return false
	}

	// **`/AF` goes from every carried element, always.** It names a `/Filespec` whose `/EF` stream is
	// an embedded file, and `pruneNames` deletes the catalog's `/EmbeddedFiles` tree on EVERY subset
	// — its own header records an extract shipping the source's attachments *"with the payload
	// readable in the output bytes"*. A carried element's `/AF` re-opens exactly that door, and
	// worse than before: `Attachments()` reads the name tree, so nib reports no attachment and
	// offers no way to remove one. Measured: an embedded payload and its filename in an extract's
	// bytes while `Attachments()` answered empty and the gate answered clean.
	delete(d, "AF")

	// **`/T` goes from an element the prune CHANGED.** It is a human-written title for a span the
	// element no longer wholly covers, it carries no function a reader depends on, and it is a
	// provenance channel out of a document some of whose pages were removed. `/Alt` and
	// `/ActualText` are KEPT and that is the declared limit of what a subset can promise: they are
	// accessibility payload, dropping them is a real loss to the reader who needs them, and a
	// truncated `/ActualText` would be a false statement rather than a lossy one. An element the
	// prune did not touch keeps everything it had.
	if len(newKids) < len(srcKids) {
		delete(d, "T")
	}

	switch {
	case hadNoKids:
		// Nothing to write back: this element never had a `/K`.
	case !wasArray && len(newKids) == 1:
		d["K"] = newKids[0] // it arrived as a single entry and it still is one
	default:
		d["K"] = newKids
	}
	return true
}

// kidPage resolves the page an MCR or OBJR names — its own `/Pg` where it has one, otherwise the
// element's.
func kidPage(xt *model.XRefTable, d types.Dict, inheritPg int) int {
	if ir, isRef := d["Pg"].(types.IndirectRef); isRef {
		return ir.ObjectNumber.Value()
	}
	return inheritPg
}

// eachParentTreeClaim visits every object in the OUTPUT that claims a `/ParentTree` key, in output
// order, handing each its key and a way to rewrite it (a negative key deletes the claim).
//
// It is the write-side twin of `parentTreeOwners`, which answers the same question for reading, and
// the two walk the same three places for the reason that walk records: a page's `/StructParents`, an
// annotation's `/StructParent`, and a form XObject's — both spellings, because they differ by a
// letter and mean different shapes. Measured over veraPDF's 294 tagged files, that enumeration finds
// every claimant in every one of them: 299 pages, 51 `/Link`, 25 `/Widget`, 8 `/Highlight`, 5
// `/FreeText`, 5 `/FileAttachment`, 5 `/Screen` and 2 form XObjects, with zero missed.
func eachParentTreeClaim(ctx *model.Context, fn func(key int, set func(int))) {
	xt := ctx.XRefTable
	claim := func(d types.Dict, key string, fn func(int, func(int))) {
		v, ok := pdfNumber(xt, d[key])
		if !ok || v < 0 {
			return
		}
		dict, k := d, key
		fn(int(v), func(n int) {
			if n < 0 {
				delete(dict, k)
				return
			}
			dict[k] = types.Integer(n)
		})
	}
	// **ONE visited set across every page, not one per page.** A form XObject in two pages'
	// resources is one object claiming one key; a fresh set per page offered it twice, and the
	// second offer re-read the key the first `set()` had just rewritten. Measured on a two-page
	// fixture sharing one form: the walk offered keys `[0 4 1 101]` — four offers for three
	// objects, the last being the value the second call had written a moment earlier.
	seen := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil || d == nil {
			continue
		}
		claim(d, "StructParents", fn)
		for _, a := range derefArray(xt, d["Annots"]) {
			ad := derefDict(xt, a)
			if ad == nil {
				continue
			}
			claim(ad, "StructParent", fn)
			// **An annotation's appearance stream is a form XObject and can claim a key too** —
			// a claimant class no walk here reached. Measured: a row claimed only by an
			// appearance stream was dropped from `/Nums` while the stream went on naming it, and
			// `structureCarriedCompletely` reported the output clean.
			for _, ap := range []string{"N", "R", "D"} {
				eachFormXObject(ctx, types.Dict{"XObject": apStates(xt, ad, ap)}, seen, 0,
					func(_ int, sd *types.StreamDict) {
						claim(sd.Dict, "StructParents", fn)
						claim(sd.Dict, "StructParent", fn)
					})
			}
		}
		res := derefDict(xt, d["Resources"])
		if res == nil {
			continue
		}
		eachFormXObject(ctx, res, seen, 0, func(_ int, sd *types.StreamDict) {
			claim(sd.Dict, "StructParents", fn)
			claim(sd.Dict, "StructParent", fn)
		})
	}
}

// apStates presents one `/AP` entry as an XObject resource dictionary, so the same form walk reaches
// it. An appearance may be a single stream or a dictionary of named states (`/Off`, `/On`); both are
// offered, keyed by a name that only this walk sees.
func apStates(xt *model.XRefTable, annot types.Dict, which string) types.Dict {
	ap := derefDict(xt, annot["AP"])
	if ap == nil {
		return nil
	}
	entry, has := ap[which]
	if !has {
		return nil
	}
	out := types.Dict{}
	if ir, isRef := entry.(types.IndirectRef); isRef {
		if sd, _, err := xt.DereferenceStreamDict(ir); err == nil && sd != nil {
			out[which] = ir
			return out
		}
	}
	if states := derefDict(xt, entry); states != nil {
		names := make([]string, 0, len(states))
		for n := range states {
			names = append(names, n)
		}
		sort.Strings(names) // a Go map's order would make the numbering differ run to run
		for _, n := range names {
			out[which+"/"+n] = states[n]
		}
	}
	return out
}

// clearRemovedFromRows empties every `/ParentTree` slot naming an element the prune took out.
//
// **It runs BEFORE anything is cloned**, and that ordering is the fix for a measured leak: the clone
// path copies the elements a row names, so a row still naming a removed element hands it one to
// deep-copy — `/AF`, `/T` and all — and attaches the copy to the tree. See `carryStructure`.
//
// The array is indexed BY MCID, so a slot is EMPTIED and never compacted: closing the gap would
// shift every later MCID onto the wrong element. An emptied slot writes as a `null`, which is what
// `checkStructConsistency` reads as "no element owns this content".
func clearRemovedFromRows(ctx *model.Context, tree *structTree, removed map[int]bool) {
	if len(removed) == 0 {
		return
	}
	xt := ctx.XRefTable
	pt := derefDict(xt, tree.root["ParentTree"])
	if pt == nil {
		return
	}
	nums := derefArray(xt, pt["Nums"])
	for i := 0; i+1 < len(nums); i += 2 {
		val := nums[i+1]
		if arr := derefArray(xt, val); arr != nil {
			cleaned := make(types.Array, len(arr))
			for j, x := range arr {
				if ir, isRef := x.(types.IndirectRef); isRef && removed[ir.ObjectNumber.Value()] {
					continue
				}
				cleaned[j] = x
			}
			nums[i+1] = cleaned
			continue
		}
		if ir, isRef := val.(types.IndirectRef); isRef && removed[ir.ObjectNumber.Value()] {
			nums[i+1] = nil
		}
	}
	pt["Nums"] = nums
}

// renumberParentTree rebuilds `/Nums` from the claimants the OUTPUT actually has, numbering them
// `0..n-1` in output order.
//
// # Three things at once, and the third is why it renumbers rather than filters
//
//  1. **A row nothing claims goes.** The dropped pages' rows and the dropped pages' annotation rows
//     leave by one rule rather than two.
//  2. **A claim on a row that did not survive is DELETED from the claimant.** Keeping it leaves an
//     annotation naming a key the tree no longer has, which nothing in this package can see:
//     `checkStructConsistency` tests that a *page's* `/StructParents` resolves and never an
//     annotation's (`structcheck.go:61-144`). Both directions or neither, which is the rule the rest
//     of this package already keeps.
//  3. **The keys are renumbered, because the source's keys are the source's PAGE INDICES.** Measured
//     on nib's own six-page document, keeping pages 4 and 5: the output's two pages carried
//     `/StructParents` 3 and 4 over `/Nums` keys `[3 4]` with `/ParentTreeNextKey` still **6**. A
//     two-page document declaring six spent keys says *"I was pages 4 and 5 of something with six"* —
//     a source-cardinality channel nobody chose, in the artifact a user hands to a stranger, six
//     lines from where the trailer's permanent `/ID[0]` is cleared for exactly that reason
//     (`pageselect.go:246-249`). Renumbering also makes the output internally consistent, which a
//     filtered tree is not.
//
// Claimants are visited in output order and keys handed out ascending, so `/Nums` comes out sorted —
// which a number tree requires and which nothing guarantees of the source.
func renumberParentTree(ctx *model.Context, tree *structTree) error {
	xt := ctx.XRefTable
	pt := derefDict(xt, tree.root["ParentTree"])
	if pt == nil {
		return fmt.Errorf("pdfops: the structure tree's /ParentTree stopped resolving mid-carry")
	}
	rows := map[int]types.Object{}
	nums := derefArray(xt, pt["Nums"])
	for i := 0; i+1 < len(nums); i += 2 {
		n, isInt := nums[i].(types.Integer)
		if !isInt {
			continue
		}
		// The rows were cleaned of removed elements before the clones ran, so this only re-keys.
		if nums[i+1] != nil {
			rows[n.Value()] = nums[i+1]
		}
	}

	out := types.Array{}
	assigned := map[int]int{}
	next := 0
	eachParentTreeClaim(ctx, func(key int, set func(int)) {
		row, has := rows[key]
		if !has {
			set(-1)
			return
		}
		if n, done := assigned[key]; done {
			set(n) // two claimants of one source key stay sharing it, which condition 5 reports
			return
		}
		assigned[key] = next
		out = append(out, types.Integer(next), row)
		set(next)
		next++
	})
	pt["Nums"] = out
	tree.root["ParentTreeNextKey"] = types.Integer(next)
	return nil
}

// carryOntoClone gives a repeated page its own `/ParentTree` key and its own elements.
//
// # Why a copy rather than a second claim on the original's row
//
// A `/ParentTree` key names one row of elements and `/StructParents` is one integer on one object, so
// two pages carrying the same key share one row: the elements then describe one of the two pages and
// the other's content is reached through references naming its twin. That is completeness condition 5,
// and `DuplicatePage` is `Collect(pdf, ["1-p", "p-"])` — so from this slice on it is exactly what
// duplicating a page produces unless the subtree is copied. Measured without this: `/ParentTree` key 1
// claimed by two pages, `carryIsComplete` false, and the fate degrading to `partial`.
//
// # It follows the CLONE's own claims, not the page's alone
//
// `clonePage` copies a page's annotations into fresh objects, and a copied annotation carries the
// source's `/StructParent` unchanged — a second claim on a single-reference row, the same defect one
// shape over. So the keys re-mapped here are the page's `/StructParents` and every cloned
// annotation's `/StructParent`, each written back onto the object that claimed it.
//
// # The keys come from the allocator and are claimed by the writer
//
// `allocParentTreeKey` caches its floor and only `claimParentTreeKey` raises it — and only
// `setParentTreeSlot`/`setParentTreeSingle` call that. So every key here is allocated and then
// immediately written through one of those two doors; allocating a batch first would hand out the
// same key every time.
func carryOntoClone(ctx *model.Context, tree *structTree, k keptPage) error {
	xt := ctx.XRefTable
	type claim struct {
		key   int
		write func(newKey int)
	}
	var claims []claim
	if v, ok := pdfNumber(xt, k.dic["StructParents"]); ok && v >= 0 {
		page := k.dic
		claims = append(claims, claim{int(v), func(n int) { page["StructParents"] = types.Integer(n) }})
	}
	for _, a := range derefArray(xt, k.dic["Annots"]) {
		ad := derefDict(xt, a)
		if ad == nil {
			continue
		}
		if v, ok := pdfNumber(xt, ad["StructParent"]); ok && v >= 0 {
			annot := ad
			claims = append(claims, claim{int(v), func(n int) { annot["StructParent"] = types.Integer(n) }})
		}
	}
	if len(claims) == 0 {
		return nil // an untagged page repeated in a tagged document claims no row
	}

	c := &subtreeClone{ctx: ctx, page: k.ref, annotOf: k.annotOf, made: map[int]types.IndirectRef{}}
	for _, cl := range claims {
		arr, single, found := rowFor(ctx, tree, cl.key)
		if !found {
			continue // a key with no row: the claim is already dangling in the source
		}
		if single != nil {
			newKey := allocParentTreeKey(ctx, tree)
			ref, err := c.of(*single)
			if err != nil {
				return err
			}
			if ref == nil {
				continue
			}
			if err := setParentTreeSingle(ctx, tree, newKey, *ref); err != nil {
				return err
			}
			cl.write(newKey)
			continue
		}
		newKey := allocParentTreeKey(ctx, tree)
		wrote := false
		for slot, x := range arr {
			ir, isRef := x.(types.IndirectRef)
			if !isRef {
				continue // an empty slot stays empty in the copy
			}
			ref, err := c.of(ir)
			if err != nil {
				return err
			}
			if ref == nil {
				continue
			}
			if err := setParentTreeSlot(ctx, tree, newKey, slot, *ref); err != nil {
				return err
			}
			wrote = true
		}
		if wrote {
			cl.write(newKey)
		}
	}
	return c.attach(ctx, tree)
}

// rowFor returns the `/ParentTree` entry for one key, in whichever of the two shapes it has.
func rowFor(ctx *model.Context, tree *structTree, key int) (arr types.Array, single *types.IndirectRef, found bool) {
	xt := ctx.XRefTable
	pt := derefDict(xt, tree.root["ParentTree"])
	if pt == nil {
		return nil, nil, false
	}
	nums := derefArray(xt, pt["Nums"])
	for i := 0; i+1 < len(nums); i += 2 {
		n, isInt := nums[i].(types.Integer)
		if !isInt || n.Value() != key {
			continue
		}
		if a := derefArray(xt, nums[i+1]); a != nil {
			return a, nil, true
		}
		if ir, isRef := nums[i+1].(types.IndirectRef); isRef {
			return nil, &ir, true
		}
		return nil, nil, false
	}
	return nil, nil, false
}

// subtreeClone copies structure elements onto a duplicated page.
//
// # `types.Dict.Clone()` is NOT a deep copy, and the first design rested on it
//
// `Dict.Clone` recurses through `v.Clone()`, and `IndirectRef.Clone()` returns `ir2 := ir` — a copy of
// the REFERENCE (`types/dict.go:43-52`, `types/types.go:564-567`). An element's `/K` holds indirect
// references to its children, so cloning the top element yields a clone whose children *are the
// originals* — and repointing "the clone's kids' `/Pg`" then mutates the source subtree. That is
// `/pending 503`'s defect arriving from the other side. So this recurses through references and
// allocates one new object per element, which is what `clonePage` already hand-rolls for annotations
// and says why (`pageselect.go:266-268`).
type subtreeClone struct {
	ctx     *model.Context
	page    types.IndirectRef         // the duplicated page every copy is anchored to
	annotOf map[int]types.IndirectRef // source annotation -> the clone's own copy
	made    map[int]types.IndirectRef // source element -> its copy, so one element is copied once
	parents map[int]types.Object      // copy objNr -> the `/P` its original had
	order   []int                     // the source elements copied, in the order they were copied
}

// of copies one element and everything under it, returning the reference to the copy, or nil where
// the source names an element the document does not have.
//
// **A slot naming an object that does not resolve is SKIPPED, not an error**, and that is a
// correction: it returned one, so a corrupt `/ParentTree` row made the whole page-duplicate refuse
// the document — measured, `Collect(src, ["1","1"])` came back with 0 bytes and
// `pdfops: a structure element … could not be read` on a document whose `Collect(src, ["1"])`
// succeeded and reported `carried`. A defect in the SOURCE must produce the honest loss, which is
// what this file's header promises and what `rowFor`'s neighbouring case already did.
func (c *subtreeClone) of(src types.IndirectRef) (*types.IndirectRef, error) {
	nr := src.ObjectNumber.Value()
	if ref, done := c.made[nr]; done {
		return &ref, nil
	}
	xt := c.ctx.XRefTable
	d := derefDict(xt, src)
	if d == nil {
		return nil, nil
	}
	copyDict, ok := d.Clone().(types.Dict)
	if !ok {
		return nil, fmt.Errorf("pdfops: a structure element could not be copied")
	}
	// Allocate before descending, so an element reached twice inside its own subtree resolves to the
	// copy rather than being copied again.
	ref, err := xt.IndRefForNewObject(copyDict)
	if err != nil {
		return nil, err
	}
	c.made[nr] = *ref
	if c.parents == nil {
		c.parents = map[int]types.Object{}
	}
	c.parents[ref.ObjectNumber.Value()] = d["P"]
	c.order = append(c.order, nr)

	if _, hasPg := copyDict["Pg"]; hasPg {
		copyDict["Pg"] = c.page
	}
	wasArray := derefArray(xt, d["K"]) != nil
	kids, err := c.kidsOf(d["K"], *ref)
	if err != nil {
		return nil, err
	}
	switch {
	case kids == nil:
		delete(copyDict, "K")
	case !wasArray && len(kids) == 1:
		copyDict["K"] = kids[0]
	default:
		copyDict["K"] = kids
	}
	return ref, nil
}

// kidsOf copies one `/K`, giving each copied child element the copy's own `/P`.
//
// **`/P` is required and it named the ORIGINAL's parent until this took an argument.** `Dict.Clone`
// copies the key along with everything else, so a copied child pointed at the original parent while
// living in the copy's `/K` — a tree that disagrees with itself in the two directions a reader can
// walk it, which is the defect `appendToElementKids` and the structure editor both maintain `/P` to
// avoid (ADR-009). Measured before the fix: `elem 44 /S=P /P=(40 0 R)` under tree parent 43, with
// `structureCarriedCompletely` reporting zero defects — no reader in this package looks at `/P`, and
// `internal/uacheck`'s `declaresLangFor` climbs it.
func (c *subtreeClone) kidsOf(o types.Object, parent types.IndirectRef) (types.Array, error) {
	if o == nil {
		return nil, nil
	}
	xt := c.ctx.XRefTable
	entries := derefArray(xt, o)
	if entries == nil {
		entries = types.Array{o}
	}
	out := make(types.Array, 0, len(entries))
	for _, raw := range entries {
		if _, isInt := raw.(types.Integer); isInt {
			out = append(out, raw) // the same MCID, on the copy's own page
			continue
		}
		d := derefDict(xt, raw)
		if d == nil {
			continue
		}
		switch nameVal(d, "Type") {
		case "MCR", "OBJR":
			nd, ok := d.Clone().(types.Dict)
			if !ok {
				return nil, fmt.Errorf("pdfops: a marked-content reference could not be copied")
			}
			if _, hasPg := nd["Pg"]; hasPg {
				nd["Pg"] = c.page
			}
			if ir, isRef := nd["Obj"].(types.IndirectRef); isRef {
				// An OBJR names an annotation, and the copy must name the COPY's annotation:
				// `clonePage` gave the duplicated page its own annotation objects, and an OBJR still
				// naming the original's would put one annotation under two elements.
				cloned, known := c.annotOf[ir.ObjectNumber.Value()]
				if !known {
					continue // nothing on the copy corresponds to it
				}
				nd["Obj"] = cloned
			}
			// Written inline, exactly as the producer's were, so they need no object of their own.
			out = append(out, nd)
		default:
			ir, isRef := raw.(types.IndirectRef)
			if !isRef {
				continue // an inline child element: the copy does without it
			}
			ref, err := c.of(ir)
			if err != nil {
				return nil, err
			}
			if ref == nil {
				continue // the source names a child it does not have
			}
			if cd := derefDict(xt, *ref); cd != nil {
				cd["P"] = parent
			}
			out = append(out, *ref)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// attach puts the copies into the tree, so a reader walking from the root reaches them.
//
// **Each copy goes immediately after its original in the original's parent**, which needs no element
// of nib's own — authoring one would be a claim about structure this slice is not entitled to make.
// A copy whose original's parent is ITSELF a copy is skipped: it is already reachable through that
// parent's own `/K`, and attaching it again would put it in the tree twice.
//
// **The reading order this produces is item-by-item, not page-then-page**, and that is a declared
// limit rather than an oversight. A real document's `/ParentTree` rows name leaves — a `/Table
// Contents`, a `/Link`, an `/H2` — whose common ancestors span other pages and so cannot be copied,
// so a reader hears the original's paragraph and then the duplicate's rather than the whole page and
// then its copy. Every element is anchored, every MCID resolves and nothing describes content that is
// not there; what is lost is sequence, on an operation whose two pages are identical. Recorded as
// `/pending 529`.
func (c *subtreeClone) attach(ctx *model.Context, tree *structTree) error {
	xt := ctx.XRefTable
	for _, srcNr := range c.order {
		ref := c.made[srcNr]
		parent := c.parents[ref.ObjectNumber.Value()]
		if pr, isRef := parent.(types.IndirectRef); isRef {
			if _, alsoCopied := c.made[pr.ObjectNumber.Value()]; alsoCopied {
				continue
			}
		}
		src := types.NewIndirectRef(srcNr, 0)
		pd := derefDict(xt, parent)
		if pd == nil {
			pd = tree.root
		}
		insertAfter(xt, pd, *src, ref)
	}
	return nil
}

// insertAfter puts ref into d's `/K` immediately after `after`, appending when it is not there.
//
// **With several copies of one page the copies land in reverse creation order**, because each goes
// immediately after the ORIGINAL rather than after the previous copy. Measured on
// `Collect(["1","1","1"])`: `/K=[(32) (66) (64) (65) (67)]`. Every element is anchored and every
// MCID resolves; what is lost is sequence, which is `/pending 529`'s subject one degree further.
func insertAfter(xt *model.XRefTable, d types.Dict, after, ref types.IndirectRef) {
	arr := derefArray(xt, d["K"])
	if arr == nil {
		if d["K"] == nil {
			d["K"] = types.Array{ref}
			return
		}
		arr = types.Array{d["K"]}
	}
	out := make(types.Array, 0, len(arr)+1)
	placed := false
	for _, x := range arr {
		out = append(out, x)
		if ir, isRef := x.(types.IndirectRef); isRef && !placed &&
			ir.ObjectNumber.Value() == after.ObjectNumber.Value() {
			out = append(out, ref)
			placed = true
		}
	}
	if !placed {
		out = append(out, ref)
	}
	d["K"] = out
}

// carryTitleFloor gives a carried subset the document title PDF/UA needs, REBUILT rather than
// inherited.
//
// # Why the source's `/Metadata` is not carried whole
//
// `selectPages` empties the document `/Info` six lines away, and says why: *"a title and nib's own
// NibFlags are both gone — and it travels into every extract and split artifact if kept"*. XMP
// carries the same fields and more. Measured on a producer-style packet, everything below rode into
// a two-page extract of a six-page document: `dc:title`, `dc:creator` (an email address),
// `dc:description` (*"internal, privileged"*), `pdf:Keywords`, `pdf:Producer`, `xmpMM:DocumentID` and
// `xmpMM:History` — while the trailer's permanent `/ID[0]` is cleared four lines below for exactly
// the reason `xmpMM:DocumentID` exists. One door stripping what another restores.
//
// So the allowlist principle the catalog already follows is applied INSIDE the packet: the only thing
// PDF/UA needs from a document's metadata is its title (7.1 t8/t9), a title is not page-indexed, and
// a fresh packet built through `buildXMP` — the door `SetTitle` uses — cannot carry a field nobody
// enumerated. The identification cannot ride back in either, by construction rather than by ordering:
// the packet is new.
//
// **`/Info`'s `/Title` is deliberately NOT written back**, unlike `SetTitle`'s three-in-one. `/Info`
// being empty after a subset is a measured parity decision of P02.S04a, and re-adding a key to it
// would reverse that decision from inside a different slice.
//
// **All three places a title lives are written, which is `SetTitle`'s rule and not a second one.**
// That door's header says why: *"a document carrying two of them is a worse state than one carrying
// none — a viewer told to display a title it cannot find shows an empty chrome bar"*. The first cut
// of this wrote the packet and the preference and skipped `/Info`'s `/Title`, producing exactly that
// two-of-three state — measured, XMP `dc:title` present and `/Info /Title` nil. `/Info` is still
// EMPTIED first, so only the title comes back and nib's own `NibFlags` and the source's dates and
// producer do not; this writes through `setInfoTitle`, the door `SetTitle` uses.
//
// # `/ViewerPreferences` is PRESERVED key by key, never invented
//
// It is carried for `/DisplayDocTitle`, which 7.1 t10 requires. Carried whole it also brings
// `/PrintPageRange` — which is PAGE-INDEXED, the exact property `/Outlines` and `/PageLabels` are
// refused for. Measured: a two-page extract declaring `/PrintPageRange [3 7]`, alongside
// `/NumCopies 3` and a duplex setting from a document it is no longer part of.
//
// **And the value is the SOURCE's.** Writing `true` unconditionally overrode an author who had said
// `false` — measured, a source with `/DisplayDocTitle false` and a source with the key absent both
// came out `true`, so a subset was authoring a preference rather than carrying one. A subset states
// what the document stated; where the source asked for nothing, nothing is written, and 7.1 t10's
// fate follows the source's as every other clause does.
//
// # It cannot fail the operation
//
// Its errors are nib's own — an xref insert, a stream encode — and it runs after the catalog's
// structure keys are back, so returning one would both add a failure mode the header disclaims and
// leave state to roll back. A failure here drops the title floor and the carry with it, which is the
// honest loss the rest of this file is built on.
func carryTitleFloor(ctx *model.Context, root types.Dict, title string, showTitle *bool) bool {
	if showTitle != nil {
		ctx.ViewerPref = &model.ViewerPreferences{DisplayDocTitle: showTitle}
		ctx.XRefTable.BindViewerPreferences()
	}
	if title == "" {
		return true // nothing to state; 7.1 t8 was already failing on the source
	}
	packet, err := buildXMP(title, time.Now())
	if err != nil {
		return false
	}
	sd, err := ctx.NewStreamDictForBuf(packet)
	if err != nil {
		return false
	}
	sd.Dict["Type"] = types.Name("Metadata")
	sd.Dict["Subtype"] = types.Name("XML")
	if err := sd.Encode(); err != nil {
		return false
	}
	ref, err := ctx.IndRefForNewObject(*sd)
	if err != nil {
		return false
	}
	root["Metadata"] = *ref
	if err := setInfoTitle(ctx, title); err != nil {
		return false
	}
	return true
}

// displayDocTitle reads a document's `/ViewerPreferences /DisplayDocTitle`, or nil where it states
// none. pdfcpu has already parsed it into `ctx.ViewerPref` by the time the carry runs.
func displayDocTitle(ctx *model.Context) *bool {
	if ctx.ViewerPref == nil || ctx.ViewerPref.DisplayDocTitle == nil {
		return nil
	}
	v := *ctx.ViewerPref.DisplayDocTitle
	return &v
}

// documentTitle reads a document's `dc:title` out of its catalog XMP packet.
func documentTitle(ctx *model.Context, root types.Dict) string {
	obj, ok := root["Metadata"]
	if !ok {
		return ""
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(obj)
	if err != nil || sd == nil {
		return ""
	}
	if err := sd.Decode(); err != nil {
		return ""
	}
	return packetTitle(string(sd.Content))
}

// packetTitle returns a packet's `dc:title`, read BY NAMESPACE rather than by element name — a packet
// may bind the Dublin Core namespace to any prefix, and `pdfa.go` and `labelua.go` both learned that
// the hard way.
func packetTitle(packet string) string {
	const nsDC = "http://purl.org/dc/elements/1.1/"
	dec := xml.NewDecoder(bytes.NewReader([]byte(packet)))
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == nsDC && t.Name.Local == "title" {
				depth++
			} else if depth > 0 {
				depth++
			}
		case xml.EndElement:
			if depth > 0 {
				depth--
			}
		case xml.CharData:
			if depth > 0 {
				if s := strings.TrimSpace(string(t)); s != "" {
					return s
				}
			}
		}
	}
}

// orphanPageObjects returns every `/Type /Page` object in the document that is NOT in the page tree.
//
// # It is the one reader that catches a whole class rather than a shape
//
// pdfcpu writes by reachability, so an unlinked page's bytes never reach the output — and every
// re-anchoring defect this package has had is something that still NAMED a dropped page: a field's
// `/Kids`, a link's destination, a surviving named destination, an element's `/Pg`, an `/IDTree`
// entry, an OBJR's `/Obj`. Each was found separately and fixed separately. This asks the question all
// six answer: *is a page the operation removed still in the file?* It needs no list of keys and it
// cannot go stale as new ones are added.
//
// **It DEREFERENCES rather than type-asserting, and that is load-bearing.** pdfcpu holds an object
// that lives in a compressed object stream as a `LazyObjectStreamObject` until something resolves it,
// so `e.Object.(types.Dict)` is false for exactly the pages a subset leaves behind — a detector
// written that way reported a document with a dropped page in it as clean. It is the same mistake as
// counting `/StructElem` in the raw bytes (`tagfate.go:23-28`), one layer down.
// **It takes the live set rather than building one**, because `ctx.PageDictIndRef` walks the page
// tree from the root on every call: measured, that loop alone cost 3.4 ms at 100 pages, 53 ms at 400
// and 238 ms at 800, against **175 µs** for this function's whole xref sweep at 800 pages. The O(n²)
// shape is the one two neighbouring comments in this package already forbid, and every caller here
// has the set to hand.
func orphanPageObjects(ctx *model.Context, inTree map[int]bool) []int {
	var out []int
	for nr, e := range ctx.Table {
		if e == nil || inTree[nr] {
			continue
		}
		gen := 0
		if e.Generation != nil {
			gen = *e.Generation
		}
		o, err := ctx.Dereference(*types.NewIndirectRef(nr, gen))
		if err != nil {
			continue
		}
		// **`/Type /Page` is the whole test, and the alternative was defensive code for a shape
		// that cannot arrive.** `collectLeaves` treats a node with no `/Type` and no `/Kids` as a
		// leaf page — so the two walks look as though they could disagree about what a page is. They
		// cannot: pdfcpu refuses a page-tree node with no `/Type` during the read, measured
		// (`pdfcpu: dictTypeForPageNodeDict: missing pageNodeDict type`), so nothing this function
		// sees ever got that far. `TestPDFCPURefusesATypelessPageNode` is the guard on that reason,
		// because if the reader ever relaxes it, this becomes a blind spot.
		if d, isDict := o.(types.Dict); isDict && nameVal(d, "Type") == "Page" {
			out = append(out, nr)
		}
	}
	sort.Ints(out)
	return out
}
