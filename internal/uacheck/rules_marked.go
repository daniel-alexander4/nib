package uacheck

import "fmt"

// The marked-content rules — `PLAN-ua-coverage.md` P04.S04.
//
// Five clauses share one subject, veraPDF's `SEMarkedContent`: every BALANCED `BMC` or `BDC` sequence in page
// content, and in the form XObjects page content draws. `content.go` builds the population; this file is only
// the five predicates over it.
//
// # The one thing to know before changing anything here
//
// **The three inheritances differ, and they were measured rather than assumed.** `parentsTags` includes the
// sequence's OWN tag and crosses a form XObject boundary; `isTaggedContent`'s struct parent crosses it too;
// `inheritedLang` does NOT — its chain stops at the stream it was opened in. A change that makes any two of
// them share a walk will pass documents veraPDF fails, in one direction or the other.

func init() {
	register(Rule{
		Clause:  "7.1 t1",
		Summary: "content marked as Artifact shall not be present inside tagged content",
		Check:   checkArtifactNotInsideTaggedContent,
	})
	register(Rule{
		Clause:  "7.1 t2",
		Summary: "tagged content shall not be present inside content marked as Artifact",
		Check:   checkTaggedContentNotInsideArtifact,
	})
	for _, row := range spanAlternates {
		register(Rule{
			Clause:  row.clause,
			Summary: fmt.Sprintf("natural language for text in the %s attribute in Span marked content shall be determined", row.key),
			Check:   func(d *Document) Result { return checkSpanAlternateLanguage(d, row) },
		})
	}
}

// spanAlternate is one of the three keys 7.2 t30, t31 and t32 ask about. They differ in the key and in
// nothing else, so they are one predicate over a table rather than three copies (ADR-009).
type spanAlternate struct {
	clause string
	key    string
	// carries reads the presence of this row's key off a subject. A field selector rather than a map
	// lookup, so a fourth row cannot be added without saying where its answer comes from.
	carries func(mcSubject) bool
}

var spanAlternates = []spanAlternate{
	{clause: "7.2 t30", key: "ActualText", carries: func(s mcSubject) bool { return s.actualText }},
	{clause: "7.2 t31", key: "Alt", carries: func(s mcSubject) bool { return s.alt }},
	{clause: "7.2 t32", key: "E", carries: func(s mcSubject) bool { return s.expansion }},
}

// markedContentSubjects is the population all five rules run over, with why it may be short.
//
// **A content walk that stopped is `CannotCheck` for every one of them, never a Pass** (law 4): the sequences
// past the stop were never read, and a population that is missing its failures scores perfectly.
func markedContentSubjects(d *Document) ([]mcSubject, string) {
	_, why := d.contentEvents()
	return d.mcSubjects, why
}

// checkArtifactNotInsideTaggedContent evaluates ua1 7.1 t1 — `tag != 'Artifact' || isTaggedContent == false`.
func checkArtifactNotInsideTaggedContent(d *Document) Result {
	subjects, why := markedContentSubjects(d)
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	if len(subjects) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no marked-content sequences"}
	}
	for _, s := range subjects {
		if s.tag != "Artifact" {
			continue
		}
		if s.taggedUnread != "" {
			return Result{Verdict: CannotCheck, Why: s.taggedUnread, Where: s.where}
		}
		if s.tagged {
			return Result{
				Verdict: Fail,
				Why: "an /Artifact sequence is open inside tagged content, so content a reader is told to " +
					"ignore sits inside content the structure tree describes",
				Where: s.where,
			}
		}
	}
	return Result{Verdict: Pass}
}

// checkTaggedContentNotInsideArtifact evaluates ua1 7.1 t2 — `isTaggedContent == false ||
// parentsTags.contains('Artifact') == false`.
//
// **`parentsTags` includes the sequence's OWN tag** (`GFOpMarkedContent.java:142-151`), so an `/Artifact` that
// is itself inside tagged content breaks this clause as well as 7.1 t1 — measured, veraPDF reports one failure
// under each clause for that single sequence. A reading that looked only at enclosing tags would report t1's
// failure and miss t2's on the same document.
func checkTaggedContentNotInsideArtifact(d *Document) Result {
	subjects, why := markedContentSubjects(d)
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	if len(subjects) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no marked-content sequences"}
	}
	for _, s := range subjects {
		if !s.insideArtifact {
			continue
		}
		if s.taggedUnread != "" {
			return Result{Verdict: CannotCheck, Why: s.taggedUnread, Where: s.where}
		}
		if s.tagged {
			return Result{
				Verdict: Fail,
				Why: "this marked-content sequence is tagged content and is marked as an artifact or sits " +
					"inside one, so the structure tree describes content a reader is told to ignore",
				Where: s.where,
			}
		}
	}
	return Result{Verdict: Pass}
}

// checkSpanAlternateLanguage evaluates ua1 7.2 t30, t31 and t32 — `tag != 'Span' || KEY == null ||
// Lang != null || inheritedLang != null || gContainsCatalogLang == true`.
//
// **The catalog's `/Lang` settles every subject at once**, so it is asked FIRST — `catalogDeclaresLang` is
// P04.S01's one door for that question, and every other language clause asks it before reading anything.
//
// **The subject is every marked-content sequence, not every Span carrying the key** — veraPDF runs one check
// per `SEMarkedContent` and PASSES it where the tag is not Span or the key is absent. So a tagged document with
// no `/Alt` anywhere reports the clause passed WITH checks, and NotApplicable belongs only to a document with
// no marked content at all. Measured: the oracle reported "veraPDF passed, nib not applicable" on fifteen
// documents before this, which is the same class as P04.S02's finding about `PDStructElem`.
func checkSpanAlternateLanguage(d *Document, row spanAlternate) Result {
	// **Asked FIRST, and the phase close is what caught it being asked third.** `gContainsCatalogLang` is a
	// disjunct of the predicate, so a catalog `/Lang` satisfies every check veraPDF runs and a refusal over a
	// walk nib could not finish would be a refusal over a question already answered. Asked after the walk,
	// this clause reported CannotCheck on a document with `/Lang (en)` and nine levels of nested forms while
	// 7.2 t21-t23, t24/t25, t33 and t34 all reported Pass on the same file — S02 and S03 wrote this order
	// deliberately and said why, and S04 diverged from it silently.
	if catalogDeclaresLang(d) {
		return Result{Verdict: Pass}
	}
	subjects, why := markedContentSubjects(d)
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why}
	}
	if len(subjects) == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no marked-content sequences"}
	}
	for _, s := range subjects {
		if s.tag != "Span" || !row.carries(s) || s.ownLang || s.inheritedLang {
			continue
		}
		if s.langUnread != "" {
			return Result{Verdict: CannotCheck, Why: s.langUnread, Where: s.where}
		}
		return Result{
			Verdict: Fail,
			Why: fmt.Sprintf("this Span's /%s has no language: the catalog declares no /Lang, the property list "+
				"declares none, and nothing it is nested in determines one", row.key),
			Where: s.where,
		}
	}
	return Result{Verdict: Pass}
}
