package uacheck

import (
	"fmt"
	"strings"
	"testing"
)

// The marked-content rules against what veraPDF 1.30.2 was MEASURED to do — `PLAN-ua-coverage.md` P04.S04.
//
// Every row below is a document veraPDF was run on before the rule was written; the slice's ledger holds the
// run. The fixtures are built rather than committed, for `corpus_test.go`'s stated reason — a checked-in
// binary is opaque in review.

// markedDoc assembles a one-page tagged document around a content stream.
//
// The structure is the corpus's own shape: a `/Document` element over one `/P` whose `/K [0]` claims MCID 0 on
// the page, reached through a `/ParentTree` with one row. Each option moves exactly one thing.
type markedDoc struct {
	content string
	// catalogLang, docLang and pLang are `/Lang` on the catalog, the `/Document` element and the `/P`
	// element. A nil pointer is "the key is absent"; an empty string is a present but empty `()`, which
	// veraPDF counts as declaring a language.
	catalogLang, docLang, pLang *string
	// parentTree overrides the `/Nums` array, for the two documents whose MCID resolves to nothing or to
	// an element detached from the root.
	parentTree string
	// extra adds or replaces objects, and pageRes the page's `/Resources`.
	extra   map[int]string
	pageRes string
	// rootLang puts a `/Lang` on the StructTreeRoot itself.
	rootLang string
}

func lang(s string) *string { return &s }

func (m markedDoc) build() []byte {
	langKey := func(p *string) string {
		if p == nil {
			return ""
		}
		return fmt.Sprintf(" /Lang (%s)", *p)
	}
	res := m.pageRes
	if res == "" {
		res = "<< /Font << /F1 5 0 R >> >>"
	}
	nums := m.parentTree
	if nums == "" {
		nums = "0 [9 0 R]"
	}
	root := "<< /Type /StructTreeRoot /K 7 0 R /ParentTree 8 0 R /ParentTreeNextKey 1"
	if m.rootLang != "" {
		root += " /Lang (" + m.rootLang + ")"
	}
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 6 0 R" + langKey(m.catalogLang) + " >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources " + res + " /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(m.content), m.content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: root + " >>",
		7: "<< /Type /StructElem /S /Document /P 6 0 R /K [9 0 R] /Pg 3 0 R" + langKey(m.docLang) + " >>",
		8: "<< /Nums [" + nums + "] >>",
		9: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0]" + langKey(m.pLang) + " >>",
	}
	for n, body := range m.extra {
		objs[n] = body
	}
	return buildPDF(objs)
}

const (
	markedText     = "BT /F1 12 Tf 72 700 Td (Hi) Tj ET"
	markedArtifact = "/Artifact << /Type /Pagination >> BDC"
	markedSpanAlt  = "/Span << /Alt (alt) >> BDC\n" + markedText + "\nEMC"
)

