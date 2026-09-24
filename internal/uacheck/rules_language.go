package uacheck

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Language — `PLAN-ua-coverage.md` P04. Each rule is veraPDF's own property, read from its source
// (veraPDF-validation `GFPDDocument`, `GFPDStructElem`, `GFPDAnnot`, `GFPDFormField`, `GFOpMarkedContent` and the
// profile's `gContainsCatalogLang` variable) and then measured on veraPDF 1.30.2 before it was written.

func init() {
	register(Rule{Clause: "7.2 t2", Summary: "natural language in the outline entries shall be determined", Check: checkOutlineLanguage})
	register(Rule{Clause: "7.2 t29", Summary: "a /Lang value shall be a language identifier (ISO 32000-1 14.9.2)", Check: checkLanguageIdentifiers})
	for _, k := range alternateTextKeys {
		key, what := k.key, k.what
		register(Rule{Clause: k.clause, Summary: k.summary, Check: func(d *Document) Result {
			return checkAlternateTextLanguage(d, key, what)
		}})
	}
	for _, k := range associatedTextKeys {
		row := k
		register(Rule{Clause: row.clause, Summary: row.summary, Check: func(d *Document) Result {
			return checkAssociatedTextLanguage(d, row)
		}})
	}
}

// alternateTextKey is one row of the table below: the clause, the key it names, and the words the report uses.
type alternateTextKey struct{ clause, key, summary, what string }

// alternateTextKeys is ua1 7.2 t21, t22 and t23 — **one relation with three rows**, because veraPDF's three
// predicates differ only in which key they name:
//
//	<key> == null || containsLang == true || parentLang != null || gContainsCatalogLang == true
//
// `TestTheAlternateTextLanguageClausesAreOneRelation` holds that no clause is written outside the table, and
// `checkAlternateTextLanguage` is the only function that evaluates one (ADR-009).
var alternateTextKeys = []alternateTextKey{
	{clause: "7.2 t21", key: "ActualText", summary: "natural language for text in the ActualText attribute shall be determined",
		what: "replacement text (/ActualText)"},
	{clause: "7.2 t22", key: "Alt", summary: "natural language for text in the Alt attribute shall be determined",
		what: "an alternate description (/Alt)"},
	{clause: "7.2 t23", key: "E", summary: "natural language for text in the E attribute shall be determined",
		what: "the expansion of an abbreviation (/E)"},
}

// checkAlternateTextLanguage evaluates one row of `alternateTextKeys`, measured on veraPDF 1.30.2 over the
// slice's eighty fixtures (P04.S02's grill note in `PLAN-ua-coverage.md` has the table).
//
// **The subject is every structure element, not every element carrying the key.** veraPDF runs one check per
// `PDStructElem` and passes it where the key is absent, so a tagged document with no `/Alt` anywhere reports the
// clause PASSED with checks, never "no subject" — measured. The clause is NotApplicable only where the tree holds
// no element at all.
//
// **In a document nib can read, the key is a string.** veraPDF reads `/Alt` and `/E` through `getStringKey`,
// which returns null for a NAME, and `/ActualText` through `getKey(…).getString()`, which does not — so a
// name-typed `/ActualText` is a subject there and fails. It cannot arrive here: pdfcpu's validator refuses a
// name-, number-, boolean-, array- or dictionary-typed value on all three keys (33 combinations measured), which
// leaves the string literal, the hex string, the indirect string and `null` — exactly `d.text`. An EMPTY string
// is a subject (`/Alt ()` fails with no language), unlike 7.3 t1, which wants a non-empty one.
func checkAlternateTextLanguage(d *Document, key, what string) Result {
	nodes, unread := d.structNodes()
	// **The catalog settles the clause without reading an element.** Its `/Lang` satisfies the predicate for
	// every check veraPDF runs, so a document that has one passes even where nib could not finish the tree —
	// answering CannotCheck there would be a refusal over a question the catalog already answered.
	if catalogDeclaresLang(d) {
		// No `unread` clause here, and that is deliberate: `structNodes` refuses only an element it met BELOW the
		// root, so it cannot report an unread tree without having returned the elements above the refusal —
		// "no elements" and "part of the tree was unread" cannot both hold. A conjunct no input can falsify is
		// the dead clause P03's review went looking for, and the slice's blind mutation pass found this one.
		if len(nodes) == 0 {
			return Result{Verdict: NotApplicable, Why: "the document has no structure elements"}
		}
		return Result{Verdict: Pass}
	}
	cannot := ""
	for _, n := range nodes {
		if _, ok := d.text(n.dict[key]); !ok {
			continue
		}
		if _, own := d.text(n.dict["Lang"]); own {
			continue
		}
		found, why := d.parentLang(n.dict)
		switch {
		case found:
		case why != "":
			if cannot == "" {
				cannot = fmt.Sprintf("%s: %s", nodeWhere(n, d.name(n.dict["S"])), why)
			}
		default:
			return Result{Verdict: Fail, Where: nodeWhere(n, d.name(n.dict["S"])),
				Why: fmt.Sprintf("the element carries %s and no /Lang, no ancestor it names through /P carries one, "+
					"and the catalog declares none, so the language of that text cannot be determined", what)}
		}
	}
	switch {
	case unread != "":
		return Result{Verdict: CannotCheck, Why: unread}
	case cannot != "":
		return Result{Verdict: CannotCheck, Why: cannot}
	case len(nodes) == 0:
		return Result{Verdict: NotApplicable, Why: "the document has no structure elements"}
	}
	return Result{Verdict: Pass}
}

