package pdfops

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"nib/internal/contentstream"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Is a carry COMPLETE? — `PLAN-ua-coverage.md` P02.S01, D5.
//
// # The blind spot this closes, measured rather than argued
//
// `orphaned()` is law 1's violation and it is deliberately conservative: it fires only when a tree
// anchors to NOTHING — `anchored == 0 && pagesSP == 0`. That is the right predicate for the question
// it answers ("is this claim a lie?") and it cannot see the shape a page-set operation actually
// produces, which is a tree that anchors to *most* of what it describes. `carryTagsThroughNUp`'s own
// doc says the carry is "attempted, verified, and abandoned — never trusted", and the verification it
// names is `honest`, which asks `orphaned()`. So a carry that repoints nine elements of ten reports
// success, passes the backstop, and ships.
//
// Measured at the slice grill (v1.129.134), each through `NUp(2)`:
//
//   - the corpus fixture: clean on all four conditions below, and its one `/ParentTree` key is owned
//     by a form XObject's `/StructParents` — the carry's own write;
//   - the census document (8 pages, 4 distinct contents): **4 of 8 `/ParentTree` keys owned by
//     nobody, and 2 MCID-bearing form XObjects drawn 3 times each**, because pdfcpu's optimize
//     merges equal forms and the carry then binds one XObject to two source pages;
//   - a two-page fixture whose second page is reached through an `MCR` kid: **1 dead MCR `/Pg`** —
//     the carry repoints an element's own `/Pg` and not its kid's (`/pending 503`);
//   - the twin-element fixture: **1 dead element `/Pg`**.
//
// All four report `carried`. That is the defect, and none of it is visible to `orphaned()`.
//
// # Who routes through it
//
// `NUp` since P02.S02, through `completeOrHonest`, in the commit that repaired its carry — the
// predicate and its repair landed together, because wiring it a slice earlier would have flipped the
// census n-up to `dropped` and regressed every document where pdfcpu merges equal forms. A page
// SUBSET since P02.S04b, through `subsetCarrying`'s output gate. **Both share `carryIsComplete`, so
// a condition added here changes both**: S04b's orphan-page condition was measured against `NUp`
// before it landed (`TestEveryDeclaredFateIsTheMEASUREDFate` and the veraPDF differential, both
// still green, n-up still `carried`), and the refusals that are a property of the CARRY rather than
// of the tree deliberately stayed out of it (`carryStructure`'s anchors-nothing refusal).
//
// # Not a PDF/UA verdict
//
// Like `checkStructConsistency`, an empty result means these invariants hold and nothing more. A
// document can carry a complete tree and still fail PDF/UA on everything the tree says.

// maxFormDrawDepth bounds recursion into form XObjects while counting draws. A form invoked twice at
// every level is walked twice at every level, so the cost is exponential in the depth — the same
// reasoning, and the same bound, as `internal/uacheck`'s own form walk.
const maxFormDrawDepth = 8