// TestArtifactNestingAgreesWithWhatVeraPDFMeasured — ua1 7.1 t1 and t2, every row run on veraPDF 1.30.2
// before the rules were written.
//
// **Both halves, and the near controls beside them.** A rule that failed whatever it was shown would score
// perfectly on the failing rows alone, and the three passing shapes here are each one edit away from a
// failing one.
func TestArtifactNestingAgreesWithWhatVeraPDFMeasured(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		t1, t2  Verdict
	}{
		// An /Artifact opened inside a sequence whose MCID reaches the structure tree root breaks BOTH
		// clauses on the one sequence: t1 because its tag is Artifact and it is tagged content, t2 because
		// `parentsTags` includes its OWN tag. Measured: veraPDF reports one failure under each.
		{"an artifact inside tagged content", "/P <</MCID 0>> BDC\n" + markedArtifact + "\n72 100 100 10 re f\nEMC\n" + markedText + "\nEMC", Fail, Fail},
		// The same nesting written with BMC, which has no property list and so can never carry an MCID.
		{"a BMC artifact inside tagged content", "/P <</MCID 0>> BDC\n/Artifact BMC\n72 100 100 10 re f\nEMC\n" + markedText + "\nEMC", Fail, Fail},
		// An /Artifact carrying its own MCID is tagged content by its own resolution, not an inherited one.
		{"an artifact carrying its own MCID", "/Artifact << /Type /Pagination /MCID 0 >> BDC\n" + markedText + "\nEMC", Fail, Fail},
		// Tagged content inside an artifact fails t2 only — the inner sequence's tag is not Artifact.
		{"tagged content inside an artifact", markedArtifact + "\n/P <</MCID 0>> BDC\n" + markedText + "\nEMC\nEMC", Pass, Fail},
		// CONTROL: the same two sequences as siblings rather than nested.
		{"an artifact beside tagged content", markedArtifact + "\n72 100 100 10 re f\nEMC\n/P <</MCID 0>> BDC\n" + markedText + "\nEMC", Pass, Pass},
		// CONTROL: an artifact inside an artifact is not tagged content at all.
		{"an artifact inside an artifact", markedArtifact + "\n" + markedArtifact + "\n72 100 100 10 re f\nEMC\nEMC\n/P <</MCID 0>> BDC\n" + markedText + "\nEMC", Pass, Pass},
		// CONTROL: the sequence is closed before the artifact opens.
		{"an artifact after the sequence closes", "/P <</MCID 0>> BDC\n" + markedText + "\nEMC\n" + markedArtifact + "\n72 100 100 10 re f\nEMC", Pass, Pass},
		// **A malformed `/Artifact BDC` has no tag where veraPDF looks for it** (`arguments[size-2]`), so it
		// breaks nothing. Three of this slice's first-round fixtures were this shape.
		{"an artifact BDC written with no property list", "/P <</MCID 0>> BDC\n/Artifact BDC\n72 100 100 10 re f\nEMC\n" + markedText + "\nEMC", Pass, Pass},
	} {
		pdf := markedDoc{content: tc.content}.build()
		if got := verdictOf(t, pdf, "7.1 t1"); got.Verdict != tc.t1 {
			t.Errorf("%s: 7.1 t1 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t1)
		}
		if got := verdictOf(t, pdf, "7.1 t2"); got.Verdict != tc.t2 {
			t.Errorf("%s: 7.1 t2 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.t2)
		}
	}
}

// TestSpanAlternateLanguageAgreesWithWhatVeraPDFMeasured — ua1 7.2 t30, t31 and t32.
//
// The rows move the language one place at a time, because the clause's whole content is WHERE a language may
// come from: the Span's own property list, the element its enclosing MCID names, that element's ancestors, an
// enclosing sequence's own `/Lang`, or the catalog.
func TestSpanAlternateLanguageAgreesWithWhatVeraPDFMeasured(t *testing.T) {
	spanIn := func(inner string) string { return "/P <</MCID 0>> BDC\n" + inner + "\nEMC" }
	for _, tc := range []struct {
		name string
		doc  markedDoc
		want Verdict
	}{
		{"nothing declares a language", markedDoc{content: spanIn(markedSpanAlt)}, Fail},
		{"the Span's own property list", markedDoc{content: spanIn("/Span << /Alt (alt) /Lang (en-US) >> BDC\n" + markedText + "\nEMC")}, Pass},
		{"the element the enclosing MCID names", markedDoc{content: spanIn(markedSpanAlt), pLang: lang("en-US")}, Pass},
		{"an ancestor of that element", markedDoc{content: spanIn(markedSpanAlt), docLang: lang("en-US")}, Pass},
		{"the StructTreeRoot itself", markedDoc{content: spanIn(markedSpanAlt), rootLang: "en-US"}, Pass},
		{"the catalog", markedDoc{content: spanIn(markedSpanAlt), catalogLang: lang("en-US")}, Pass},
		// An EMPTY `()` declares a language for this clause — it is 7.2 t29 that fails it, not this one.
		{"an empty language on the element", markedDoc{content: spanIn(markedSpanAlt), pLang: lang("")}, Pass},
		{"an empty language in the catalog", markedDoc{content: spanIn(markedSpanAlt), catalogLang: lang("")}, Pass},
		// The enclosing sequence's OWN /Lang, with and without an MCID on it.
		{"the enclosing sequence's own /Lang", markedDoc{content: "/P << /MCID 0 /Lang (en-US) >> BDC\n" + markedSpanAlt + "\nEMC"}, Pass},
		{"an enclosing sequence with no MCID", markedDoc{content: "/P << /Lang (en-US) >> BDC\n" + markedSpanAlt + "\nEMC\n/P <</MCID 0>> BDC\n" + markedText + "\nEMC"}, Pass},
		// The language reaches THROUGH an /Artifact into the element the MCID outside it names.
		{"through an artifact", markedDoc{content: "/P <</MCID 0>> BDC\n" + markedArtifact + "\n" + markedSpanAlt + "\nEMC\nEMC", pLang: lang("en-US")}, Pass},
		// A Span outside every sequence has nothing to inherit from.
		{"a Span at the top level", markedDoc{content: markedSpanAlt + "\n/P <</MCID 0>> BDC\n" + markedText + "\nEMC", pLang: lang("en-US")}, Fail},
		// A /P chain that cycles is a complete answer, not a refusal: every ancestor was seen and none
		// carried a language. Measured — veraPDF fails it.
		{"a /P chain that cycles", markedDoc{content: spanIn(markedSpanAlt),
			extra: map[int]string{7: "<< /Type /StructElem /S /Document /P 9 0 R /K [9 0 R] /Pg 3 0 R >>"}}, Fail},
	} {
		pdf := tc.doc.build()
		if got := verdictOf(t, pdf, "7.2 t31"); got.Verdict != tc.want {
			t.Errorf("%s: 7.2 t31 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.want)
		}
	}
}

// TestTheThreeKeysAreOneRuleOverThreeRows holds t30, t31 and t32 apart: each fires on ITS key and on no other.
//
// An implementation that read one field for all three — or that asked "does the property list carry any
// alternate" — would pass a per-key test written only for `Alt`, and would report the same failure under
// three clauses, which is the defect 6.2 t1's comment names.
func TestTheThreeKeysAreOneRuleOverThreeRows(t *testing.T) {
	// The stimulus floor this package uses everywhere: both fixture and expectation come from the production
	// table, so an emptied table would make every loop below vacuous and this test green over nothing.
	if len(spanAlternates) != 3 {
		t.Fatalf("spanAlternates holds %d row(s), want the three keys 7.2 t30, t31 and t32", len(spanAlternates))
	}
	for _, row := range spanAlternates {
		pdf := markedDoc{content: fmt.Sprintf("/P <</MCID 0>> BDC\n/Span << /%s (x) >> BDC\n%s\nEMC\nEMC", row.key, markedText)}.build()
		for _, other := range spanAlternates {
			got := verdictOf(t, pdf, other.clause)
			// The other two PASS rather than being NotApplicable: their subject is the sequence, which is
			// present, and veraPDF passes a check whose key is absent.
			want := Pass
			if other.clause == row.clause {
				want = Fail
			}
			if got.Verdict != want {
				t.Errorf("with only /%s present, %s = %v (%s), want %v", row.key, other.clause, got.Verdict, got.Why, want)
			}
		}
	}
}

// TestInheritedLanguageDoesNotCrossAFormBoundaryWhileTheArtifactDoes is the slice's central finding, asserted
// as the asymmetry it is — and with each half's control, since a rule that ignored forms entirely would pass
// the first half alone.
//
// Measured on veraPDF 1.30.2: a `/Lang` in force on the page does NOT reach a Span inside a form the page
// draws (7.2 t31 fails), in both the marked-content and the structure-element spelling — while an `/Artifact`
// opened inside that form IS tagged content, because the struct parent does cross (7.1 t1 and t2 fail).
func TestInheritedLanguageDoesNotCrossAFormBoundaryWhileTheArtifactDoes(t *testing.T) {
	form := func(body string) map[int]string {
		return map[int]string{10: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(body), body)}
	}
	res := "<< /Font << /F1 5 0 R >> /XObject << /Fm0 10 0 R >> >>"

	// The language does not cross, stated twice — once for each spelling of "in force on the page".
	for _, tc := range []struct {
		name string
		doc  markedDoc
	}{
		{"the enclosing sequence's own /Lang", markedDoc{content: "/P << /MCID 0 /Lang (en-US) >> BDC\n" + markedText + "\n/Fm0 Do\nEMC", pageRes: res, extra: form(markedSpanAlt)}},
		{"the element the enclosing MCID names", markedDoc{content: "/P << /MCID 0 >> BDC\n" + markedText + "\n/Fm0 Do\nEMC", pLang: lang("en-US"), pageRes: res, extra: form(markedSpanAlt)}},
	} {
		if got := verdictOf(t, tc.doc.build(), "7.2 t31"); got.Verdict != Fail {
			t.Errorf("%s: a Span inside a form reports 7.2 t31 = %v (%s), want Fail — the language chain stops at "+
				"the stream boundary", tc.name, got.Verdict, got.Why)
		}
	}
	// CONTROL: the same Span in the same form, with the language on the form's OWN enclosing sequence.
	inForm := "/P << /Lang (en-US) >> BDC\n" + markedSpanAlt + "\nEMC"
	ctrl := markedDoc{content: "/P << /MCID 0 >> BDC\n" + markedText + "\n/Fm0 Do\nEMC", pageRes: res, extra: form(inForm)}
	if got := verdictOf(t, ctrl.build(), "7.2 t31"); got.Verdict != Pass {
		t.Fatalf("control: with the language inside the form, 7.2 t31 = %v (%s), want Pass — if this fails, the "+
			"rows above prove nothing about the boundary", got.Verdict, got.Why)
	}
	// The artifact DOES cross: the struct parent reaches into the form.
	across := markedDoc{content: "/P << /MCID 0 >> BDC\n" + markedText + "\n/Fm0 Do\nEMC", pageRes: res,
		extra: form(markedArtifact + "\n72 0 10 10 re f\nEMC")}
	for _, clause := range []string{"7.1 t1", "7.1 t2"} {
		if got := verdictOf(t, across.build(), clause); got.Verdict != Fail {
			t.Errorf("an /Artifact inside a form drawn from tagged content reports %s = %v (%s), want Fail — the "+
				"struct parent crosses the boundary the language does not", clause, got.Verdict, got.Why)
		}
	}
}

// TestAnMCIDIsNotTaggedContentUntilItReachesTheRoot is the first two false passes, as gap-down tests.
//
// Both were nib `Pass` where veraPDF fails, which `Verdict.conformant` treats as licence to call the document
// conformant. The cause was one predicate: `covered` counted any `/MCID n`, while `isTaggedContent` resolves
// the element and climbs `/P` to the structure tree root.
func TestAnMCIDIsNotTaggedContentUntilItReachesTheRoot(t *testing.T) {
	body := "/P <</MCID %s>> BDC\n" + markedText + "\n" + markedArtifact + "\n72 100 100 10 re f\nEMC\nEMC"
	for _, tc := range []struct {
		name string
		doc  markedDoc
	}{
		// The parent tree's row for this page holds one slot, so MCID 7 names nothing.
		{"an MCID the parent tree does not hold", markedDoc{content: fmt.Sprintf(body, "7")}},
		// The element resolves and is attached to nothing: no /P chain, so it never reaches the root.
		{"an element detached from the structure tree", markedDoc{content: fmt.Sprintf(body, "0"),
			parentTree: "0 [10 0 R]", extra: map[int]string{10: "<< /Type /StructElem /S /P /Pg 3 0 R /K [0] >>"}}},
	} {
		pdf := tc.doc.build()
		if got := verdictOf(t, pdf, "7.1 t3"); got.Verdict != Fail {
			t.Errorf("%s: 7.1 t3 = %v (%s), want Fail — veraPDF fails it and a Pass here lets a non-conformant "+
				"document be called conformant", tc.name, got.Verdict, got.Why)
		}
		// And the artifact inside it is NOT tagged content, so 7.1 t1 must not fire either.
		if got := verdictOf(t, pdf, "7.1 t1"); got.Verdict != Pass {
			t.Errorf("%s: 7.1 t1 = %v (%s), want Pass — the enclosing sequence is not tagged content", tc.name, got.Verdict, got.Why)
		}
	}
	// CONTROL: the same document with an MCID that does reach the root passes t3 and fails t1.
	ok := markedDoc{content: fmt.Sprintf(body, "0")}.build()
	if got := verdictOf(t, ok, "7.1 t3"); got.Verdict != Pass {
		t.Fatalf("control: with the MCID resolving to a tree-attached element, 7.1 t3 = %v (%s), want Pass", got.Verdict, got.Why)
	}
	if got := verdictOf(t, ok, "7.1 t1"); got.Verdict != Fail {
		t.Fatalf("control: 7.1 t1 = %v (%s), want Fail", got.Verdict, got.Why)
	}
}

// TestTextInsideAnArtifactStillNeedsALanguage is the other two false passes.
//
// nib exempted every drawing operator inside an `/Artifact` from 7.2 t34. veraPDF's test is
// `gContainsCatalogLang == true || Lang != null` and carries no artifact disjunct — measured, in page content
// and across a form boundary. The language still reaches through the artifact, which is the second half here:
// an exemption and an inheritance look the same on a document where both would pass.
func TestTextInsideAnArtifactStillNeedsALanguage(t *testing.T) {
	res := "<< /Font << /F1 5 0 R >> /XObject << /Fm0 10 0 R >> >>"
	inForm := markedArtifact + "\n" + markedText + "\nEMC"
	form := map[int]string{10: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(inForm), inForm)}
	for _, tc := range []struct {
		name string
		doc  markedDoc
		want Verdict
	}{
		{"artifact text beside tagged text", markedDoc{content: "/P <</MCID 0>> BDC\n" + markedText + "\nEMC\n" + markedArtifact + "\n" + markedText + "\nEMC", pLang: lang("en-US")}, Fail},
		{"artifact text inside a form", markedDoc{content: "/P <</MCID 0>> BDC\n" + markedText + "\nEMC\n/Fm0 Do", pLang: lang("en-US"), pageRes: res, extra: form}, Fail},
		// The language DOES reach through an artifact nested inside the sequence whose element declares one.
		{"artifact text nested inside the tagged sequence", markedDoc{content: "/P <</MCID 0>> BDC\n" + markedText + "\n" + markedArtifact + "\n" + markedText + "\nEMC\nEMC", pLang: lang("en-US")}, Pass},
		// CONTROL: the same nesting with no language anywhere fails, so the row above is an inheritance
		// rather than an exemption wearing its clothes.
		{"the same nesting with no language at all", markedDoc{content: "/P <</MCID 0>> BDC\n" + markedText + "\n" + markedArtifact + "\n" + markedText + "\nEMC\nEMC"}, Fail},
	} {
		if got := verdictOf(t, tc.doc.build(), "7.2 t34"); got.Verdict != tc.want {
			t.Errorf("%s: 7.2 t34 = %v (%s), want %v", tc.name, got.Verdict, got.Why, tc.want)
		}
	}
}

