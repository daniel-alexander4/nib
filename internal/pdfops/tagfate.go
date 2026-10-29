package pdfops

import (
	"crypto/sha256"
	"encoding/hex"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Tag fate — `PLAN-accessibility.md` P01, ADR-031, and `/pending 29`'s floor.
//
// # The law this file exists to keep
//
// **Nothing claims tagging it has not.** No output may carry `/MarkInfo /Marked true`, a
// `/StructTreeRoot`, or a conformance assertion over content that is neither tagged nor marked as
// an artifact. *A visible loss is honest; a false claim is not, and it is worse than no tagging at
// all because it defeats the reader's own check* — a screen reader that is told a document is
// tagged stops looking for the fallbacks it would otherwise use.
//
// # This file has been wrong once, expensively, and the record of that is kept here
//
// The first version measured tagging with `bytes.Count(pdf, []byte("/StructElem"))`. **pdfcpu writes
// the structure tree into a compressed object stream**, so that count is `0` for every pdfcpu output
// whatever it contains — a perfectly tagged document and a stripped one are identical to it. On that
// evidence eight operations were reported as lying, enforcement was written against all of them, and
// it **stripped tag trees that had survived intact** (v1.129.9–.14, reverted at v1.129.15, never
// released). Everything in this file now parses. Nothing in it counts bytes.
//
// # What is true, measured three ways because two of them have been wrong before
//
// On a LibreOffice-produced tagged PDF — 4 pages, `/Marked true`, 45 `/StructElem`, every page
// carrying `/StructParents` — against pdfcpu v0.13.0 (MEASURED THEN, not re-measured on v0.16.2):
//
//	operation              claim  elements  anchored  undescribed pages
//	Rotate / Optimize      yes    45        45        0            carried
//	Collect / RemovePages  no     0         —         —            dropped, honestly
//	Append(tagged first)   yes    45        45        1            PARTIAL
//	NUp(2)                 yes    45        **0**     2            **ORPHANED — law 1's violation**
//
// *Anchored* means the element's `/Pg` is a page still in the page tree. **`NUp` composes its
// sheets as new page objects and carries the old tree onto them**: all 45 elements point at pages
// that are no longer in the document, no composed page carries `/StructParents`, and the result
// says `/Marked true`. Nothing in that output is described by the tree it advertises.
//
// veraPDF agrees independently and names the law in its own words. ua1 against the same documents:
// the source and `Rotate`'s output fail the same three clauses (5 t1, 7.1 t9, 7.1 t10 — producer
// limitations); the n-up output fails **7.1 t3, *"Content shall be marked as Artifact or tagged as
// real content"*, with 24 failed checks** — a failure neither the input nor `Rotate` has.
//
// # Why the remedy is a POST-CONDITION and not a strip
//
// Dropping the claim is right exactly where the tree describes nothing that exists, because there
// is then nothing to preserve — and it is wrong everywhere else, which is what the reverted version
// got wrong. So `honest` asks the output a question and acts only on the answer, and `orphaned` is
// deliberately the most conservative form of that question it can be: it refuses to strip while
// *either* linkage survives — an element anchored to a live page, or any page carrying
// `/StructParents`.
//
// **Measured, so the remedy is not itself an untested claim:** stripping the n-up output's claim
// does not remove 7.1 t3 (the content is untagged either way) and adds 6.2 t1 and 7.1 t11, because
// PDF/UA requires a structure tree. A veraPDF failure *count* therefore scores honesty as a
// regression, which is why P01.S01's acceptance is a structural property and not a clause count.
//
// # One door
//
// Every operation that needs this calls `honest`, rather than each deleting two keys correctly.
// That is ADR-009's shape and law 2's. **The guard is stronger than the door**: the tag-fate table
// measures every operation it can drive and fails on an orphaned output whether or not that
// operation routes through here, so an operation that starts lying tomorrow goes red without
// anyone having remembered to wire it up.
//
// **The law itself is ADR-031.**

// tagState is what a document says about being tagged, set beside what it actually carries.
//
// The two are separate fields on purpose: law 1 is precisely the claim that the second does not
// support the first, and a single boolean cannot express it.
type tagState struct {
	readable bool // false when the document cannot be parsed at all
	marked   bool // /MarkInfo /Marked true
	tree     bool // /StructTreeRoot present in the catalog
	pages    int
	pagesSP  int // pages carrying /StructParents
	elements int // struct elements reachable from the tree root
	anchored int // of those, the ones whose /Pg is a page still in the page tree
	// undescribed counts pages that have a content stream and that **no struct element points at**.
	//
	// **It asked about `/StructParents` until v1.129.18, and that was the wrong question.**
	// (The merge shape below is history since P02.S07a — `mergeDocs` grafts a tagged second document
	// with its keys offset, ADR-048 — and the predicate is still the right one for every other road
	// to a page whose key lies.) `api.MergeRaw` merged two tagged documents by keeping the FIRST document's `/StructTreeRoot`
	// and `/ParentTree` whole while the second document's pages keep their own `/StructParents`
	// values — so an 8-page merge carries a 4-entry `/ParentTree` and eight pages indexing keys
	// 0–3. Pages 5–8 therefore HAVE a `/StructParents`, which the old predicate accepted, and it
	// resolves to structure describing entirely different content. Nothing in the tree points at
	// them, so a screen reader walking it never reaches those pages at all — under a document that
	// says `/Marked true`. **The census rated that state `carried`, its best verdict.**
	//
	// Reachability from the tree is the property that matters and it catches both shapes with one
	// predicate: an appended untagged page (no `/StructParents`, nothing points at it) and an
	// appended tagged page (a `/StructParents` that lies, nothing points at it).
	//
	// A page with no content stream at all is NOT undescribed — an inserted blank page has nothing
	// to tag, and counting it would make `InsertBlank` a violation for adding an empty page.
	undescribed int
	// annots and formFields are counted for the COMMIT DOORS, not for any tagging question
	// (/pending 574). They ride this walk because it is already reading every page dictionary and
	// the catalog, and because the door that wants them is the one already paying for this parse
	// twice per commit — `noteCommitFate`. Counting them separately would have meant two more
	// full parses on every edit, measured at 88 ms each on a 1.4 MB document.
	//
	// `annots` is every annotation on every live page, of any subtype: the door reports what an
	// operation DESTROYED, and a sticky note and a link are both the user's to lose. `formFields`
	// is the catalog's `/AcroForm /Fields` length, which is the claim rather than the widgets —
	// deliberately, because /pending 573's whole finding was that the two can disagree.
	annots     int
	formFields int
	// annotKeys is one key per annotation counted in `annots`, in page order (`annotKey`) — what the
	// commit door compares by identity (`/pending 875`), as `fieldNames` is for fields. Nil when the
	// document holds more than `maxAnnotKeys`; the door then compares totals.
	annotKeys []string
	// fieldNames is every TERMINAL form field by its fully qualified name (`terminalFieldNames`) — what the
	// commit door compares by identity (`/pending 582`). `formFields` cannot say what was lost: a merge puts
	// the incoming document's fields under ONE new parent, so two fields arriving add one to that length.
	fieldNames []string
	// source is the tier `setTagSource` recorded on the tree, or "" when none is recorded (or the value
	// is not one this code writes). `claimTagging` reads it so a door adding to a tree never raises it.
	source tagSource
}

// claimsHonestly is a document that already makes a claim its structure can carry — the input a door
// may add to without making a new claim of its own (`claimTagging`, `/pending 495`).
func (s tagState) claimsHonestly() bool { return s.marked && s.supportsAClaim() }

// claims reports whether the document asserts tagging at all — either assertion law 1 names.
func (s tagState) claims() bool { return s.marked || s.tree }

// orphaned is law 1's violation: the document claims tagging, and the tree it advertises describes
// nothing that is in the document.
//
// **Both linkages have to be gone.** A tree anchors to content two ways — an element's `/Pg`
// pointing at a live page, and a page's `/StructParents` indexing into `/ParentTree` — and this
// refuses to strip while either survives. That conservatism is deliberate and is the direct lesson
// of the reverted version, which stripped on a predicate that could not see the tree at all.
//
// **Except when there is no tree, which the conservatism must not extend to.** `/MarkInfo /Marked
// true` with no `/StructTreeRoot` is a conformance assertion over nothing at all, and a page's
// `/StructParents` then indexes a `/ParentTree` that does not exist — so the second linkage is not
// a surviving anchor, it is a dangling number. Without this clause such a document scored
// `carried`, the census's BEST verdict, for having no structure whatsoever. Found by the P01 phase
// review; not reachable through any operation measured today, because pdfcpu drops `/MarkInfo` and
// the tree together, which is exactly why nothing had caught it.
func (s tagState) orphaned() bool {
	if !s.readable || !s.claims() {
		return false
	}
	if !s.tree {
		return true
	}
	return s.anchored == 0 && s.pagesSP == 0
}

// supportsAClaim says whether this document's structure could honestly carry a claim of tagging —
// `orphaned()` asked BEFORE the claim exists rather than after.
//
// It cannot be written as `!orphaned()`, and the reason is the one thing that makes `orphaned()`
// work: `orphaned()` returns false for a document that claims nothing, because a document making no
// claim cannot be lying. So a document with no tree at all is not orphaned — it becomes orphaned the
// moment `/MarkInfo` is written. A caller deciding whether to write it needs the question asked the
// other way round.
func (s tagState) supportsAClaim() bool {
	return s.readable && s.tree && (s.anchored > 0 || s.pagesSP > 0)
}

// partial is a claim over a document where the tree is live but does not reach every page that has
// content. Recorded rather than enforced: see the tag-fate table's `partial` verdict.
func (s tagState) partial() bool {
	return s.readable && s.claims() && !s.orphaned() && s.undescribed > 0
}

// inspectTags parses a document and reports its tag state. **It parses; it never counts bytes.**
func inspectTags(pdf []byte) tagState {
	var s tagState
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return s // unreadable: not a claim we can judge, and readable=false says so
	}
	s.readable = true
	s.pages = ctx.PageCount
	live := map[int]bool{}       // object number of every page in the page tree
	hasContent := map[int]bool{} // ... of every page carrying a non-empty content stream
	overKeyed := false           // more annotations than maxAnnotKeys: no keys, the count alone
	for _, pa := range pdfread.Pages(ctx) {
		p, d, perr := pa.Nr, pa.Dict, pa.Err
		if perr != nil || d == nil {
			continue
		}
		// Through the one reader (/pending 644): this counts pages that DECLARE a key, which is its
		// `written`, whether or not the value is one.
		if _, _, written := structParentsOf(ctx.XRefTable, d); written {
			s.pagesSP++
		}
		ir, e := pa.Ref, pa.Err
		if e != nil || ir == nil {
			continue
		}
		n := ir.ObjectNumber.Value()
		live[n] = true
		// `ErrNoContent` from an absent `/Contents` is the inserted-blank-page case and is not a
		// gap: a page with nothing on it has nothing to tag.
		// Only that error (/pending 727): a page whose content could not be READ is not known to be
		// blank, so it counts as content, and a tree that does not describe it is `undescribed` — the
		// unknown reads as the gap it may be, never as the blank page it may not be.
		if b, cerr := pdfread.PageContent(ctx, d, p); (cerr == nil && len(b) > 0) || (cerr != nil && cerr != model.ErrNoContent) {
			hasContent[n] = true
		}
		if a, aerr := ctx.DereferenceArray(d["Annots"]); aerr == nil {
			s.annots += len(a)
			if !overKeyed {
				for _, o := range a {
					s.annotKeys = append(s.annotKeys, annotKey(ctx, o))
				}
				if overKeyed = len(s.annotKeys) > maxAnnotKeys; overKeyed {
					s.annotKeys = nil
				}
			}
		}
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return s
	}
	s.marked, _ = taggingClaimOf(ctx, cat)
	// Counted BEFORE the untagged early return below, or every untagged document — which is most of
	// them — would report zero fields and an operation that destroyed a form would say nothing.
	if form, ferr := ctx.DereferenceDict(cat["AcroForm"]); ferr == nil && form != nil {
		if fields, aerr := ctx.DereferenceArray(form["Fields"]); aerr == nil {
			s.formFields = len(fields)
			s.fieldNames = terminalFieldNames(ctx, fields)
		}
	}
	if _, ok := cat["StructTreeRoot"]; !ok {
		return s
	}
	s.tree = true
	if root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"]); rerr == nil && root != nil {
		if n, isName := root[tagSourceKey].(types.Name); isName && tagSource(n.Value()).valid() {
			s.source = tagSource(n.Value())
		}
	}

	// **The walk lives in `structtree.go` now — P05.S02, D8.** It was inline here, with a visited
	// set keyed on the dictionary's CONTENT (`d.String()`), which counted two byte-identical
	// `/StructElem` dictionaries as ONE. Measured on a twin-element fixture: it reported
	// `elements=1` for a document containing two. Every figure this repo recorded from that oracle
	// was a lower bound.
	//
	// Re-expressing it on the model is D8's rule — *every operation in law 2 that reports `carried`
	// routes through this one model* — and ADR-009's: two traversals of one tree, keyed
	// differently, is the drift that was already measured rather than a risk.
	//
	// A tree the model REFUSES leaves the counts at zero while `s.tree` stays true, which is
	// exactly what `orphaned()` should say about a document asserting a structure nothing can read.
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		return s
	}
	s.elements = tree.elements()
	s.anchored = tree.anchored()
	described := tree.describedPages()
	for n := range hasContent {
		if !described[n] {
			s.undescribed++
		}
	}
	return s
}