// associatedTextKey is one row of the table below: the clause, which population it runs over, the key it
// names, and the words the report uses.
type associatedTextKey struct {
	clause, key, summary, what, none string
	// kind is the population veraPDF runs the clause over, STATED rather than derived. It was derived
	// from the clause id at first, by a two-way branch whose `else` silently made any third row a
	// form-field rule — a wrong default no test could catch, under a comment claiming the table
	// "cannot carry a row whose clause and population disagree".
	kind subjectKind
}

// associatedTextKeys is ua1 7.2 t24 and t25 — **one relation with two rows**, because veraPDF's two
// predicates differ only in the object they run over and the key they name:
//
//	<key> == null || containsLang == true || gContainsCatalogLang == true
//
// **`parentLang` is absent from that predicate and its absence is the whole slice.** 7.2 t21–t23 (a
// structure element's alternates) grant an ancestor's `/Lang` through `/P`; these two do not. An
// annotation's language comes from the ONE element its `/StructParent` names, or from the catalog, or
// nowhere — so reusing `checkAlternateTextLanguage`'s climb would pass documents veraPDF fails.
//
// `TestTheAssociatedTextLanguageClausesAreOneRelation` holds that no clause is written outside the
// table, and `checkAssociatedTextLanguage` is the only function that evaluates one (ADR-009).
var associatedTextKeys = []associatedTextKey{
	{clause: "7.2 t24", key: "Contents", kind: kindAnnotation,
		summary: "natural language in the Contents entry for annotations shall be determined",
		what:    "a text description (/Contents)", none: "the document has no annotations"},
	{clause: "7.2 t25", key: "TU", kind: kindFormField,
		summary: "natural language in the TU key for form fields shall be determined",
		what:    "an alternate field name (/TU)", none: "the document has no form fields"},
}