// TestALangOnlyStreamIsWalkedOncePerSTREAMNotOncePerUSE — the cost property, and it is a correctness
// property wearing cost's clothes.
//
// `enterType3` fires on every text-showing operator and `enterPattern` on every `scn`, and each entry spends a
// `maxFormWalks` unit. Without a per-stream memo a page that shows `maxFormWalks + 1` glyphs in ONE Type 3
// font — an ordinary page of text, in a font nib is obliged to read — trips the budget, which sets
// `contentErr`, which turns **every** content rule into CannotCheck. veraPDF reads a pattern and a font once,
// as objects, so repeating is not faithful either.
//
// Probed red: with the memo removed, both rows report CannotCheck naming the budget.
func TestALangOnlyStreamIsWalkedOncePerStreamNotOncePerUse(t *testing.T) {
	uses := maxFormWalks + 10
	type3 := "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.1 0 0 0.1 0 0] " +
		"/CharProcs << /a 11 0 R >> /Encoding << /Type /Encoding /Differences [97 /a] >> " +
		"/FirstChar 97 /LastChar 97 /Widths [10] /Resources << >> >>"
	glyph := "<< /Length 21 >>\nstream\n10 0 0 0 10 10 d0\n\nendstream"
	tiling := "<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] /XStep 10 " +
		"/YStep 10 /Resources << >> /Length 15 >>\nstream\n0 0 5 5 re f\n\nendstream"

	for _, tc := range []struct {
		name    string
		content string
		res     string
		extra   map[int]string
	}{
		{"one Type 3 font, every glyph shown again and again",
			"/P <</MCID 0>> BDC\nBT /T3 12 Tf 72 700 Td\n" + strings.Repeat("(a) Tj\n", uses) + "ET\nEMC",
			"<< /Font << /F1 5 0 R /T3 10 0 R >> >>", map[int]string{10: type3, 11: glyph}},
		{"one tiling pattern, selected again and again",
			"/P <</MCID 0>> BDC\n/Pattern cs\n" + strings.Repeat("/P0 scn 0 0 5 5 re f\n", uses) + "EMC",
			"<< /Font << /F1 5 0 R >> /Pattern << /P0 10 0 R >> >>", map[int]string{10: tiling}},
	} {
		pdf := markedDoc{content: tc.content, pageRes: tc.res, extra: tc.extra}.build()
		for _, clause := range []string{"7.1 t3", "7.2 t29", "7.1 t1"} {
			if got := verdictOf(t, pdf, clause); got.Verdict == CannotCheck {
				t.Errorf("%s: %s reports CannotCheck (%s) — %d uses of ONE stream spent the walk's budget, so an "+
					"ordinary page turns every content rule into a refusal", tc.name, clause, got.Why, uses)
			}
		}
	}
}