// taggingClaimOf reads the two assertions law 1 names off a catalog: `/MarkInfo /Marked true`, and a
// `/StructTreeRoot` key. It is the ONE reading of both — `inspectTags` and the server's per-answer read
// (`ReadAnswerFacts`) ask it, so "is this document tagged" cannot come to mean two things.
func taggingClaimOf(ctx *model.Context, cat types.Dict) (marked, tree bool) {
	if mi, ok := cat["MarkInfo"]; ok {
		if d, e := ctx.DereferenceDict(mi); e == nil && d != nil {
			// readBool dereferences: `BooleanEntry` reads an indirect /Marked as absent (/pending 489).
			marked = readBool(ctx.XRefTable, d["Marked"])
		}
	}
	_, tree = cat["StructTreeRoot"]
	return marked, tree
}

// ClaimsTagging reports whether a document asserts that it is tagged — **either** assertion law 1
// names, because `/MarkInfo /Marked true` with no tree is as much a claim as a tree is.
//
// The server's tagging notice asks this before and after a mutation; it is the only exported
// member of this file.
func ClaimsTagging(pdf []byte) bool { return inspectTags(pdf).claims() }

// DocumentFacts is what a commit door needs in order to say what an operation cost the user
// (/pending 574).
//
// **One door and one parse, because the alternative is 33 sites that have to remember.**
// `noteTaggingFate`'s own comment made that ruling for the tagging claim — *"Asking each operation
// to report its own fate would be ADR-009's rule inverted: 33 sites that have to remember, against
// two that cannot"* — and it holds harder here: the counts are wanted for EVERY operation, and the
// two commit doors are the only places that see a document's before and after together.
//
// **Free, in the sense that matters.** The commit door already calls `ClaimsTagging` on the input
// and on the result, and each of those is this same parse; `Inspect` returns what that walk saw
// instead of one boolean from it. A separate counting pass would have been two more parses per
// edit, measured at 88 ms each on a 1.4 MB document.
//
// `Readable` is false for bytes this package cannot parse, and then no other field means anything —
// a caller must not read "0 annotations" off a document it could not open.
type DocumentFacts struct {
	Readable    bool
	Tagged      bool
	Annotations int
	// FieldNames is every terminal form field by its fully qualified name, in the form's order — the
	// fields themselves. The length of the form's top-level array was published here as `FormFields`
	// until `/pending 582`: it is not a count of fields (a merge files two incoming fields under one
	// new parent), and nothing outside this package read it once the commit door compared names.
	FieldNames []string
	// AnnotationKeys is one key per annotation — what it is, not where it is (`annotKey`) — or nil
	// when there are too many to key; then only `Annotations` says anything.
	AnnotationKeys []string
}