// structureCarriedCompletely reports every way a carried structure tree is INCOMPLETE — the
// half-carried shapes `orphaned()` is blind to by construction.
//
// The four conditions, each stated as the failure it catches:
//
//  1. **The tree contradicts itself** — `checkStructConsistency`, called rather than restated
//     (ADR-009). A carry that breaks an MCID's ownership has broken the tree whatever else it did.
//  2. **No element, MCR or OBJR names a dead page.** An element whose `/Pg` left the page tree
//     describes nothing; a reader resolving its MCIDs looks in a page that is gone.
//  3. **Every `/ParentTree` key is owned by a live page, annotation or form XObject.** The key is
//     the reverse linkage, so an entry nothing claims is a row of elements no content can reach —
//     which is exactly what a carry leaves behind when two source pages collapse onto one XObject.
//  4. **No MCID-bearing form XObject is drawn more than once.** Its marked content would then have
//     two semantic parents, which is unresolvable rather than merely untidy: one `/StructParents` key
//     cannot name two places. It is NEIGHBOUR to veraPDF's 7.20 t2 (`isUniqueSemanticParent`), not the
//     same test: veraPDF keys on the `/StructParents` KEY and never reads an MCID, and counts a `Do`
//     once per traversed stream — measured at P06.S05, a form with MCIDs and no key drawn twice passes
//     it and one with the key and no MCIDs fails it. `uacheck`'s `7.20 t2` is the port of the clause;
//     this condition is the carry's own property about MCIDs, and is kept as that.
//  5. **No `/ParentTree` key is claimed by more than one owner** — condition 4 read from the other
//     side. Two pages carrying the same `/StructParents` share one row of elements, so the elements
//     describe one of them and the other's content is described by references that name its twin.
//     `DuplicatePage` is `Collect(pdf, ["1-p", "p-"])` and `Collect` preserves a repeat, so from
//     P02.S04b — where a subset carries the tree — this is what duplicating a page produces.
func structureCarriedCompletely(ctx *model.Context, tree *structTree) []structDefect {
	// **ONE page walk for all three sweeps** (/pending 530). Each used to open its own
	// `for p := 1..PageCount { ctx.PageDict(p, false) }`, and `PageDict` walks the page tree from
	// the root every time with no cache — so the gate paid four root walks per page, of a tree whose
	// walk is itself O(p). See `pageRecord` for what the fold deliberately does not change.
	pages := scanPages(ctx)
	out := append([]structDefect{}, checkStructConsistencyOn(ctx, tree, pages)...)
	add := func(key, f string, a ...any) { out = append(out, structDefect{key: key, what: fmt.Sprintf(f, a...)}) }

	for _, e := range tree.elems {
		if e.pgObj != 0 && !e.pgLive {
			add(fmt.Sprintf("dead-pg obj=%d pg=%d", e.objNr, e.pgObj),
				"an element of type /%s names page object %d, which is not in the page tree — it "+
					"describes content no reader can reach", e.kind, e.pgObj)
		}
		for _, k := range e.kids {
			if k.kind != kidMCR && k.kind != kidOBJR {
				continue
			}
			if k.pgObj != 0 && !k.pgLive {
				add(fmt.Sprintf("dead-kid-pg obj=%d pg=%d", e.objNr, k.pgObj),
					"an element of type /%s has a %s naming page object %d, which is not in the "+
						"page tree — the element was repointed and its kid was not",
					e.kind, k.kindName(), k.pgObj)
			}
		}
	}

	arrays, singles := parentTreeEntries(ctx, tree)
	owners := parentTreeOwnersOn(ctx, pages)
	for key, slots := range arrays {
		if len(owners[key]) == 0 {
			add(fmt.Sprintf("unowned-key key=%d", key),
				"/ParentTree key %d holds an array of %d element slot(s) and no page, annotation or "+
					"form XObject claims it — nothing in the document can reach those elements",
				key, len(slots))
		}
	}
	for key := range singles {
		if len(owners[key]) == 0 {
			add(fmt.Sprintf("unowned-key key=%d", key),
				"/ParentTree key %d holds a single element reference and no page, annotation or "+
					"form XObject claims it", key)
		}
	}
	for key, who := range owners {
		if len(who) > 1 {
			sorted := append([]string(nil), who...)
			sort.Strings(sorted)
			add(fmt.Sprintf("shared-key key=%d", key),
				"/ParentTree key %d is claimed by %d owners (%s). One key names one row of elements, "+
					"so those elements describe one claimant and the other's content is reached "+
					"through references that name its twin", key, len(who), strings.Join(sorted, ", "))
		}
	}

	for nr, d := range formDrawCountsOn(ctx, pages) {
		if d.count > 1 && d.mcid {
			add(fmt.Sprintf("shared-form obj=%d", nr),
				"form XObject %d carries marked content and is drawn %d times, so its MCIDs have "+
					"more than one semantic parent and its single /StructParents key cannot name "+
					"them all", nr, d.count)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].what < out[j].what })
	return out
}