// checkAssociatedTextLanguage evaluates one row of `associatedTextKeys`.
//
// **The subject is every annotation (or every field), not every one carrying the key** — P04.S02's
// lesson, and it holds here for the same reason: veraPDF runs one check per object and passes it
// where the key is absent, so a document full of annotations with no `/Contents` reports the clause
// PASSED with checks. The clause is NotApplicable only where the population is empty.
//
// **An empty string is a language.** veraPDF's `containsLang` is `getLang() != null` and `getLang`
// requires only `COS_STRING`, so `/Lang ()` on the named element satisfies the predicate — the same
// present-not-valid reading `declaresLang` already documents for 7.2 t33/t34. Whether that value is a
// well-formed identifier is 7.2 t29's separate question.
//
// **A subject whose element nib never read is CannotCheck, never Fail.** An unresolved `/StructParent`
// and a parent-tree slot below the depth bound are the same observation from here (/pending 496), so a
// definite failure is only reported for a subject whose element nib DID read and which carries no
// `/Lang`. A document holding both answers Fail: a failure found is a failure whatever else was unread.
func checkAssociatedTextLanguage(d *Document, row associatedTextKey) Result {
	sc := d.scanAnnotsAndFields()
	kind := row.kind
	// **Only this rule's OWN population can be incomplete for it.** A truncated form-field walk says
	// nothing about annotations, and answering CannotCheck for t24 because t25's population was short
	// is a refusal over a question nib answered in full.
	short := sc.incompleteFor(kind)
	population := 0
	for _, s := range sc.subjects {
		if s.kind == kind {
			population++
		}
	}
	// **The catalog settles the clause without reading a subject** — its `/Lang` satisfies the
	// predicate for every check veraPDF runs, so answering CannotCheck over an unread parent tree
	// would be a refusal over a question the catalog has already answered.
	if catalogDeclaresLang(d) {
		// **`short` is consulted here for the same reason it precedes `population == 0` below**: an empty
		// population that nib never finished enumerating is not "the document has no annotations", it is a
		// claim nib did not establish. Found at the P04 close — this branch ordered the two the other way,
		// so a page whose `/Annots` does not dereference reported NotApplicable with a catalog `/Lang` and
		// CannotCheck without one, over the same unread document.
		if population == 0 && short == "" {
			return Result{Verdict: NotApplicable, Why: row.none}
		}
		if population == 0 {
			return Result{Verdict: CannotCheck, Why: short}
		}
		return Result{Verdict: Pass}
	}
	cannot := ""
	for _, s := range sc.subjects {
		if s.kind != kind {
			continue
		}
		if _, ok := d.text(s.holder[row.key]); !ok {
			continue
		}
		if s.elem != nil {
			if _, ok := d.text(s.elem["Lang"]); ok {
				continue
			}
			return Result{Verdict: Fail, Where: s.where,
				Why: fmt.Sprintf("it carries %s, the structure element it names through /StructParent declares no "+
					"/Lang, and the catalog declares none, so the language of that text cannot be determined", row.what)}
		}
		// **THIS subject's slot was unread — not "something somewhere was unread".** A holder naming no
		// `/StructParent` falls through to the definite failure below, which is what veraPDF reports.
		if s.unresolved != "" {
			if cannot == "" {
				cannot = fmt.Sprintf("%s: %s", s.where, s.unresolved)
			}
			continue
		}
		return Result{Verdict: Fail, Where: s.where,
			Why: fmt.Sprintf("it carries %s and names no structure element through /StructParent, and the catalog "+
				"declares no /Lang, so the language of that text cannot be determined", row.what)}
	}
	switch {
	case cannot != "":
		return Result{Verdict: CannotCheck, Why: cannot}
	// Reached only when every ENUMERATED subject was settled, so the question left is whether the
	// POPULATION is short: a member nib never enumerated could be the one that fails.
	case short != "":
		return Result{Verdict: CannotCheck, Why: short}
	case population == 0:
		return Result{Verdict: NotApplicable, Why: row.none}
	}
	return Result{Verdict: Pass}
}

// maxLangClimb is how many `/P` links one climb may follow; see parentLang for why it is one more than the tree
// walk's own bound.
const maxLangClimb = maxWalkDepth + 1

// langClimbTooFar is the refusal past that bound, built once rather than per call.
var langClimbTooFar = fmt.Sprintf("its /P chain climbs through more than %d ancestors; nib stops climbing there, "+
	"so an ancestor language past that point was never read", maxLangClimb)

// langClimb is one memoised climb: whether the dictionary's own `/P` chain determines a language, and how many
// links above the dictionary the answer sits — `dist` is P03's `kidsResult.height` in the other direction, and it
// is what makes a memo hit answerable for a climb that met the dictionary deeper.
//
// **The language itself is not kept**, because no rule asks which language an ancestor declared — only whether
// one did. P04.S04's `inheritedLang` may want the value; it can add the field when it has a reader.
type langClimb struct {
	found bool
	dist  int
}