// Inspect reports the facts a commit door compares across an operation. See DocumentFacts.
func Inspect(pdf []byte) DocumentFacts {
	s := inspectTags(pdf)
	return DocumentFacts{
		Readable:    s.readable,
		Tagged:      s.claims(),
		Annotations: s.annots,
		FieldNames:  s.fieldNames,

		AnnotationKeys: s.annotKeys,
	}
}

// maxAnnotKeys bounds the keys kept for one document: past it the commit door compares totals, as it
// did for every document before `/pending 875`.
const maxAnnotKeys = 100000

// annotKey names an annotation by what survives a rewrite: its `/Subtype`, its `/NM`, a digest of its
// `/Contents`, and its `/T` (a widget's partial field name, a note's author).
//
// **What is NOT in it, each by measurement (/pending 875).** The object number: pdfcpu renumbers on every
// write. The page: a reorder moves an annotation and loses nothing. The `/Rect`: an n-up carries a sticky
// note onto its sheet at a new place — keyed with the rectangle, the two notes it carried read as two lost
// and two arrived, a loss notice for comments that are on the page.
//
// **It is not unique and need not be.** Two links with no text share a key; the door counts keys as a
// multiset, so sharing costs precision and never a false report. What matters is that no operation changes
// a key of an annotation it KEEPS — `TestNoOperationRenamesAnAnnotationItKeeps` drives every operation in
// the census and fails on the first that does.
//
// **What it costs, measured**: 0.17 ms, 31 ms and 604 ms for 100, 10,000 and 100,000 annotations, on a parse
// of 9.8 ms, 589 ms and 8.2 s that the commit door already makes — a twentieth of it, outside the server's
// lock (`fateOf`).
func annotKey(ctx *model.Context, o types.Object) string {
	d, err := ctx.DereferenceDict(o)
	if err != nil || d == nil {
		return "?"
	}
	text := func(key string) string {
		if d[key] == nil {
			return ""
		}
		s, err := ctx.DereferenceStringOrHexLiteral(d[key], model.V10, nil)
		if err != nil {
			return "?"
		}
		return s
	}
	subtype := ""
	if n, err := ctx.DereferenceName(d["Subtype"], model.V10, nil); err == nil {
		subtype = n.Value()
	}
	contents := ""
	if c := text("Contents"); c != "" {
		sum := sha256.Sum256([]byte(c))
		contents = hex.EncodeToString(sum[:8])
	}
	return subtype + "\x00" + text("NM") + "\x00" + contents + "\x00" + text("T")
}