// parentTreeOwners returns every object that CLAIMS a `/ParentTree` key, by key.
//
// **Form XObjects are walked, and until P02.S01 they were not.** `allocParentTreeKey`'s own comment
// declared that residue in as many words — *"Form XObjects can carry `/StructParent` too and are not
// walked … That residue is declared, not missed"* — and the carry is the caller that makes it bite:
// `carryTagsThroughNUp` writes `/StructParents` onto the form XObject it built from each source
// page, so after an n-up EVERY key in the tree is owned by an XObject and by nothing else. A walk
// that skipped them would report a correctly carried document as owning nothing at all, which is the
// loudest possible wrong answer for a completeness check.
//
// **Both spellings are claims and both are collected.** A form XObject holding marked content with
// MCIDs carries `/StructParents` (plural, an array entry indexed by MCID); one treated as a single
// content item carries `/StructParent` (singular). They differ by a letter, mean different shapes,
// and either one names a key that is spent.
//
// **EVERY claimant is returned, not the first, and keeping the first hid a defect.** This map was
// `key -> owner` and discarded any later claim, so two pages carrying the same `/StructParents`
// reported as one owned key and the completeness check passed. Measured on a two-page fixture
// sharing key 0: `parentTreeOwners` answered `map[0:page 1]` while the document had two claimants,
// and `checkStructConsistency` reported nothing either — so nothing in this repo could see the
// shape `DuplicatePage` produces once a subset carries its tree (condition 5).
func parentTreeOwners(ctx *model.Context) map[int][]string {
	return parentTreeOwnersOn(ctx, scanPages(ctx))
}

func parentTreeOwnersOn(ctx *model.Context, pages []pageRecord) map[int][]string {
	out := map[int][]string{}
	// ONE visited set across every page: a form XObject in two pages' resources is one object
	// claiming one key, and a fresh set per page would report it as two owners of that key —
	// condition 5's exact shape, which would make such a document's carry unable ever to pass the
	// gate. (Measured through `api.ReadValidateAndOptimize`, the only read path `carryIsComplete`
	// uses, the duplicate did NOT reproduce — the optimize pass consolidates the resource dicts. It
	// reproduces on a plainly-read context, which `structcheck`'s callers use, so the set is shared
	// rather than left to depend on which read path got here.)
	seenForms := map[int]bool{}
	claim := func(o types.Object, what string) {
		v, ok := pdfNumber(ctx.XRefTable, o)
		if !ok || v < 0 {
			return
		}
		out[int(v)] = append(out[int(v)], what)
	}
	for _, rec := range pages {
		p, d := rec.nr, rec.dict
		claim(d["StructParents"], fmt.Sprintf("page %d", p))
		annots, _ := ctx.DereferenceArray(d["Annots"])
		for i, a := range annots {
			if ad, aerr := ctx.DereferenceDict(a); aerr == nil && ad != nil {
				claim(ad["StructParent"], fmt.Sprintf("annotation %d on page %d", i+1, p))
			}
		}
		if rec.res == nil {
			continue
		}
		eachFormXObject(ctx, rec.res, seenForms, 0, func(nr int, sd *types.StreamDict) {
			claim(sd.Dict["StructParents"], fmt.Sprintf("form XObject %d", nr))
			claim(sd.Dict["StructParent"], fmt.Sprintf("form XObject %d", nr))
		})
	}
	return out
}