// parentLang is veraPDF's `GFPDStructElem.getparentLang` — whether any `/Lang` string sits on the element's `/P`
// chain, not counting its own. The second result is why the climb did not finish, and it is `CannotCheck`, never
// a Pass.
//
// **The climb is blind and it does not stop at the structure tree.** veraPDF's `getParent()` wraps whatever `/P`
// names and reads its `/Lang`, so — measured on 1.30.2 — a `/Lang` on the **StructTreeRoot** satisfies the rule, as
// does one on a `/P` naming a **non-ancestor element** or a **plain non-element dictionary**. Nothing is skipped
// either: a `Div` ancestor's `/Lang` counts, which is the opposite of `significantParent`, whose job is to climb
// PAST the pass-through types. The two climbs answer different questions and are deliberately not one door.
//
// **It is the ONLY climb for this question now** (P04.S04). `structure.go` used to carry a second,
// `declaresLangFor`, which 7.2 t34 read and which diverged on three shapes — a `/Lang` on the StructTreeRoot
// (veraPDF passes, it returned false at the root test), a `/P` cycle with no `/Lang`, and the bound's
// arithmetic. That was `/pending 635`, a live false FAIL in a shipped clause; t34 now reads `inheritedLangOf`,
// which reads this, and the second climb is gone (ADR-009).
//
// **A cycle is a complete answer, not a refusal.** veraPDF seeds its loop guard with the element's own key and
// returns null on a revisit, so `/P` naming itself and a true two-element cycle with no `/Lang` both FAIL there
// (measured) — every ancestor was seen and none carried a language. `significantParent` reports "parent
// unreadable" for the same shape because for containment the parent is the answer; here it is only the road.
//
// **The chain is unbounded in the input, so the climb is bounded.** The tree itself cannot be deep — pdfcpu
// refuses a structure tree past 100 levels, and `structNodes` stops at `maxWalkDepth` before that — but a
// SIDEWAYS chain hanging off one shallow element is not the tree: 5,000 links is readable and veraPDF passes it.
//
// **The bound is `maxWalkDepth + 1`, and the `+ 1` is the StructTreeRoot.** The deepest element the tree walk
// admits sits at walk depth `maxWalkDepth`, which is `maxWalkDepth` element ancestors AND the root above them —
// a chain the walk never counts, because it starts below it. At a bare `maxWalkDepth` the climb refused that
// element: measured, a 65-deep tree whose only `/Lang` is on the StructTreeRoot answered CannotCheck where
// veraPDF passes, while 63 and 64 deep passed. With the `+ 1` the claim this bound is chosen for is true — an
// element deep enough to exhaust the climb is already past the walk's own bound, where every rule answers
// CannotCheck for the whole tree.
//
// **A refusal is never memoised, and a memo hit carries its distance** — P03's kid-depth lesson, that an answer
// must not depend on which element asked first. A dictionary's climb is memoised with how far above it the
// answer sits, so a later climb that meets it deeper is refused exactly where a fresh walk would have been.
// A climb that ended in a cycle is not memoised either: how many links a cycle costs depends on where it was
// entered, and a pathological shape is not worth a wrong memo.
func (d *Document) parentLang(elem types.Dict) (bool, string) {
	if d.langs == nil {
		d.langs = map[uintptr]langClimb{}
	}
	// veraPDF seeds its loop guard with the element's OWN key, so a /P that names the element itself ends the
	// climb with no answer rather than reading the element's own /Lang as an ancestor's.
	onPath := map[uintptr]bool{dictID(elem): true}
	var path []uintptr
	answer, tail, looped := langClimb{}, 0, false
	for p := d.dict(elem["P"]); p != nil; p = d.dict(p["P"]) {
		id := dictID(p)
		if onPath[id] {
			looped = true
			break
		}
		if len(path) >= maxLangClimb {
			return false, langClimbTooFar
		}
		if memo, ok := d.langs[id]; ok {
			// Only a dictionary with no `/Lang` of its own is ever memoised — the one that HOLDS the language is
			// left out below — so the memo is this asker's answer too, if its own climb reaches that far.
			if len(path)+1+memo.dist > maxLangClimb {
				return false, langClimbTooFar
			}
			answer, tail = memo, 1+memo.dist
			break
		}
		onPath[id] = true
		if _, ok := d.text(p["Lang"]); ok {
			answer, tail = langClimb{found: true}, 1
			break
		}
		path = append(path, id)
	}
	if looped {
		// A cycle is reached only with `answer` still zero: every assignment to it breaks the loop.
		return false, ""
	}
	// Every dictionary the climb passed ends at the same answer, each one link nearer to it than the one below.
	for i, id := range path {
		d.langs[id] = langClimb{found: answer.found, dist: len(path) - 1 - i + tail}
	}
	return answer.found, ""
}