// TestTheContentLanguageStopsAtTheStreamBoundary — 7.2 t34's half of the slice's central finding.
//
// `TestInheritedLanguageDoesNotCrossAFormBoundaryWhileTheArtifactDoes` asserts it for a marked-content
// SEQUENCE; this asserts it for a content ITEM, which is a different rule reading a different field and was a
// separate false pass. Measured on veraPDF 1.30.2: text inside a form drawn from a sequence the page gave a
// language FAILS 7.2 t34, in both spellings of "in force on the page", while nib passed it.
//
// **The boundary is the WALKER's stream, not the innermost frame's**, and that is what the second row pins: a
// form whose own content opens no sequence has a stack of inherited frames only, every one carrying the
// invoking stream's number.
func TestTheContentLanguageStopsAtTheStreamBoundary(t *testing.T) {
	form := func(body string) map[int]string {
		return map[int]string{10: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources "+
			"<< /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(body), body)}
	}
	res := "<< /Font << /F1 5 0 R >> /XObject << /Fm0 10 0 R >> >>"
	for _, tc := range []struct {
		name string
		doc  markedDoc
	}{
		// The form opens a sequence of its own, so the stack has a frame in the form's stream.
		{"the page sequence's own /Lang", markedDoc{content: "/P << /MCID 0 /Lang (en-US) >> BDC\n" + markedText + "\n/Fm0 Do\nEMC",
			pageRes: res, extra: form("/Span << /Alt (alt) >> BDC\n" + markedText + "\nEMC")}},
		{"the element the page sequence's MCID names", markedDoc{content: "/P << /MCID 0 >> BDC\n" + markedText + "\n/Fm0 Do\nEMC",
			pLang: lang("en-US"), pageRes: res, extra: form("/Span << /Alt (alt) >> BDC\n" + markedText + "\nEMC")}},
		// The form opens NOTHING: every frame on the stack belongs to the page's stream.
		{"a form that opens no sequence at all", markedDoc{content: "/P << /MCID 0 /Lang (en-US) >> BDC\n" + markedText + "\n/Fm0 Do\nEMC",
			pageRes: res, extra: form(markedText)}},
	} {
		if got := verdictOf(t, tc.doc.build(), "7.2 t34"); got.Verdict != Fail {
			t.Errorf("%s: text inside the form reports 7.2 t34 = %v (%s), want Fail — the language chain stops "+
				"at the stream it was opened in", tc.name, got.Verdict, got.Why)
		}
	}
	// CONTROL: the language inside the form determines it, or every row above is a rule that fails whatever
	// text it is shown inside a form.
	ctrl := markedDoc{content: "/P << /MCID 0 >> BDC\n" + markedText + "\n/Fm0 Do\nEMC", pageRes: res,
		extra: form("/P << /Lang (en-US) >> BDC\n" + markedText + "\nEMC"), pLang: lang("en-US")}
	if got := verdictOf(t, ctrl.build(), "7.2 t34"); got.Verdict != Pass {
		t.Fatalf("control: with the language declared inside the form, 7.2 t34 = %v (%s), want Pass", got.Verdict, got.Why)
	}
}

// nestedLeakDoc is a page whose NESTED stream — a drawn tiling pattern, a shown Type 3 glyph, or a form one of
// those draws — holds everything the five rules and 7.1 t3 look for: an `/Artifact` inside a tagged sequence,
// an unlanguaged Span carrying `/Alt`, an uncovered paint and untagged text.
//
// The page itself breaks nothing: its text sits in a sequence whose element declares a language. So every
// verdict below is `Pass` if and only if the nested stream contributed no subject and no content item.
func nestedLeakDoc(mode string) markedDoc {
	// The `/Quote` sequence carries a VALID `/Lang` and exists only to be counted: 7.2 t29's reader is what
	// proves the nested stream was entered at all, and without an entry to prove, every assertion below
	// would pass over a walk that had stopped entering nested streams entirely.
	// **An inline image, an image XObject and an unresolvable one are in here deliberately.** Their appends
	// did not go through the `langOnly` check — it lived in the text-and-paint branch alone — so each of them
	// reached the content events from inside a pattern or a glyph procedure, with the empty stack
	// `enterLangOnly` passes, i.e. as UNCOVERED content. An inline image is the canonical Type 3 bitmap
	// glyph, so that was a false FAIL of 7.1 t3 on an ordinary tagged document.
	leak := "/Quote << /Lang (en-GB) >> BDC\n" + markedText + "\nEMC\n" +
		"/P << /MCID 0 >> BDC\n" + markedArtifact + "\n72 0 10 10 re f\nEMC\n" +
		"/Span << /Alt (alt) >> BDC\n" + markedText + "\nEMC\nEMC\n0 0 5 5 re f\n" + markedText +
		"\nBI /W 1 /H 1 /BPC 8 /CS /G ID \x00 EI\n/NoSuchXObject Do"
	st := func(dict, body string) string {
		return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(body), body)
	}
	fontRes := "/Resources << /Font << /F1 5 0 R >> >>"
	switch mode {
	case "pattern":
		return markedDoc{
			content: "/P <</MCID 0>> BDC\n" + markedText + "\n/Pattern cs /P0 scn 0 0 100 100 re f\nEMC",
			pLang:   lang("en-US"),
			pageRes: "<< /Font << /F1 5 0 R >> /Pattern << /P0 10 0 R >> >>",
			extra: map[int]string{10: st("/Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 "+
				"/BBox [0 0 10 10] /XStep 10 /YStep 10 "+fontRes, leak)}}
	case "type3":
		return markedDoc{
			content: "/P <</MCID 0>> BDC\n" + markedText + "\nBT /T3 12 Tf 72 500 Td (a) Tj ET\nEMC",
			pLang:   lang("en-US"),
			pageRes: "<< /Font << /F1 5 0 R /T3 10 0 R >> >>",
			extra: map[int]string{
				10: "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.1 0 0 0.1 0 0] " +
					"/CharProcs 11 0 R /Encoding << /Type /Encoding /Differences [97 /a] >> /FirstChar 97 " +
					"/LastChar 97 /Widths [10] " + fontRes + " >>",
				11: "<< /a 12 0 R >>",
				12: st("", "10 0 0 0 10 10 d0\n"+leak)}}
	default: // "form-in-pattern": the form is reached only from inside the pattern
		return markedDoc{
			content: "/P <</MCID 0>> BDC\n" + markedText + "\n/Pattern cs /P0 scn 0 0 100 100 re f\nEMC",
			pLang:   lang("en-US"),
			pageRes: "<< /Font << /F1 5 0 R >> /Pattern << /P0 10 0 R >> >>",
			extra: map[int]string{
				10: st("/Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 10 10] "+
					"/XStep 10 /YStep 10 /Resources << /XObject << /Fx 11 0 R >> >>", "/Fx Do"),
				11: st("/Type /XObject /Subtype /Form /BBox [0 0 10 10] "+fontRes, leak)}}
	}
}

