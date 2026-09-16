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
// # It is a READER this slice, and that is deliberate
//
// `NUp` is NOT routed through this yet — S02 does that, in the commit that repairs the carry. Wiring
// it here would flip the census n-up from `carried` to `dropped`, which reddens
// `TestNoOperationAddsAUA1ClauseItsInputDidNotFail` in both directions at once (its `7.20 t2` row
// goes stale AND five `pageSetLoss` clauses appear) and would regress n-up on every document where
// pdfcpu merges equal forms — a real loss shipped for the length of one slice. The predicate and its
// repair land together.
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
//     two semantic parents, which is veraPDF's 7.20 t2 (`isUniqueSemanticParent`) and is
//     unresolvable rather than merely untidy: one `/StructParents` key cannot name two places.
func structureCarriedCompletely(ctx *model.Context, tree *structTree) []structDefect {
	out := append([]structDefect{}, checkStructConsistency(ctx, tree)...)
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
	owners := parentTreeOwners(ctx)
	for key, slots := range arrays {
		if _, owned := owners[key]; !owned {
			add(fmt.Sprintf("unowned-key key=%d", key),
				"/ParentTree key %d holds an array of %d element slot(s) and no page, annotation or "+
					"form XObject claims it — nothing in the document can reach those elements",
				key, len(slots))
		}
	}
	for key := range singles {
		if _, owned := owners[key]; !owned {
			add(fmt.Sprintf("unowned-key key=%d", key),
				"/ParentTree key %d holds a single element reference and no page, annotation or "+
					"form XObject claims it", key)
		}
	}

	for nr, d := range formDrawCounts(ctx) {
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
func parentTreeOwners(ctx *model.Context) map[int]string {
	out := map[int]string{}
	claim := func(o types.Object, what string) {
		v, ok := pdfNumber(ctx.XRefTable, o)
		if !ok || v < 0 {
			return
		}
		if _, taken := out[int(v)]; !taken {
			out[int(v)] = what
		}
	}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil || d == nil {
			continue
		}
		claim(d["StructParents"], fmt.Sprintf("page %d", p))
		annots, _ := ctx.DereferenceArray(d["Annots"])
		for i, a := range annots {
			if ad, aerr := ctx.DereferenceDict(a); aerr == nil && ad != nil {
				claim(ad["StructParent"], fmt.Sprintf("annotation %d on page %d", i+1, p))
			}
		}
		res, rerr := ctx.DereferenceDict(d["Resources"])
		if rerr != nil || res == nil {
			continue
		}
		eachFormXObject(ctx, res, map[int]bool{}, 0, func(nr int, sd *types.StreamDict) {
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
	for _, v := range xobjs {
		ir, isRef := v.(types.IndirectRef)
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
	counts := map[int]formDraw{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil || d == nil {
			continue
		}
		res, rerr := ctx.DereferenceDict(d["Resources"])
		if rerr != nil || res == nil {
			continue
		}
		src, cerr := ctx.PageContent(d, p)
		if cerr != nil || len(src) == 0 {
			continue
		}
		countFormDraws(ctx, src, res, counts, map[int]bool{}, 0)
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
	return len(structureCarriedCompletely(ctx, tree)) == 0
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