// maxFieldNodes bounds the field walk: a form is a tree a stranger wrote, read on every commit.
const maxFieldNodes = 100000

// terminalFieldNames is every terminal field under an `/AcroForm /Fields` array, by fully qualified name
// (ISO 32000-1 12.7.3.2: the partial names from the root down, joined by a period). A field is terminal when
// none of its kids has a `/T` — its kids, if any, are its widgets. A node that only groups other fields is not
// one. Each object is visited once, to a depth of 32 and `maxFieldNodes` in all, so a looped or enormous tree
// costs a bounded walk and reports what it reached.
//
// **Measured (/pending 875)**: 82 µs, 23 ms and 100 ms for 100, 10,000 and 100,000 fields (131 ms for 200,000,
// where the bound stops it at 100,000 names), beside the parse it follows — 6 ms, 459 ms and 6.9 s, which is
// pdfcpu validating that many fields and is what a form of that size costs to open at all.
func terminalFieldNames(ctx *model.Context, fields types.Array) []string {
	var out []string
	seen := map[int]bool{}
	nodes := 0
	var walk func(o types.Object, prefix string, depth int)
	walk = func(o types.Object, prefix string, depth int) {
		if nodes++; nodes > maxFieldNodes || depth > 32 {
			return
		}
		if ir, ok := o.(types.IndirectRef); ok {
			if seen[ir.ObjectNumber.Value()] {
				return
			}
			seen[ir.ObjectNumber.Value()] = true
		}
		d, err := ctx.DereferenceDict(o)
		if err != nil || d == nil {
			return
		}
		name := prefix
		if d["T"] != nil {
			if t, terr := ctx.DereferenceStringOrHexLiteral(d["T"], model.V10, nil); terr == nil {
				if name != "" {
					name += "."
				}
				name += t
			}
		}
		kids, _ := ctx.DereferenceArray(d["Kids"])
		terminal := true
		for _, k := range kids {
			if kd, kerr := ctx.DereferenceDict(k); kerr == nil && kd != nil && kd["T"] != nil {
				terminal = false
				walk(k, name, depth+1)
			}
		}
		if terminal {
			out = append(out, name)
		}
	}
	for _, f := range fields {
		walk(f, "", 0)
	}
	return out
}