// TestADrawnPatternAndGlyphContributeNoSubjectAndNoEvent asserts the property DIRECTLY rather than through
// verdicts, because the verdicts could not see it.
//
// veraPDF builds a PLAIN content stream for a tiling pattern and a Type 3 glyph procedure — `SEMarkedContent`
// is created at two sites only, the page and a form XObject drawn from a semantic stream — so nothing in
// either is a marked-content subject or a content item. nib walks them anyway, for 7.2 t29's `/Lang` values
// and nothing else.
//
// **The counted stimulus is the whole point.** An earlier version of this test read four clause verdicts over
// `patternDoc`, and three of the four could not fail on that fixture for reasons unrelated to leaking: its
// catalog declares a `/Lang`, so 7.2 t34 short-circuits to Pass before reading an event, and it contains no
// `/Artifact` at all, so 7.1 t1 and t2 have nothing to fire on. A leak that was Artifact-tagged or
// language-bearing would have gone undetected. Counting the populations cannot be dodged that way, and the
// rising `/Lang` count proves the stream WAS entered — without it, a walk that stopped entering nested
// streams entirely would pass this test perfectly.
func TestADrawnPatternAndGlyphContributeNoSubjectAndNoEvent(t *testing.T) {
	for _, mode := range []string{"pattern", "type3", "form-in-pattern"} {
		doc := nestedLeakDoc(mode)
		withLeak, err := open(doc.build())
		if err != nil {
			t.Fatalf("%s: the fixture did not open: %v", mode, err)
		}
		events, why := withLeak.contentEvents()
		if why != "" {
			t.Fatalf("%s: the content walk did not finish: %s", mode, why)
		}
		// The same page with the nested stream's own content emptied: the baseline populations.
		bare := doc
		bare.extra = map[int]string{}
		for k, v := range doc.extra {
			bare.extra[k] = v
		}
		plain, err := open(markedDoc{content: doc.content, pLang: doc.pLang, pageRes: doc.pageRes, extra: doc.extra}.build())
		if err != nil {
			t.Fatalf("%s: the baseline did not open: %v", mode, err)
		}
		_, _ = plain.contentEvents()

		// STIMULUS: the nested stream really was entered, or every assertion below is vacuous.
		if withLeak.mcLangCount == 0 && mode != "form-in-pattern" {
			t.Errorf("%s: the walk recorded no marked-content /Lang from the nested stream, so it was never "+
				"entered and this test asserts nothing", mode)
		}
		// RESPONSE: and it contributed neither a subject nor a drawing event.
		for _, ev := range events {
			if strings.Contains(ev.where, "tiling pattern") || strings.Contains(ev.where, "Type 3 font") {
				t.Errorf("%s: a drawing operator inside a nested stream reached the content events at %q — "+
					"veraPDF makes no content item there", mode, ev.where)
			}
		}
		for _, s := range withLeak.mcSubjects {
			if strings.Contains(s.where, "tiling pattern") || strings.Contains(s.where, "Type 3 font") {
				t.Errorf("%s: a marked-content sequence inside a nested stream became a subject at %q — "+
					"veraPDF makes no SEMarkedContent there", mode, s.where)
			}
		}
		// And the verdicts agree, which is what a user sees. 7.1 t3 is the one that is genuinely live on
		// this fixture; the others are here because the fixture was built so that they CAN fail.
		for _, clause := range []string{"7.1 t3", "7.1 t1", "7.1 t2", "7.2 t31", "7.2 t34"} {
			if got := verdictOf(t, doc.build(), clause); got.Verdict == Fail {
				t.Errorf("%s: %s reports Fail (%s at %s) over content veraPDF does not read there",
					mode, clause, got.Why, got.Where)
			}
		}
	}
}