// catalogLang is the catalog's `/Lang` and whether it is a string — the one place the catalog's language is read.
//
// It is veraPDF's `gContainsCatalogLang` (the profile variable, set from `GFPDDocument.getcontainsLang`): the key
// is present and a string. **Measured on 1.30.2: an empty `()` counts, an indirect string counts, a name does not.**
// Every rule that asks whether the catalog determines a language asks here (`catalogDeclaresLang`), and
// `TestTheCatalogLanguageIsReadThroughOneDoor` holds that nothing reads the key another way.
func (d *Document) catalogLang() (string, bool) {
	return d.text(d.Catalog["Lang"])
}

// checkOutlineLanguage evaluates ua1 7.2 t2. veraPDF's subject is each outline item (`PDOutline`) and its test is
// the catalog variable alone, so the verdict is "is there an item, and does the catalog determine a language".
// Measured: no catalog /Lang fails, `()` passes, `/en` fails, an indirect string passes; no item is no subject.
// The `/First` half cannot be reached through nib's reader today — pdfcpu's validator drops an `/Outlines` whose
// `/First` is missing or dangling (both measured in the slice review's fix pass) — and it stays as veraPDF's
// subject definition, so a reader that kept such a dictionary would still answer "no entries", not fail.
func checkOutlineLanguage(d *Document) Result {
	outlines := d.dict(d.Catalog["Outlines"])
	if outlines == nil || d.dict(outlines["First"]) == nil {
		return Result{Verdict: NotApplicable, Why: "the document has no outline entries"}
	}
	if catalogDeclaresLang(d) {
		return Result{Verdict: Pass}
	}
	return Result{Verdict: Fail, Where: "catalog /Outlines",
		Why: "the document has outline entries and the catalog declares no /Lang, so their language cannot be determined"}
}

// languageTag is ISO 32000-1 14.9.2's language identifier as veraPDF tests it (`CosLang`, 7.2-29), over the
// decoded value: measured, `()`, `en_US`, `en-`, a nine-letter subtag, `1en` and `en US` fail; `x-klingon` and a
// UTF-16 `en-US` pass.
var languageTag = regexp.MustCompile(`^[a-zA-Z]{1,8}(-[a-zA-Z0-9]{1,8})*$`)