// eachFormXObject calls fn once for every DISTINCT form XObject reachable from res, including forms
// nested inside other forms' resources. The visited set is by object number, so a form drawn twice
// is offered once — ownership is a property of the object, not of how often it is painted.
func eachFormXObject(ctx *model.Context, res types.Dict, seen map[int]bool, depth int,
	fn func(nr int, sd *types.StreamDict)) {

	if res == nil || depth > maxFormDrawDepth {
		return
	}
	xobjs, err := ctx.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil {
		return
	}
	// **By SORTED name, because a Go map's iteration order is randomised** and a caller that hands
	// out `/ParentTree` keys in visit order would then number two claimants differently run to run —
	// measured, a page with two claiming form XObjects enumerated `[0 2 3]` on 15 of 24 reads of the
	// same bytes and `[0 3 2]` on the other 9. The set of objects offered never varied; which key
	// each got did.
	names := make([]string, 0, len(xobjs))
	for n := range xobjs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ir, isRef := xobjs[n].(types.IndirectRef)
		if !isRef {
			continue
		}
		nr := ir.ObjectNumber.Value()
		if seen[nr] {
			continue
		}
		sd, _, serr := ctx.DereferenceStreamDict(ir)
		if serr != nil || sd == nil {
			continue
		}
		if st := sd.Dict.NameEntry("Subtype"); st == nil || *st != "Form" {
			continue
		}
		seen[nr] = true
		fn(nr, sd)
		if inner, ierr := ctx.DereferenceDict(sd.Dict["Resources"]); ierr == nil && inner != nil {
			eachFormXObject(ctx, inner, seen, depth+1, fn)
		}
	}
}

// formDraw is how often one form XObject is painted, and whether it carries marked content.
type formDraw struct {
	count int
	mcid  bool
}

// formDrawCounts counts every `Do` that paints a form XObject, over every page and recursively
// through the forms themselves.
//
// **Invocations, not objects** — the opposite of `eachFormXObject`'s question. A form drawn twice
// from one page is drawn twice, and that is the whole condition: the `chain` stops a form that draws
// itself from recursing forever without collapsing the repeat that matters.
func formDrawCounts(ctx *model.Context) map[int]formDraw {
	return formDrawCountsOn(ctx, scanPages(ctx))
}

func formDrawCountsOn(ctx *model.Context, pages []pageRecord) map[int]formDraw {
	counts := map[int]formDraw{}
	for _, rec := range pages {
		if rec.res == nil {
			continue
		}
		src, cerr := ctx.PageContent(rec.dict, rec.nr)
		if cerr != nil || len(src) == 0 {
			continue
		}
		// A FRESH chain per page, deliberately: it stops a form that draws itself from recursing
		// forever, and sharing it across pages would collapse the repeat this function exists to
		// count.
		countFormDraws(ctx, src, rec.res, counts, map[int]bool{}, 0)
	}
	return counts
}

// countFormDraws walks one content stream, counting the forms it paints.
//
// **Tokenized, never searched.** `Do` is two bytes that occur inside string literals routinely —
// every glyph nib draws is a two-byte index since P04 — and `alreadyMarked` records the same lesson
// one file over: a byte scan reports a page as drawing a form because of what it says.
func countFormDraws(ctx *model.Context, src []byte, res types.Dict, counts map[int]formDraw,
	chain map[int]bool, depth int) {

	if depth > maxFormDrawDepth {
		return
	}
	var operands []contentstream.Token
	for _, tk := range contentstream.Tokenize(src) {
		switch tk.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.Operator:
		default:
			operands = append(operands, tk)
			continue
		}
		if string(tk.Bytes(src)) == "Do" && len(operands) > 0 {
			// Operands precede their operator, so the name is the last one before `Do`.
			name := strings.TrimPrefix(string(operands[len(operands)-1].Bytes(src)), "/")
			drawForm(ctx, name, res, counts, chain, depth)
		}
		operands = operands[:0]
	}
}