// TestTheMarkedContentRulesAreCannotCheckWhenTheTreeRunsOut — the five clauses' `CannotCheck` branches,
// which nothing reached.
//
// **They were unreachable by the whole suite and the exemption is why.** `notTreeRules` takes these five out
// of `TestEveryTreeRuleIsCannotCheckPastTheTreeBound` — correctly, because their subject is a marked-content
// sequence and a deep-Div document that draws nothing has no subject — and the exemption's own comment
// claimed the bound was covered "with its own test", which did not exist. So: make a document that HAS the
// subject and whose tree nib cannot finish reading, and require the refusal.
//
// Each half needs its own shape, because the two questions stop on different things. `isTaggedContent` stops
// on a **parent tree** nib could not read to the slot; `inheritedLang` stops on a `/P` **climb** past its
// bound. And each is reached only through its clause's own guard — 7.1 t1 needs an `/Artifact` tag, t30-t32
// need a Span carrying the key — so a fixture without those measures nothing.
func TestTheMarkedContentRulesAreCannotCheckWhenTheTreeRunsOut(t *testing.T) {
	// A parent tree whose one row sits under seventy levels of /Kids: the MCID resolves to nothing nib read.
	unreadTree := map[int]string{8: "<< /Kids [300 0 R] /Limits [0 0] >>"}
	for i := 0; i < 70; i++ {
		unreadTree[300+i] = fmt.Sprintf("<< /Kids [%d 0 R] /Limits [0 0] >>", 301+i)
	}
	unreadTree[370] = "<< /Nums [0 [9 0 R]] /Limits [0 0] >>"

	// A /P chain seventy links long with no /Lang on it: the climb runs out before it can answer.
	longClimb := map[int]string{9: "<< /Type /StructElem /S /P /P 400 0 R /Pg 3 0 R /K [0] >>"}
	for i := 0; i < 70; i++ {
		longClimb[400+i] = fmt.Sprintf("<< /Type /StructElem /S /Div /P %d 0 R >>", 401+i)
	}
	longClimb[470] = "<< /Type /StructElem /S /Document /P 6 0 R /K [9 0 R] >>"

	artifactInTagged := "/P <</MCID 0>> BDC\n" + markedArtifact + "\n72 0 10 10 re f\nEMC\n" + markedText + "\nEMC"
	spanInTagged := "/P <</MCID 0>> BDC\n" + markedSpanAlt + "\nEMC"
	// Every alternate at once, so all three clauses have a subject — a Span carrying only `/Alt` reaches
	// t31's guard and skips t30's and t32's, which is the rule behaving correctly and the fixture measuring
	// one clause while claiming three.
	spanAllKeys := "/P <</MCID 0>> BDC\n/Span << /ActualText (a) /Alt (b) /E (c) >> BDC\n" + markedText + "\nEMC\nEMC"

	for _, tc := range []struct {
		name    string
		doc     markedDoc
		clauses []string
	}{
		// `isTaggedContent` cannot be settled: 7.1 t1 and t2 must refuse rather than pass the artifact.
		{"a parent tree nib could not read to the slot",
			markedDoc{content: artifactInTagged, extra: unreadTree}, []string{"7.1 t1", "7.1 t2"}},
		// `inheritedLang` cannot be settled: the Span's alternates must refuse rather than fail.
		{"a /P chain past the climb's bound",
			markedDoc{content: spanAllKeys, extra: longClimb}, []string{"7.2 t30", "7.2 t31", "7.2 t32"}},
		{"a parent tree nib could not read, under a Span",
			markedDoc{content: spanInTagged, extra: unreadTree}, []string{"7.2 t31"}},
	} {
		pdf := tc.doc.build()
		for _, clause := range tc.clauses {
			got := verdictOf(t, pdf, clause)
			if got.Verdict != CannotCheck {
				t.Errorf("%s: %s reports %v (%s), want CannotCheck — nib did not finish reading the tree, so it "+
					"cannot say whether the clause holds", tc.name, clause, got.Verdict, got.Why)
			}
		}
	}
	// CONTROLS, one per shape: the same documents with a tree nib CAN read reach a verdict. Without these,
	// a rule that answered CannotCheck unconditionally would score perfectly above.
	if got := verdictOf(t, markedDoc{content: artifactInTagged}.build(), "7.1 t1"); got.Verdict != Fail {
		t.Fatalf("control: with a readable parent tree, 7.1 t1 = %v (%s), want Fail", got.Verdict, got.Why)
	}
	if got := verdictOf(t, markedDoc{content: spanInTagged}.build(), "7.2 t31"); got.Verdict != Fail {
		t.Fatalf("control: with a readable tree, 7.2 t31 = %v (%s), want Fail", got.Verdict, got.Why)
	}
}