// checkLanguageIdentifiers evaluates ua1 7.2 t29 over veraPDF's CosLang population — every STRING `/Lang` on the
// catalog, on a structure element, on the element an annotation's or a form field's `/StructParent` names (measured
// with an element reachable ONLY that way), and on a BDC property list (inline, or named in `/Properties` and
// used). A `/Lang` that is not a string is no subject, a property list defined but never used is none, and a DP's
// property list is none either — veraPDF 1.30.2 passes a bad `/Lang` there (measured, `recordLang`).
//
// **Marked content inside a tiling pattern or a Type 3 glyph procedure is read by veraPDF when it is used, and
// **Since P04.S04 the walk DOES enter them**, so such a value is a definite Fail rather than a refusal: a
// tiling pattern is read once `scn` selects it and a Type 3 font's every `CharProc` once any glyph is shown in
// it, in a lang-only mode that emits no content event and no marked-content subject. `unwalkedBadLang`, which
// read what was DEFINED rather than what is drawn and so could only ever answer CannotCheck, retired with it.
func checkLanguageIdentifiers(d *Document) Result {
	subjects := 0
	check := func(value, where string) *Result {
		subjects++
		if languageTag.MatchString(value) {
			return nil
		}
		return &Result{Verdict: Fail, Where: where,
			Why: fmt.Sprintf("the /Lang value %q is not a language identifier (a primary tag of 1-8 letters, then subtags of 1-8 letters or digits, joined by hyphens)", value)}
	}
	if v, ok := d.catalogLang(); ok {
		if r := check(v, "catalog /Lang"); r != nil {
			return *r
		}
	}
	nodes, unread := d.structNodes()
	for _, n := range nodes {
		if v, ok := d.text(n.dict["Lang"]); ok {
			if r := check(v, nodeWhere(n, d.name(n.dict["S"]))); r != nil {
				return *r
			}
		}
	}
	parents, ptWhy := d.structParentsOfAnnotsAndFields()
	for _, sp := range parents {
		if v, ok := d.text(sp.elem["Lang"]); ok {
			if r := check(v, sp.where); r != nil {
				return *r
			}
		}
	}
	_, contentWhy := d.contentEvents()
	subjects += d.mcLangCount
	if b := d.mcLangBad; b != nil {
		return Result{Verdict: Fail, Where: b.where,
			Why: fmt.Sprintf("the /Lang value %q is not a language identifier (a primary tag of 1-8 letters, then subtags of 1-8 letters or digits, joined by hyphens)", b.value)}
	}
	switch {
	case unread != "":
		return Result{Verdict: CannotCheck, Why: unread}
	case ptWhy != "":
		return Result{Verdict: CannotCheck, Why: ptWhy}
	case contentWhy != "":
		return Result{Verdict: CannotCheck, Why: contentWhy}
	}
	if subjects == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no /Lang entry"}
	}
	return Result{Verdict: Pass}
}

// subjectKind says which population a `langSubject` belongs to: veraPDF runs 7.2 t24 over `PDAnnot`
// and t25 over `PDFormField`, and one traversal reaches both.
type subjectKind int

const (
	kindAnnotation subjectKind = iota
	kindFormField
)

// langSubject is one annotation or one form field, as three facts that three different rules read.
//
// **`holder` and `elem` are different dictionaries and the distinction is the slice.** 7.2 t24 and t25
// read a key on the ANNOTATION or the FIELD (`/Contents`, `/TU`) and then ask whether a language is
// determined for it; 7.2 t29 reads the `/Lang` on the structure ELEMENT that `/StructParent` names.
// A traversal that yielded only the element could not evaluate t24 or t25, and one that yielded only
// the holder could not evaluate t29.
type langSubject struct {
	kind   subjectKind
	holder types.Dict
	where  string
	// elem is the structure element the holder names through `/StructParent` and the parent tree, or
	// nil when there is none to name.
	elem types.Dict
	// unresolved is why `elem` is nil DESPITE the holder naming a key — the parent tree stopped at its
	// depth bound before reaching that slot — and it is empty when the holder names no element at all.
	//
	// **The two are not the same fact, and treating them as one loses failures.** A holder with no
	// `/StructParent` is definitively unlanguaged: veraPDF's `getLang` returns null, `containsLang` is
	// false, and the clause FAILS. A holder whose slot nib never read is a question nib cannot answer.
	// This field was absent at first and one document-wide string carried both, so a deep parent tree
	// anywhere in the file downgraded every definite failure in it to CannotCheck — including failures
	// in the OTHER population, since the string was not partitioned by kind either.
	unresolved string
}