// honest is the one door. It returns the document unchanged unless the document claims tagging over
// a tree that describes nothing in it, in which case it removes the claim.
//
// **The unchanged path costs a parse and never a rewrite**, which matters for more than speed: a
// rewrite re-encodes, which moves bytes and invalidates any signature over them.
//
// **Measured at 88 ms on a 1.4 MB, 22-page document** — the parse, against `api.NUp`'s own 147 ms
// for the composition. Its caller asks `inspectTags(...).orphaned()` directly before attempting a
// carry, so since v1.129.19 `honest` is reached only by a document whose tree was re-anchored, and
// what it costs there is the verification of that remap rather than a toll on every n-up.
//
// **There is a cheap pre-filter available and it is deliberately NOT taken.** The catalog-level keys
// do survive uncompressed, so a byte scan for `/StructTreeRoot` would skip the parse for the
// overwhelmingly common untagged document. A false negative there — a catalog written into an
// object stream, which PDF 1.5 permits — would silently switch law 1 off for `NUp` and leave no
// trace, and that is the exact failure this file exists because of. 123 ms is cheaper than finding
// out.
func honest(pdf []byte) ([]byte, error) {
	if !inspectTags(pdf).orphaned() {
		return pdf, nil
	}
	return dropTaggingClaim(pdf)
}

// dropTaggingClaim removes `/StructTreeRoot` and `/MarkInfo` from the catalog.
//
// **Unexported and called only through `honest`**, so the decision to strip is taken in exactly one
// place against exactly one predicate. The reverted version had the strip reachable on its own, and
// that is how it came to run over documents whose trees were intact.
//
// **That sentence was false for one commit** (v1.129.19): `NUp` called this directly, having already
// established `orphaned`, to save a re-parse. The saving was real and the precedent was not worth
// it — a second caller is how the predicate and the strip drift apart, and this comment asserting
// otherwise is the shape of defect this repo keeps paying for. Restored to one caller at the P01
// phase review; `zerocaller_test.go` cannot police this, because the function is unexported and has
// a caller either way.
func dropTaggingClaim(pdf []byte) ([]byte, error) {
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return nil, err
	}
	delete(cat, "StructTreeRoot")
	delete(cat, "MarkInfo")
	out, err := pdfread.Write(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}