// TestAnAppearanceStreamIsNoMarkedContentSubject — the acceptance clause's other half, measured.
//
// `GFPDAnnot.java:465-473` builds the appearance as a NON-semantic form (`isRealContent=false`,
// `isAnnotation=true`), so `GFPDXForm`'s semantic branch is never taken and nothing in an appearance is an
// `SEMarkedContent`. Measured on 1.30.2: an `/Artifact` inside tagged content and an unlanguaged Span carrying
// `/Alt`, both inside a widget's appearance, leave 7.1 t1, t2, t3, 7.2 t31 and t34 all passing — veraPDF
// counts ONE sequence on that document, the page's.
//
// The appearance still yields drawing EVENTS, because the font rules read them; it is the subject population
// and 7.1 t3 that it stays out of. That split is `contentEvent.appearance`, and it predates this slice.
func TestAnAppearanceStreamIsNoMarkedContentSubject(t *testing.T) {
	inAppearance := "/P << /MCID 0 >> BDC\n" + markedArtifact + "\n0 0 5 5 re f\nEMC\n" + markedSpanAlt + "\nEMC"
	ap := fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Resources "+
		"<< /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(inAppearance), inAppearance)
	// The PAGE's own sequence carries the language, so 7.2 t34 passes on the page's text without giving the
	// appearance's Span anything to inherit — a `/Lang` on the Document ELEMENT would have reached the Span
	// too, and the stimulus below would then have had nothing to fail on.
	doc := markedDoc{
		content: "/P << /MCID 0 /Lang (en-US) >> BDC\n" + markedText + "\nEMC",
		extra: map[int]string{
			10: ap,
			11: "<< /Type /Annot /Subtype /Square /Rect [10 10 110 110] /F 4 /AP << /N 10 0 R >> >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> " +
				"/Contents 4 0 R /StructParents 0 /Annots [11 0 R] >>",
		},
	}
	pdf := doc.build()
	for _, clause := range []string{"7.1 t1", "7.1 t2", "7.1 t3", "7.2 t31", "7.2 t34"} {
		if got := verdictOf(t, pdf, clause); got.Verdict == Fail {
			t.Errorf("%s reports Fail (%s at %s) over marked content inside an annotation's appearance, which "+
				"veraPDF does not read as a subject", clause, got.Why, got.Where)
		}
	}
	// STIMULUS: the same constructs on the PAGE do fail, or the rows above pass because the fixture is inert.
	onPage := markedDoc{content: "/P <</MCID 0>> BDC\n" + markedArtifact + "\n0 0 5 5 re f\nEMC\n" +
		markedSpanAlt + "\nEMC"}.build()
	for _, clause := range []string{"7.1 t1", "7.1 t2", "7.2 t31"} {
		if got := verdictOf(t, onPage, clause); got.Verdict != Fail {
			t.Fatalf("stimulus: with the same constructs in page content, %s = %v (%s), want Fail — if this "+
				"passes, the appearance rows above assert nothing", clause, got.Verdict, got.Why)
		}
	}
}

// TestAStoppedContentWalkIsCannotCheckForAllFive — the acceptance clause, verbatim: "A content walk that
// stopped (P03's budgets, an unreadable stream) is CannotCheck for all five, never Pass."
//
// A population that is missing its failures scores perfectly, so a walk that stopped must refuse rather than
// report on the part it read. `formFanOut(7, false)` spends the nested-stream budget.
func TestAStoppedContentWalkIsCannotCheckForAllFive(t *testing.T) {
	// Control first: at two levels the walk finishes, and the clauses reach verdicts.
	for _, clause := range []string{"7.1 t1", "7.1 t2", "7.2 t30", "7.2 t31", "7.2 t32"} {
		if got := verdictOf(t, formFanOut(2, false), clause); got.Verdict == CannotCheck {
			t.Fatalf("control: at two levels %s already reports CannotCheck (%s), so the row below proves nothing",
				clause, got.Why)
		}
	}
	for _, clause := range []string{"7.1 t1", "7.1 t2", "7.2 t30", "7.2 t31", "7.2 t32"} {
		got := verdictOf(t, formFanOut(7, false), clause)
		if got.Verdict != CannotCheck || !strings.Contains(got.Why, "never read") {
			t.Errorf("with the content walk stopped at its budget, %s reports %v (%s), want CannotCheck naming "+
				"what was not read", clause, got.Verdict, got.Why)
		}
	}
}