// subjectScan is one traversal's result: the subjects, and — separately per population — why that
// population may be missing members nib never enumerated.
//
// **Incompleteness of a POPULATION and unreadability of one SUBJECT are different facts**, and they
// belong to different verdicts. A truncated form-field walk says nothing about annotations, and a
// parent-tree slot nib did not read says nothing about whether some other subject is missing.
type subjectScan struct {
	subjects []langSubject
	// missed is 7.2 t29's reader, first-wins, with the semantics it had before this struct existed.
	missed string
	// annots and fields are why that population may be short of members.
	annots, fields string
}

// incompleteFor is why the population a rule runs over may be missing members, or empty.
func (sc subjectScan) incompleteFor(k subjectKind) string {
	if k == kindAnnotation {
		return sc.annots
	}
	return sc.fields
}

// structParentSite is a structure element reached from an annotation's or a field's `/StructParent`.
type structParentSite struct {
	elem  types.Dict
	where string
}

// structParentsOfAnnotsAndFields is 7.2 t29's reader: the elements that resolve, and nothing else.
//
// It is a filter over `annotAndFieldSubjects` rather than its own walk, because the two questions
// share one traversal and ADR-009 gives a rule one door. t29 wants the `/Lang` values that EXIST, so
// a subject naming no element is not its business; t24 and t25 want every subject, because a subject
// with no element is precisely the one whose language cannot be determined.
func (d *Document) structParentsOfAnnotsAndFields() ([]structParentSite, string) {
	subjects, missed := d.annotAndFieldSubjects()
	var out []structParentSite
	for _, s := range subjects {
		if s.elem != nil {
			out = append(out, structParentSite{elem: s.elem, where: s.where + " → its /StructParent element"})
		}
	}
	return out, missed
}

// annotAndFieldSubjects is every annotation on every page and every form field in the AcroForm tree,
// each paired with the structure element it names through `/StructParent` and the parent tree.
//
// **The populations are veraPDF's, read from its source.** `GFPDPage.parseAnnotations` builds a
// `PDAnnot` for every entry of a page's `/Annots` and filters NOTHING — `GFPDAnnot.createAnnot`
// switches on the subtype only to pick a subclass, and every subclass IS a `PDAnnot`, so no subtype
// is excluded. `GFPDAcroForm.getFormFields` takes the AcroForm's `/Fields`, and `GFPDFormField`
// exposes its `/Kids` as linked objects, so the profile's object graph reaches nested fields.
//
// **The `/StructParent` hop is an ASSOCIATION, not an ancestor climb**, and that is what separates
// these rules from 7.2 t21–t23. `GFPDAnnot.getLang` and `GFPDFormField.getLang` take the holder's
// `/StructParent`, look it up in the parent tree, and read that ONE element's own `/Lang`, requiring
// a string. There is no `/P` walk: `parentLang` is a structure element's rule and must not be reused
// here, or an annotation would inherit a language veraPDF never gives it.
//
// The second result is why part could not be read.
func (d *Document) annotAndFieldSubjects() ([]langSubject, string) {
	sc := d.scanAnnotsAndFields()
	return sc.subjects, sc.missed
}