// drawForm records one painting of the named XObject and walks into it.
func drawForm(ctx *model.Context, name string, res types.Dict, counts map[int]formDraw,
	chain map[int]bool, depth int) {

	xobjs, err := ctx.DereferenceDict(res["XObject"])
	if err != nil || xobjs == nil || name == "" {
		return
	}
	obj, ok := xobjs[name]
	if !ok {
		return
	}
	sd, _, serr := ctx.DereferenceStreamDict(obj)
	if serr != nil || sd == nil {
		return
	}
	if st := sd.Dict.NameEntry("Subtype"); st == nil || *st != "Form" {
		return // an image is drawn content, and has no marked content of its own to parent
	}
	ir, isRef := obj.(types.IndirectRef)
	if !isRef {
		return // a direct form cannot be shared: it is reachable from exactly one place
	}
	nr := ir.ObjectNumber.Value()
	body := streamContent(sd)
	d := counts[nr]
	d.count++
	d.mcid = d.mcid || carriesMCID(body)
	counts[nr] = d

	if chain[nr] {
		return // a form that draws itself: counted, not followed
	}
	inner, ierr := ctx.DereferenceDict(sd.Dict["Resources"])
	if ierr != nil || inner == nil {
		inner = res // a form with no resources of its own inherits the invoking stream's
	}
	chain[nr] = true
	countFormDraws(ctx, body, inner, counts, chain, depth+1)
	delete(chain, nr)
}

// completeOrHonest ships a carried document only when the carry is COMPLETE, and falls back to the
// honest loss when it is not — P02.S02's gate, and the caller `NUp` routes through.
//
// # Why this is a door rather than three lines inside `NUp`
//
// A test that drove `NUp` and asserted "the output is complete" would be **vacuous**: once the carry
// is repaired, no document `NUp` composes produces an incomplete tree, so such a test passes because
// the case never arises rather than because the gate works. Given its own door, the gate has its own
// stimulus — a document that really is incompletely carried — and a reader that can go red.
//
// # It re-measures rather than believing the carry's report
//
// `carryTagsThroughNUp` already returns false on anything it does not understand, and that is not
// the same question. Its report says *"I repointed everything I found"*; this asks *"is what you
// produced a tree the document can actually reach"*, which is the question `orphaned()` answers too
// weakly to be the only one asked (`structureCarriedCompletely`'s own header says what it cannot
// see). Three of the four conditions below shipped for months under a carry reporting success.
func completeOrHonest(carried, raw []byte) ([]byte, error) {
	if carryIsComplete(carried) {
		return honest(carried)
	}
	return honest(raw)
}

// carryIsComplete reads a document and asks `structureCarriedCompletely` of it.
//
// **An unreadable or unmodellable document is NOT complete.** A carry whose output this cannot parse
// is one nothing downstream can parse either, and answering "complete" on a failed read would make
// the gate report its own blindness as a pass — the vacuous-green shape this slice exists to remove.
func carryIsComplete(pdf []byte) bool {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return false
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		ir, e := ctx.PageDictIndRef(p)
		if e != nil || ir == nil {
			continue
		}
		live[ir.ObjectNumber.Value()] = true
	}
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		return false
	}
	// **A page the operation removed must not still be IN the file**, which is a different question
	// from every condition above and the only one that catches a class rather than a shape. The five
	// conditions ask whether the tree contradicts itself; this asks whether the tree — or anything
	// else — is still holding a page dictionary the page tree no longer lists, and pdfcpu writes by
	// reachability, so a held page is a page whose `/Contents` ships. Added at P02.S04b, where a
	// subset's carry gave the tree a new way to do it; it is asked here rather than in
	// `structureCarriedCompletely` because it is a property of the DOCUMENT and not of the tree, and
	// `orphanPageObjects`' header says why it dereferences rather than type-asserting.
	return len(structureCarriedCompletely(ctx, tree)) == 0 && len(orphanPageObjects(ctx, live)) == 0
}

// carriesMCID reports whether a content stream marks any content with an `/MCID` — the property that
// makes a doubly-drawn form a structural problem rather than a repeated picture.
func carriesMCID(src []byte) bool {
	for _, tk := range contentstream.Tokenize(src) {
		if tk.Kind == contentstream.Operand && string(tk.Bytes(src)) == "/MCID" {
			return true
		}
	}
	return false
}