// TestTheCatalogLanguageIsAskedBeforeTheWalk — the P04 close's cross-slice finding.
//
// `gContainsCatalogLang` is a disjunct of t30/t31/t32's predicate, so a catalog `/Lang` satisfies every check
// veraPDF runs and nothing the walk failed to read can change that. Asked AFTER the walk — which is how S04
// shipped it, while S02 and S03 asked it first and said why — these three refused on a document every sibling
// clause passed. The corpus felt it: two files moved from refused to settled, 293 → 295.
//
// 7.1 t1 and t2 carry no such disjunct, so they stay CannotCheck on the same document. That contrast is the
// test: a fix that simply stopped refusing would take them with it.
func TestTheCatalogLanguageIsAskedBeforeTheWalk(t *testing.T) {
	// Nine levels of form XObjects, one past `maxFormDepth`, so the walk stops with `contentErr` set.
	deep := map[int]string{}
	for i := 0; i <= 9; i++ {
		body := fmt.Sprintf("/Fm%d Do", i+1)
		res := fmt.Sprintf("/Resources << /XObject << /Fm%d %d 0 R >> >> ", i+1, 11+i)
		if i == 9 {
			body, res = "0 0 5 5 re f", ""
		}
		deep[10+i] = fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] %s/Length %d >>\nstream\n%s\nendstream",
			res, len(body), body)
	}
	content := "/P <</MCID 0>> BDC\n" + markedText + "\n/Fm0 Do\nEMC"
	pageRes := "<< /Font << /F1 5 0 R >> /XObject << /Fm0 10 0 R >> >>"

	withLang := markedDoc{content: content, pageRes: pageRes, extra: deep, catalogLang: lang("en-US")}.build()
	for _, clause := range []string{"7.2 t30", "7.2 t31", "7.2 t32"} {
		if got := verdictOf(t, withLang, clause); got.Verdict != Pass {
			t.Errorf("with a catalog /Lang and a walk that stopped, %s = %v (%s), want Pass — the catalog "+
				"satisfies the predicate for every subject, read or not", clause, got.Verdict, got.Why)
		}
	}
	// The contrast: the artifact clauses have no catalog disjunct and must still refuse.
	for _, clause := range []string{"7.1 t1", "7.1 t2"} {
		if got := verdictOf(t, withLang, clause); got.Verdict != CannotCheck {
			t.Errorf("with a walk that stopped, %s = %v (%s), want CannotCheck — no catalog disjunct applies "+
				"to it", clause, got.Verdict, got.Why)
		}
	}
	// STIMULUS: without the catalog /Lang the same document refuses, or the rows above pass because the walk
	// never actually stopped.
	noLang := markedDoc{content: content, pageRes: pageRes, extra: deep}.build()
	if got := verdictOf(t, noLang, "7.2 t31"); got.Verdict != CannotCheck {
		t.Fatalf("stimulus: with no catalog /Lang, 7.2 t31 = %v (%s), want CannotCheck — the fixture's walk "+
			"did not stop, so this test asserts nothing", got.Verdict, got.Why)
	}
}

// TestWhatTheLANGUAGEPopulationGradesAndWhatItMerelyACCEPTS — two asymmetries that read like defects and are
// veraPDF's own, each measured on 1.30.2 at the P04 close.
//
//   - **A `/Lang` on the StructTreeRoot is ACCEPTED by the climb and never GRADED by 7.2 t29.** A review read
//     that as one of the two clauses being wrong. It is not: measured, an invalid `(en_US)` on the root is not
//     a t29 subject at all (zero checks), while the same value on an ordinary element fails. The profile says
//     t29's subject is the catalog, a structure element, or a property list — and the root is none of those.
//   - **An appearance stream's BDC `/Lang` IS graded by 7.2 t29** although that sequence is no
//     `SEMarkedContent`. Measured: veraPDF fails t29 on it. nib already did both; what was missing was the
//     measurement, which is what this test is.
func TestWhatTheLanguagePopulationGradesAndWhatItMerelyAccepts(t *testing.T) {
	root := "<< /Type /StructTreeRoot /Lang (en_US) /K 7 0 R /ParentTree 8 0 R /ParentTreeNextKey 1 >>"
	onRoot := markedDoc{content: "/P <</MCID 0>> BDC\n" + markedSpanAlt + "\nEMC",
		extra: map[int]string{6: root}}.build()
	if got := verdictOf(t, onRoot, "7.2 t29"); got.Verdict != NotApplicable {
		t.Errorf("an invalid /Lang on the StructTreeRoot reports 7.2 t29 = %v (%s), want NotApplicable — it is "+
			"not one of t29's subjects, measured", got.Verdict, got.Why)
	}
	// ...and the climb still ACCEPTS it, which is the asymmetry.
	if got := verdictOf(t, onRoot, "7.2 t31"); got.Verdict != Pass {
		t.Errorf("the same root /Lang reports 7.2 t31 = %v (%s), want Pass — `parentLang` reads the root",
			got.Verdict, got.Why)
	}
	// The near control: the same value on an ordinary element IS graded.
	onElem := markedDoc{content: "/P <</MCID 0>> BDC\n" + markedText + "\nEMC", pLang: lang("en_US")}.build()
	if got := verdictOf(t, onElem, "7.2 t29"); got.Verdict != Fail {
		t.Fatalf("control: an invalid /Lang on an element reports 7.2 t29 = %v (%s), want Fail", got.Verdict, got.Why)
	}
	// An appearance stream's property list is graded, though it is no marked-content subject.
	apBody := "/Span << /Lang (en_US) >> BDC\n" + markedText + "\nEMC"
	ap := fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Resources << /Font << /F1 5 0 R "+
		">> >> /Length %d >>\nstream\n%s\nendstream", len(apBody), apBody)
	inAP := markedDoc{content: "/P <</MCID 0>> BDC\n" + markedText + "\nEMC", extra: map[int]string{
		10: ap,
		11: "<< /Type /Annot /Subtype /Square /Rect [10 10 110 110] /F 4 /AP << /N 10 0 R >> >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> " +
			"/Contents 4 0 R /StructParents 0 /Annots [11 0 R] >>",
	}}.build()
	if got := verdictOf(t, inAP, "7.2 t29"); got.Verdict != Fail {
		t.Errorf("an invalid /Lang on a BDC inside an appearance stream reports 7.2 t29 = %v (%s), want Fail — "+
			"veraPDF grades it there even though it makes no marked-content subject", got.Verdict, got.Why)
	}
	if got := verdictOf(t, inAP, "7.2 t31"); got.Verdict == Fail {
		t.Errorf("the same appearance sequence reports 7.2 t31 = Fail (%s), and it is no SEMarkedContent", got.Why)
	}
}