// scanAnnotsAndFields performs the traversal. See `annotAndFieldSubjects` for what it reads and why.
func (d *Document) scanAnnotsAndFields() subjectScan {
	pt, ptWhy := d.parentTree()
	sc := subjectScan{}
	add := func(kind subjectKind, holder types.Dict, where string) {
		s := langSubject{kind: kind, holder: holder, where: where}
		if sp, ok := d.intValue(holder["StructParent"]); ok {
			if elem := d.dict(pt[sp]); elem != nil {
				s.elem = elem
			} else if ptWhy != "" {
				// The slot is absent from a tree nib did not finish reading, so for THIS subject "it
				// names no element" and "nib never read the slot" are indistinguishable (/pending 496).
				// It is recorded on the subject; a holder with no `/StructParent` at all is a different
				// and definite answer, and must not be swept up by it.
				s.unresolved = ptWhy
				if sc.missed == "" {
					sc.missed = ptWhy
				}
			}
		}
		sc.subjects = append(sc.subjects, s)
	}
	// A page nib cannot read truncates BOTH populations: the annotations it holds are never seen, and
	// the walk returns before the AcroForm is reached at all.
	truncateAll := func(why string) subjectScan {
		sc.missed, sc.annots, sc.fields = why, why, why
		return sc
	}
	// The annotation population is the shared door's (`annots.go`, ADR-009) — this walk used to be one
	// of three enumerations of `/Annots`. A door that stopped short still hands back what it read, so
	// the subjects before the unreadable page are kept and the reason truncates both populations.
	annots, annotsMissed := d.annots()
	for _, a := range annots {
		add(kindAnnotation, a.dict, a.where)
	}
	if annotsMissed != "" {
		return truncateAll(annotsMissed)
	}
	seen := map[uintptr]bool{}
	var fields func(o types.Object, depth int)
	fields = func(o types.Object, depth int) {
		arr, err := d.Ctx.DereferenceArray(o)
		if err != nil || arr == nil {
			return
		}
		for _, f := range arr {
			fd := d.dict(f)
			if fd == nil || seen[dictID(fd)] {
				continue
			}
			if depth > maxWalkDepth {
				why := fmt.Sprintf("the form field tree nests deeper than %d levels; nib stops reading there", maxWalkDepth)
				if sc.missed == "" {
					sc.missed = why
				}
				// The FIELD population is short of members; the annotations were all enumerated above.
				sc.fields = why
				return
			}
			seen[dictID(fd)] = true
			add(kindFormField, fd, "form field")
			fields(fd["Kids"], depth+1)
		}
	}
	if acro := d.dict(d.Catalog["AcroForm"]); acro != nil {
		fields(acro["Fields"], 0)
	}
	return sc
}

// mcLang is a string `/Lang` on a BDC property list the content walk reached — kept only for the first that fails.
type mcLang struct {
	value, where string
}

// sortedKeys is a dictionary's keys in order, so a first hit is reported the same way every run.
func sortedKeys(d types.Dict) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// hasInlineType3Font reports whether the file, parsed WITHOUT validation, holds a Type 3 font dictionary written
// inline — nested in another object rather than an object of its own — which is the shape pdfcpu's validator
// drops. Asked only when a font entry is missing, and memoised; a file that will not parse raw answers true,
// because nib then cannot rule the dropped font out. Its one caller is `content.go`'s `enterType3`, which reads
// every `CharProc` of a Type 3 font a glyph is shown in — so a font the validator dropped is marked content nib
// could not read, never marked content that is not there.
func (d *Document) hasInlineType3Font() bool {
	if d.directType3 != nil {
		return *d.directType3
	}
	found := d.scanInlineType3()
	d.directType3 = &found
	return found
}

// scanInlineType3 walks the raw parse for a Type 3 font dictionary written inside another object.
//
// **A bound reached is `true`, not `false`.** Its one reader turns a `true` into `contentErr`, so returning
// `false` because nib stopped looking would turn "nib did not look" into "there is none" — the collapse
// `hasInlineType3Font` refuses one function up, where a file that will not parse raw answers true.
func (d *Document) scanInlineType3() bool {
	ctx, err := api.ReadContext(bytes.NewReader(d.raw), model.NewDefaultConfiguration())
	if err != nil {
		return true
	}
	rd := &Document{Ctx: ctx} // the typed-value door over the raw parse
	var walk func(o types.Object, nested bool, depth int) bool
	walk = func(o types.Object, nested bool, depth int) bool {
		if depth > maxWalkDepth {
			return false
		}
		switch v := o.(type) {
		case types.Dict:
			if nested {
				if rd.name(v["Subtype"]) == "Type3" {
					return true
				}
			}
			for _, x := range v {
				if walk(x, true, depth+1) {
					return true
				}
			}
		case types.StreamDict:
			return walk(v.Dict, false, depth+1)
		case types.Array:
			for _, x := range v {
				if walk(x, true, depth+1) {
					return true
				}
			}
		}
		return false
	}
	for nr := range ctx.XRefTable.Table {
		o, err := ctx.Dereference(types.IndirectRef{ObjectNumber: types.Integer(nr)})
		if err == nil && walk(o, false, 0) {
			return true
		}
	}
	return false
}
