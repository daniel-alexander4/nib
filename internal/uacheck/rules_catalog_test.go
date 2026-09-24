package uacheck

import (
	"strings"
	"testing"

	"nib/internal/pdfops"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The catalog and metadata rules — `PLAN-accessibility.md` P07.S02.

// verdictOf returns one clause's result from a document's report.
func verdictOf(t *testing.T, pdf []byte, clause string) Result {
	t.Helper()
	rep, err := Check(pdf)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, r := range rep.Results {
		if r.Clause == clause {
			return r
		}
	}
	t.Fatalf("clause %q is not in the report — the registry is not running it, which is "+
		"indistinguishable from a clause that passes", clause)
	return Result{}
}

// plainDoc is a nib-authored page with no metadata, no title and no language — the document P03
// measured and the one most of these clauses fail.
func plainDoc(t *testing.T) []byte {
	t.Helper()
	out, err := pdfops.CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[` +
		`{"value":"plain","anchor":"TopLeft","position":[72,720],"font":{"name":"Helvetica","size":12}}]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTheCatalogRulesAgreeWithWhatP03Measured — the slice's first acceptance clause.
//
// **Every expected verdict below is what veraPDF reported about that exact document**, recorded in
// the plan at P03 and re-measured at this slice's step zero. They are not derived from reading the
// specification, which is the whole point of law 5 existing: a checker agreeing with its author's
// reading of the standard is a checker agreeing with itself.
func TestTheCatalogRulesAgreeWithWhatP03Measured(t *testing.T) {
	plain := plainDoc(t)
	titled, err := pdfops.SetTitle(plain, "A named document")
	if err != nil {
		t.Fatal(err)
	}
	full, err := pdfops.SetLang(titled, "en")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		doc    string
		pdf    []byte
		clause string
		want   Verdict
	}{
		// veraPDF on a plain nib page: 7.1 t8 and 7.1 t10 fail; 7.1 t9 and 5 t1 are NOT EVALUATED
		// because there is no packet to hold them; 7.2 t34 fails.
		{"a plain page", plain, "7.1 t8", Fail},
		{"a plain page", plain, "7.1 t10", Fail},
		{"a plain page", plain, "7.1 t9", NotApplicable},
		{"a plain page", plain, "5 t1", NotApplicable},
		{"a plain page", plain, "7.2 t33", NotApplicable},
		{"a plain page", plain, "7.2 t34", Fail},

		// After SetTitle: the packet exists, so the title clauses become applicable and pass, and
		// 5 t1 and 7.2 t33 become applicable and FAIL — which is what P03 recorded when it said
		// "supplying an artefact CREATES the object other clauses inspect".
		{"a titled page", titled, "7.1 t8", Pass},
		{"a titled page", titled, "7.1 t9", Pass},
		{"a titled page", titled, "7.1 t10", Pass},
		{"a titled page", titled, "5 t1", Fail},
		{"a titled page", titled, "7.2 t33", Fail},

		// And with a language, the two 7.2 clauses clear — measured at P06's close.
		{"a titled page with /Lang", full, "7.2 t33", Pass},
		{"a titled page with /Lang", full, "7.2 t34", Pass},
	} {
		got := verdictOf(t, c.pdf, c.clause)
		if got.Verdict != c.want {
			t.Errorf("%s: %s reports %v (%s), want %v — this is the verdict veraPDF gave this "+
				"document", c.doc, c.clause, got.Verdict, got.Why, c.want)
		}
		if got.Verdict == Fail && (got.Why == "" || got.Where == "") {
			t.Errorf("%s: %s fails without saying why (%q) or where (%q)",
				c.doc, c.clause, got.Why, got.Where)
		}
	}
}

// TestTheMetadataStreamRuleChecksAllThreeThingsTheClauseNames — the clause names the key, the
// stream's /Type and its /Subtype, and veraPDF's error names all three.
//
// A rule checking only presence would pass a document whose `/Metadata` is a dictionary rather than
// a stream, or a stream not identified as metadata. P03's `SetTitle` writes all three, so the rule
// that guards it has to read all three — and the way to know it does is to break each separately.
//
// # Two of the three cannot be driven through `Check`, and finding that out is the point
//
// `open` reads with `ReadValidateAndOptimize`, so **the checker only ever sees documents pdfcpu
// considers valid** — and pdfcpu's own validator refuses a metadata stream whose `/Type` is not
// `/Metadata` (`validateNameEntry: dict=metaDataDict entry=Type invalid dict entry: NotMetadata`).
// A document in that state reaches `Check` as a read error, not as a report.
//
// So those two branches are driven by calling the rule DIRECTLY against a mutated context. They are
// kept rather than deleted because they are correct and cheap, and because the limitation belongs
// to `open`'s reader rather than to the clause — a different read path would reach them. The
// limitation itself is recorded in the seam inventory, and it is one law 5's agreement guard will
// meet head-on: veraPDF validates documents pdfcpu declines to parse.
func TestTheMetadataStreamRuleChecksAllThreeThingsTheClauseNames(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// Control: the untouched document passes, or the mutations below prove nothing.
	if got := verdictOf(t, titled, "7.1 t8"); got.Verdict != Pass {
		t.Fatalf("control: a titled document reports %v (%s) for 7.1 t8", got.Verdict, got.Why)
	}

	// The key's absence is reachable end to end, because a document with no /Metadata is valid.
	got := verdictOf(t, dropMetadataKey(titled), "7.1 t8")
	if got.Verdict != Fail {
		t.Errorf("with the /Metadata key removed, 7.1 t8 reports %v, want Fail", got.Verdict)
	} else if !strings.Contains(got.Why, "no /Metadata key") {
		t.Errorf("the reason does not name the missing key: %q", got.Why)
	}

	// The two type branches, driven directly.
	for _, c := range []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"the stream's /Type changed", "Type", "NotMetadata", "/Type"},
		{"the stream's /Subtype changed", "Subtype", "Text", "/Subtype"},
	} {
		d := docWithMetadataKey(t, titled, c.key, c.value)
		res := checkMetadataStream(d)
		if res.Verdict != Fail {
			t.Errorf("%s: the rule reports %v, want Fail", c.name, res.Verdict)
			continue
		}
		if !strings.Contains(res.Why, c.want) {
			t.Errorf("%s: the reason does not name %s: %q", c.name, c.want, res.Why)
		}
	}
}

// TestADocumentPDFCPUWILLNOTPARSEIsReportedAsUnreadable — the limitation above, asserted rather than
// left as a comment.
//
// This is not a defect to fix in this slice; it is a boundary the checker has, and a boundary nobody
// asserted is one that moves silently. If `open` ever stops validating, this test goes red and the
// two direct-call branches above become reachable end to end — which is the change worth noticing.
func TestADocumentPDFCPUWILLNOTPARSEIsReportedAsUnreadable(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// Control: it reads fine before the mutation.
	if _, cerr := Check(titled); cerr != nil {
		t.Fatalf("control: the unmutated document does not read: %v", cerr)
	}
	_, err = Check(retypeMetadata("Type", "NotMetadata")(titled))
	if err == nil {
		t.Error("a document whose metadata /Type pdfcpu rejects was READ by the checker. That is " +
			"a change worth noticing: `open` validates, so until now the checker could only ever " +
			"see documents pdfcpu accepts, and the /Type and /Subtype branches of 7.1 t8 were " +
			"unreachable end to end")
	}
}

// TestAnUnreadableMetadataPacketIsCannotCheckNotFail.
//
// A packet that is present but will not parse is nib failing to read it, not the document lacking
// what the clause wants. Reporting `Fail` would tell the user to add a title they already have.
func TestAnUnreadableMetadataPacketIsCannotCheckNotFail(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	broken := corruptMetadataXML(titled)
	// **The stimulus, asserted first — because this test once passed on the wrong stimulus.** The
	// original fixture never landed its malformed XML; the reader parsed zlib bytes and reported
	// "invalid UTF-8", which is also CannotCheck. The reason must name the XML the fixture wrote.
	// **Named to the BRANCH, not just to "not well-formed XML".** Three branches share that phrase —
	// an unclosed element, a mismatched end tag and a stray end tag — so the looser assertion could
	// not tell which one fired, and the other two are pinned by name in their own test.
	if got := verdictOf(t, broken, "5 t1"); !strings.Contains(got.Why, "never closed") ||
		strings.Contains(got.Why, "invalid UTF-8") {
		t.Fatalf("setup: the unreadable packet is not the unclosed element this fixture supplies — "+
			"the reason is %q", got.Why)
	}
	for _, clause := range []string{"7.1 t9", "5 t1"} {
		got := verdictOf(t, broken, clause)
		if got.Verdict != CannotCheck {
			t.Errorf("%s on a document whose packet will not parse reports %v (%s), want "+
				"CannotCheck — the document may carry what the clause wants and nib is what "+
				"could not read it", clause, got.Verdict, got.Why)
		}
	}
	// 7.1 t8 still PASSES: the stream is there and correctly typed. The two facts are separate and
	// a checker that conflated them would report a missing stream for a malformed one.
	if got := verdictOf(t, broken, "7.1 t8"); got.Verdict != Pass {
		t.Errorf("7.1 t8 reports %v for a present, correctly-typed stream whose XML is malformed — "+
			"the stream's existence and its parseability are different facts", got.Verdict)
	}
}

// TestTheContentLanguageRuleResolvesEachPieceOfText — 7.2 t34 once the tree walk exists.
//
// P07.S02 shipped this rule answering `CannotCheck` for any tagged document, because a structure
// element may declare the language and walking the tree was S03's. S03 walks it, so the verdicts are
// now established rather than deferred — and the one that changed is the one veraPDF already gave:
// a tagged document whose elements declare no language FAILS.
func TestTheContentLanguageRuleResolvesEachPieceOfText(t *testing.T) {
	plain := plainDoc(t)
	untaggedResult := verdictOf(t, plain, "7.2 t34")
	if untaggedResult.Verdict != Fail {
		t.Errorf("an untagged document with no /Lang reports %v for 7.2 t34, want Fail", untaggedResult.Verdict)
	}
	// **The reason, not just the verdict — found by probing.** Deleting the "in no tagged sequence"
	// branch left this green, because the next guard ALSO fails an unmarked MCID-less text, with a
	// reason about a missing element. Same verdict, different problem handed to the user.
	if !strings.Contains(untaggedResult.Why, "in no tagged sequence") {
		t.Errorf("untagged text is reported as %q, which does not say the text is untagged", untaggedResult.Why)
	}
	tagged := committedProposal(t, plain)
	// A committed proposal writes no element /Lang. veraPDF failed exactly that shape at S02's live
	// verification, on the generic emitter's document this fixture replaced — and the oracle guard
	// measures this one against veraPDF too.
	got := verdictOf(t, tagged, "7.2 t34")
	if got.Verdict != Fail {
		t.Errorf("a tagged document whose elements declare no language reports %v (%s), want Fail — "+
			"the verdict veraPDF gave this exact document", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "ancestors") {
		t.Errorf("the failure does not say the tree was walked for a language: %q", got.Why)
	}
	// And the case S02 could not reach: the ELEMENT declares the language, the catalog does not.
	// This is the pass that makes the walk worth having — without it the rule could still be
	// "tagged means fail".
	withElemLang := langOnEveryElement(t, tagged, "en")
	if cl := verdictOf(t, withElemLang, "7.2 t34"); cl.Verdict != Pass {
		t.Errorf("a tagged document whose ELEMENTS declare /Lang and whose catalog does not reports "+
			"%v (%s), want Pass — language inherits down the tree (ISO 32000-1 §14.9.2)", cl.Verdict, cl.Why)
	}
	// **Inherited, not only direct — found by probing.** The fixture above sets /Lang on EVERY
	// element, so the walk to an ancestor was never needed and removing it left the suite green.
	// Converted Markdown nests (L → LI → LBody owns the MCID), so a /Lang on the top-level elements
	// ONLY must reach the text through the ancestor chain.
	md, err := pdfops.ConvertDocToPDF([]byte("- first item\n- second item\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	inherited := langOnTopLevelOnly(t, md, "en")
	if got := verdictOf(t, inherited, "7.2 t34"); got.Verdict != Pass {
		t.Errorf("a /Lang declared only on top-level elements, with the MCIDs owned by their "+
			"descendants, reports %v (%s at %s), want Pass — language inherits down the tree",
			got.Verdict, got.Why, got.Where)
	}
	if lang := verdictOf(t, withElemLang, "7.2 t33"); lang.Verdict == Pass {
		// Not an assertion about 7.2 t34; recorded so the two clauses are not confused: an element
		// language says nothing about the METADATA's language.
		t.Logf("note: 7.2 t33 reports %v on the same document", lang.Verdict)
	}
}

// TestTheOptionalContentRulesReadEveryConfigurationDictionary — the clause says "each".
//
// The population is `/D` plus every `/Configs` entry, which is the population
// `honestOptionalContent` corrects. A rule reaching one site of two is ADR-009's defect inside a
// checker, and pdfcpu writing only `/D` today is not the clause's population.
func TestTheOptionalContentRulesReadEveryConfigurationDictionary(t *testing.T) {
	// A stamped document: nib now corrects both clauses at the stamping door, so this PASSES —
	// and that pass is the control for the two failures below.
	stamped, err := pdfops.StampWatermark(plainDoc(t), "DRAFT", pdfops.WatermarkStyle{})
	if err != nil {
		t.Fatal(err)
	}
	for _, clause := range []string{"7.10 t1", "7.10 t2"} {
		if got := verdictOf(t, stamped, clause); got.Verdict != Pass {
			t.Errorf("control: a nib-stamped document reports %v (%s) for %s. `/pending 473` "+
				"fixed both at the stamping door, so a failure here means either the fix "+
				"regressed or the rule is wrong", got.Verdict, got.Why, clause)
		}
	}
	// A document with no optional content at all: both clauses have no subject.
	for _, clause := range []string{"7.10 t1", "7.10 t2"} {
		if got := verdictOf(t, plainDoc(t), clause); got.Verdict != NotApplicable {
			t.Errorf("a document with no optional content reports %v for %s, want NotApplicable — "+
				"rating it Pass would make these rules look exercised across a corpus where they "+
				"were never asked", got.Verdict, clause)
		}
	}
	// And the defect itself, in each of the two places the clause's population has.
	for _, c := range []struct {
		name   string
		pdf    []byte
		clause string
		where  string
	}{
		{"/AS restored on /D", withASOnDefault(t, stamped), "7.10 t2", "/D"},
		{"/Name dropped from /D", withoutNameOnDefault(t, stamped), "7.10 t1", "/D"},
		{"a second config under /Configs carrying /AS", withBadConfigsEntry(t, stamped), "7.10 t2", "/Configs"},
	} {
		got := verdictOf(t, c.pdf, c.clause)
		if got.Verdict != Fail {
			t.Errorf("%s: %s reports %v (%s), want Fail", c.name, c.clause, got.Verdict, got.Why)
			continue
		}
		if !strings.Contains(got.Where, c.where) {
			t.Errorf("%s: the failure says %q, which does not name the %s it was found in",
				c.name, got.Where, c.where)
		}
	}
}

// TestTheTitleDisplayRuleReadsTheVALUEAndNotJustTheKey.
//
// **Found by probing, not by reading**: no document in the table above carries `DisplayDocTitle
// false`, so disabling the value check left every test green. A viewer told `false` shows the file
// name — which is the exact outcome the clause exists to prevent — and the key being present is not
// the fact the clause is about.
func TestTheTitleDisplayRuleReadsTheVALUEAndNotJustTheKey(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// Control: true passes.
	if got := verdictOf(t, titled, "7.1 t10"); got.Verdict != Pass {
		t.Fatalf("control: a titled document reports %v for 7.1 t10", got.Verdict)
	}
	// `false` is reachable end to end: it is a valid document, merely a non-conformant one.
	got := verdictOf(t, withDisplayDocTitle(t, titled, false), "7.1 t10")
	if got.Verdict != Fail {
		t.Errorf("DisplayDocTitle false reports %v (%s), want Fail — a viewer told false shows the "+
			"file name, which is the outcome this clause exists to prevent", got.Verdict, got.Why)
	} else if !strings.Contains(got.Why, "is false") {
		t.Errorf("the reason does not say the value is false: %q", got.Why)
	}

	// **The wrong-TYPE branch cannot be driven end to end**, and that is the second instance of the
	// boundary `7.1 t8` hit: pdfcpu PANICS writing a document whose /DisplayDocTitle is a name
	// rather than a boolean, so no such document can be produced to hand the checker. Driven
	// directly, and the branch is kept because a document from another producer could well arrive
	// in that state — nib is not the only thing that writes PDFs.
	res := checkDisplayDocTitle(docWithViewerPref(t, titled, "DisplayDocTitle", types.Name("true")))
	if res.Verdict != Fail {
		t.Errorf("a /DisplayDocTitle that is a NAME reports %v, want Fail", res.Verdict)
	} else if !strings.Contains(res.Why, "want a boolean") {
		t.Errorf("the reason does not name the type problem: %q", res.Why)
	}
}

// TestAWrongIdentificationIsDistinguishedFromAMissingOne.
//
// **Also found by probing.** Deleting the "no pdfuaid:part" branch left the suite green, because the
// next guard caught an empty value too and returned `Fail` with a different message — a dead
// conjunct hidden by its own successor. The verdicts were the same and the REASONS were not, and a
// user told *"pdfuaid:part is \"\""* has been handed a different problem from *"there is no
// pdfuaid:part"*.
//
// **Two clauses since `/pending 489`, as veraPDF has them.** veraPDF's 5 t1 is the identification's
// presence and its 5 t2 the part's value (measured on its corpus file `5-t02-fail-a.pdf`), so a
// missing identification fails 5 t1 and has no 5 t2 subject, while a wrong part passes 5 t1 and fails
// 5 t2 — still naming the value, so the two cases stay distinguishable to a user.
func TestAWrongIdentificationIsDistinguishedFromAMissingOne(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	missing := verdictOf(t, titled, "5 t1")
	if missing.Verdict != Fail {
		t.Fatalf("control: a titled document reports %v for 5 t1, want Fail", missing.Verdict)
	}
	if !strings.Contains(missing.Why, "no property in the PDF/UA identification namespace") {
		t.Errorf("an ABSENT identification is reported as %q, which does not say it is absent",
			missing.Why)
	}
	if got := verdictOf(t, titled, "5 t2"); got.Verdict != NotApplicable {
		t.Errorf("an ABSENT identification reports %v for 5 t2 (%s), want NotApplicable — there is no part value to judge", got.Verdict, got.Why)
	}
	part2 := withUAPart(t, titled, "2")
	if got := verdictOf(t, part2, "5 t1"); got.Verdict != Pass {
		t.Errorf("a document declaring pdfuaid:part 2 reports %v for 5 t1 (%s), want Pass — the identification is present", got.Verdict, got.Why)
	}
	wrong := verdictOf(t, part2, "5 t2")
	if wrong.Verdict != Fail {
		t.Errorf("a document declaring pdfuaid:part 2 reports %v for 5 t2, want Fail — a PDF/UA-1 file declares part 1", wrong.Verdict)
	}
	if !strings.Contains(wrong.Why, `"2"`) {
		t.Errorf("a WRONG identification is reported as %q, which does not name the value found — "+
			"so it reads as the absent case and the two branches are indistinguishable to a user",
			wrong.Why)
	}
	if got := verdictOf(t, withUAPart(t, titled, "1"), "5 t2"); got.Verdict != Pass {
		t.Errorf("a document declaring pdfuaid:part 1 reports %v for 5 t2 (%s), want Pass", got.Verdict, got.Why)
	}
}

// TestAPropertyIsReadByNamespaceNotByPrefix — the claim `xmp.go` makes, executed.
//
// A prefix is the document's choice. A packet may bind `dc:` to some other URI entirely, and then
// `<dc:title>` is not the Dublin Core title at all — a reader trusting the prefix would find a title
// that is not there and report 7.1 t9 passing.
func TestAPropertyIsReadByNamespaceNotByPrefix(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// Control: the right binding passes, or the failure below could be any unreadable packet.
	right := withPacket(t, titled, "http://purl.org/dc/elements/1.1/")
	if got := verdictOf(t, right, "7.1 t9"); got.Verdict != Pass {
		t.Fatalf("control: a dc:title in the Dublin Core namespace reports %v (%s)", got.Verdict, got.Why)
	}
	wrong := withPacket(t, titled, "http://example.com/not-dublin-core/")
	got := verdictOf(t, wrong, "7.1 t9")
	if got.Verdict != Fail {
		t.Errorf("a `dc:title` whose prefix is bound to a DIFFERENT namespace reports %v (%s), want "+
			"Fail — the element is not the Dublin Core title, and a prefix match found one anyway",
			got.Verdict, got.Why)
	}
}

// TestMetadataLanguageFollowsTheLanguageAlternatives — 7.2 t33, as law 5 measured it.
func TestMetadataLanguageFollowsTheLanguageAlternatives(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	alt := func(prop, lang string) string {
		return `<dc:` + prop + `><rdf:Alt><rdf:li xml:lang="` + lang + `">text</rdf:li></rdf:Alt></dc:` + prop + `>`
	}
	for _, c := range []struct {
		name string
		body string
		want Verdict
	}{
		{"dc:title x-default, no catalog /Lang", alt("title", "x-default"), Fail},
		{"dc:description x-default", alt("description", "x-default"), Fail},
		{"dc:rights x-default", alt("rights", "x-default"), Fail},
		{"dc:creator as a Seq — no language alternative", `<dc:creator><rdf:Seq><rdf:li>Someone</rdf:li></rdf:Seq></dc:creator>`, NotApplicable},
		{"xmp:CreateDate alone", `<xmp:CreateDate>2026-09-14T00:00:00Z</xmp:CreateDate>`, NotApplicable},
		{"dc:title with its own xml:lang, no catalog /Lang", alt("title", "en"), Pass},
	} {
		got := verdictOf(t, withPacketBody(t, titled, c.body), "7.2 t33")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.2 t33 reports %v (%s), want %v — measured against veraPDF", c.name, got.Verdict, got.Why, c.want)
		}
	}
}

// 5 t3, 5 t4 and 5 t5 — the identification's namespace prefixes (P06.S01).
//
// **Every expectation here was measured on veraPDF 1.30.2 before it was written**, on its own corpus
// files and on length-preserving mutations of them. The clause is easy to get backwards in two ways,
// and both are covered below: an absent property PASSES rather than having no subject, and the
// property is located by NAMESPACE while being judged by the PREFIX that namespace was bound to.
func TestTheIdentificationsPrefixesAreJudgedNotItsValues(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	for _, clause := range []string{"5 t3", "5 t4", "5 t5"} {
		// No identification at all: no subject. Measured — veraPDF reports 0 passed, 0 failed on its
		// own `5-t01-fail-a.pdf` for all three.
		if got := verdictOf(t, titled, clause); got.Verdict != NotApplicable {
			t.Errorf("with no identification, %s reports %v (%s), want NotApplicable", clause, got.Verdict, got.Why)
		}
		// The required prefix on all three properties.
		if got := verdictOf(t, withUAPrefix(t, titled, "pdfuaid"), clause); got.Verdict != Pass {
			t.Errorf("under the pdfuaid prefix, %s reports %v (%s), want Pass", clause, got.Verdict, got.Why)
		}
		// A foreign prefix bound to the RIGHT namespace: found, and refused.
		got := verdictOf(t, withUAPrefix(t, titled, "pdfuaia"), clause)
		if got.Verdict != Fail {
			t.Errorf("under a foreign prefix, %s reports %v (%s), want Fail", clause, got.Verdict, got.Why)
		}
		if !strings.Contains(got.Why, "pdfuaia") {
			t.Errorf("%s refuses with %q, which does not name the prefix it found", clause, got.Why)
		}
	}
	// **An ABSENT property is a passing check, not an absent subject** — `withUAPart` writes `part`
	// alone, so `amd` and `corr` are missing and both must pass. This is the trap P05 measured in the
	// annotation family, one phase later: veraPDF reports `passedChecks="1"` for 5 t4 and 5 t5 on its
	// own `5-t03-pass-a.pdf`, which carries neither.
	// **A property under the DEFAULT namespace has no prefix, and that PASSES.** Measured on veraPDF
	// with a length-preserving mutation of `5-t03-pass-a.pdf` that rebinds `xmlns:pdfuaid` to a bare
	// `xmlns` and writes `<part>`: `5 t2` still reports `part == 1`, so the property was found, and
	// `5 t3` reports `passedChecks="1"`. nib refused it until a blind mutation pass asked — every
	// targeted probe had used a WRONG prefix and none a missing one.
	for _, clause := range []string{"5 t3", "5 t4", "5 t5"} {
		if got := verdictOf(t, withUADefaultNamespace(t, titled), clause); got.Verdict != Pass {
			t.Errorf("under the default namespace, %s reports %v (%s), want Pass — no prefix is the "+
				"`null` half of the profile's disjunction, not a wrong prefix", clause, got.Verdict, got.Why)
		}
	}
	// **A binding made on the OUTERMOST element resolves like any other.** Real packets declare their
	// namespaces on `rdf:Description`, so a reader whose scope stack skipped the root element passed
	// every other fixture in this package — found by a blind mutation pass.
	if got := verdictOf(t, withUARootBinding(t, titled), "5 t3"); got.Verdict != Fail {
		t.Errorf("with the identification bound on the packet's root element, 5 t3 reports %v (%s), "+
			"want Fail — the binding is a foreign prefix wherever it was declared", got.Verdict, got.Why)
	}

	partOnly := withUAPart(t, titled, "1")
	for _, clause := range []string{"5 t4", "5 t5"} {
		if got := verdictOf(t, partOnly, clause); got.Verdict != Pass {
			t.Errorf("with the property absent, %s reports %v (%s), want Pass — nothing declared it, so "+
				"nothing declared it wrongly", clause, got.Verdict, got.Why)
		}
	}
}

// **The identification exists on ANY property in its namespace, not on `part`.** Measured on veraPDF
// by three length-preserving mutations of `5-t03-pass-a.pdf`: a packet whose only pdfuaid property is
// `corr` PASSES 5 t1 and FAILS 5 t2, and one whose only property is an unknown name does the same,
// while a packet carrying the pdfaExtension schema for the namespace but no property in it fails 5 t1
// and leaves the rest of the family with no subject.
//
// nib had both of those backwards until P06.S01 — a live false fail on 5 t1 and a live false pass on
// 5 t2 — and the corpus could not see either, because it holds no such file.
func TestAnIdentificationWithoutAPartStillExists(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	corrOnly := withUAProperty(t, titled, "corr", "0")
	if got := verdictOf(t, corrOnly, "5 t1"); got.Verdict != Pass {
		t.Errorf("a packet whose only identification property is corr reports %v for 5 t1 (%s), want "+
			"Pass — the identification is present", got.Verdict, got.Why)
	}
	got := verdictOf(t, corrOnly, "5 t2")
	if got.Verdict != Fail {
		t.Fatalf("an identification with no part reports %v for 5 t2 (%s), want Fail — veraPDF's test is "+
			"`part == 1` and a null part does not satisfy it", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "no pdfuaid:part") {
		t.Errorf("5 t2 refuses with %q, which does not say the part is the thing that is missing", got.Why)
	}
	// An unknown property in the namespace counts too — measured, veraPDF passes 5 t1 on a `zzzz`.
	if got := verdictOf(t, withUAProperty(t, titled, "zzzz", "x"), "5 t1"); got.Verdict != Pass {
		t.Errorf("a packet whose only identification property is an unknown name reports %v for 5 t1 "+
			"(%s), want Pass", got.Verdict, got.Why)
	}
}

// **A packet whose tags do not NEST is unreadable too, and that is a separate condition from one
// whose tags are never closed** (P06.S01, found by probing).
//
// It needs its own test because of how `parseXMP` reads the packet. `Token()` verified that start and
// end elements match and returned an error when they did not; `RawToken()`, which the prefix clauses
// require, performs no such check, so nib now makes it itself. `corruptMetadataXML`'s fixture leaves
// an element UNCLOSED, which a different branch catches — so removing the mismatch check left every
// test in the package green while a packet like `<a></b>` read as perfectly well-formed, and nib would
// have reported `Fail` over a document it had misparsed rather than `CannotCheck`.
func TestAPacketWhoseTagsDoNotNestIsCannotCheck(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// Well-formed but for the one thing under test: every tag is closed, and one is closed by the
	// wrong name. A reader that only counts opens and closes cannot tell this from the control.
	broken := withRawPacket(t, titled, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
		`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
		`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
		`<rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">`+
		`<pdfuaid:part>1</pdfuaid:corr>`+
		`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)

	got := verdictOf(t, broken, "5 t1")
	// The stimulus before the response, as the sibling test above does: the refusal must be about the
	// nesting, not about some other way the fixture failed to land.
	if !strings.Contains(got.Why, "mismatched end tag") {
		t.Fatalf("setup: the packet was refused as %q, which is not the mismatched nesting this "+
			"fixture supplies", got.Why)
	}
	for _, clause := range []string{"5 t1", "5 t2", "5 t3", "5 t4", "5 t5", "7.1 t9"} {
		if got := verdictOf(t, broken, clause); got.Verdict != CannotCheck {
			t.Errorf("%s on a packet whose tags do not nest reports %v (%s), want CannotCheck — nib "+
				"could not read the packet, which is not the same as the document lacking what the "+
				"clause wants", clause, got.Verdict, got.Why)
		}
	}

	// **An end tag with nothing open is a third condition**, and it is the one that would CRASH rather
	// than answer: the reader's stack is empty, so without its own guard the pop indexes a zero-length
	// slice. `runOne` would recover the panic into a CannotCheck, so the package stays green and the
	// clause's reason becomes a stack message — measured reachable, from a stray tag either before or
	// after the root element.
	for _, packet := range []string{
		`<x:xmpmeta xmlns:x="adobe:ns:meta/"></x:xmpmeta></stray>`,
		`</stray><x:xmpmeta xmlns:x="adobe:ns:meta/"></x:xmpmeta>`,
	} {
		got := verdictOf(t, withRawPacket(t, titled, packet), "5 t1")
		if got.Verdict != CannotCheck {
			t.Errorf("a packet with a stray end tag reports %v (%s), want CannotCheck", got.Verdict, got.Why)
		}
		if !strings.Contains(got.Why, "end tag with no open element") {
			t.Errorf("a stray end tag is refused as %q, which is not the reader's own diagnosis — a "+
				"recovered panic reads as CannotCheck too and says nothing useful", got.Why)
		}
	}
}

// **Attribute-form properties — RDF/XML's abbreviated syntax — are ordinary XMP, and nib read none
// of them** (P06.S01, found by review). A simple-valued property may be written as an attribute on
// `rdf:Description` rather than as a child element, and veraPDF reads it either way.
//
// Measured on a length-preserving mutation of `5-t03-pass-a.pdf`: with `pdfuaid:part="1"` as an
// attribute, veraPDF passes all five clause-5 tests, while nib reported `5 t1` **Fail** and the other
// four NotApplicable — a live false fail across a whole serialisation form, invisible to the corpus
// because every corpus identification is element-form.
func TestAnAttributeFormIdentificationIsRead(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// All three properties as attributes under the required prefix: veraPDF passes all five.
	ok := withUAAttributes(t, titled, "pdfuaid", "1")
	for _, clause := range []string{"5 t1", "5 t2", "5 t3", "5 t4", "5 t5"} {
		if got := verdictOf(t, ok, clause); got.Verdict != Pass {
			t.Errorf("with an attribute-form identification, %s reports %v (%s), want Pass — "+
				"measured on veraPDF, which passes all five", clause, got.Verdict, got.Why)
		}
	}
	// The value is read, not merely the presence: a part of 2 fails 5 t2 in attribute form too.
	if got := verdictOf(t, withUAAttributes(t, titled, "pdfuaid", "2"), "5 t2"); got.Verdict != Fail {
		t.Errorf("an attribute-form pdfuaid:part of 2 reports %v (%s) for 5 t2, want Fail", got.Verdict, got.Why)
	}
	// And the PREFIX is read: the same attributes under a foreign prefix fail the prefix clauses.
	bad := withUAAttributes(t, titled, "pdfuaia", "1")
	for _, clause := range []string{"5 t3", "5 t4", "5 t5"} {
		if got := verdictOf(t, bad, clause); got.Verdict != Fail {
			t.Errorf("attribute-form properties under a foreign prefix report %v (%s) for %s, want Fail",
				got.Verdict, got.Why, clause)
		}
	}
	// An UNPREFIXED attribute is in no namespace at all under XML Namespaces, so it is not a property
	// of the identification and must not create a subject.
	if got := verdictOf(t, withUnprefixedPartAttribute(t, titled), "5 t1"); got.Verdict != Fail {
		t.Errorf("a bare part=\"1\" attribute reports %v (%s) for 5 t1, want Fail — an unprefixed "+
			"attribute is in no namespace and names no property", got.Verdict, got.Why)
	}
}

// **A property's prefix and its value must come from the SAME element.** Recording the prefix at the
// start tag and letting any later character data fill the value let the two be synthesised from two
// different elements — a pairing no element in the packet has, which both the value clause and the
// prefix clause then judged.
func TestAPropertysPrefixAndValueComeFromOneElement(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// An empty `pdfuaid:part` first, then a `pdfuaia:part` carrying the value. First occurrence wins,
	// so the property is the EMPTY one: prefix `pdfuaid`, no value.
	doc := withRawPacket(t, titled, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
		`<x:xmpmeta xmlns:x="adobe:ns:meta/">`+
		`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">`+
		`<rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" `+
		`xmlns:pdfuaia="http://www.aiim.org/pdfua/ns/id/">`+
		`<pdfuaid:part></pdfuaid:part><pdfuaia:part>1</pdfuaia:part>`+
		`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	if got := verdictOf(t, doc, "5 t3"); got.Verdict != Pass {
		t.Errorf("5 t3 reports %v (%s); the first part is written `pdfuaid:`, so the prefix passes — "+
			"a Fail means the prefix was taken from the second element", got.Verdict, got.Why)
	}
	got := verdictOf(t, doc, "5 t2")
	if got.Verdict != Fail {
		t.Fatalf("5 t2 reports %v (%s); the first part carries no value, so it cannot be 1 — a Pass "+
			"means the value was taken from a different element than the prefix", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "present but empty") {
		t.Errorf("5 t2 refuses with %q, which does not distinguish a declared-and-empty part from an "+
			"absent one — they are different things to fix", got.Why)
	}
}

// **A packet nested past the reader's ceiling is refused, not parsed.** Before the bound, resolving a
// prefix walked the open-element stack, so the parse was O(depth²) on a packet whose decoded size the
// document chooses: measured end to end through `Check`, 5,000 deep took 57 ms, 20,000 deep 667 ms and
// 80,000 deep **11.1 s** — an attacker-supplied quadratic reached by opening a PDF.
func TestADeeplyNestedPacketIsRefusedRatherThanWalked(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	deep := withRawPacket(t, titled, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
		`<x:xmpmeta xmlns:x="adobe:ns:meta/" xmlns:z="urn:z">`+
		strings.Repeat("<z:a>", maxXMPDepth+5)+strings.Repeat("</z:a>", maxXMPDepth+5)+
		`</x:xmpmeta><?xpacket end="w"?>`)
	got := verdictOf(t, deep, "5 t1")
	if got.Verdict != CannotCheck {
		t.Fatalf("a packet nested past the ceiling reports %v (%s) for 5 t1, want CannotCheck — nib "+
			"stopped reading, which is not the same as the document lacking an identification", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "nests more than") {
		t.Errorf("the refusal is %q, which does not name the bound nib hit", got.Why)
	}
	// The stimulus, asserted: one element under the ceiling must still be READ, or the test above
	// would pass against a reader that refused every packet.
	shallow := withRawPacket(t, titled, `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>`+
		`<x:xmpmeta xmlns:x="adobe:ns:meta/" xmlns:z="urn:z">`+
		strings.Repeat("<z:a>", maxXMPDepth-5)+strings.Repeat("</z:a>", maxXMPDepth-5)+
		`</x:xmpmeta><?xpacket end="w"?>`)
	if got := verdictOf(t, shallow, "5 t1"); got.Verdict != Fail {
		t.Errorf("a packet just under the ceiling reports %v (%s) for 5 t1, want Fail — it is readable "+
			"and carries no identification", got.Verdict, got.Why)
	}
}

// **Each prefix clause reads ITS OWN property, and nothing else pinned that.** Every other fixture
// gives `part`, `amd` and `corr` the same fate at once, so swapping the property names in the `5 t4`
// and `5 t5` registrations left the whole package green except the corpus test — which is skipped
// wherever veraPDF's corpus is absent. One property wrong at a time is what separates them.
func TestEachPrefixClauseReadsItsOwnProperty(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ prop, clause string }{
		{"part", "5 t3"}, {"amd", "5 t4"}, {"corr", "5 t5"},
	} {
		doc := withOneUAPropertyWronglyPrefixed(t, titled, c.prop)
		if got := verdictOf(t, doc, c.clause); got.Verdict != Fail {
			t.Errorf("with only %s wrongly prefixed, %s reports %v (%s), want Fail — the clause is "+
				"reading some other property", c.prop, c.clause, got.Verdict, got.Why)
		}
		// And every OTHER prefix clause must pass on that same document, or the three are not distinct.
		for _, other := range []string{"5 t3", "5 t4", "5 t5"} {
			if other == c.clause {
				continue
			}
			if got := verdictOf(t, doc, other); got.Verdict != Pass {
				t.Errorf("with only %s wrongly prefixed, %s reports %v (%s), want Pass — it is reading "+
					"%s rather than its own property", c.prop, other, got.Verdict, got.Why, c.prop)
			}
		}
	}
}

// **`pdfuaid:part` is the element's whole text, untrimmed, read as an INTEGER** (the P06 phase-close review).
// Every row was run on veraPDF 1.30.2 first; the verdict is veraPDF's. nib used to trim and take the first
// text run, then compare to the string "1" — four of these eight disagreed.
func TestTheIdentificationPartIsAnIntegerOverTheWholeText(t *testing.T) {
	md, err := pdfops.ConvertDocToPDF([]byte("# Heading\n\nA paragraph.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	mdt, err := pdfops.SetTitle(md, "A named document")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		part string
		want Verdict
		why  string
	}{
		{"1", Pass, "the plain case"},
		{"01", Pass, "an integer, so a leading zero is still 1"},
		{"+1", Pass, "and so is a sign"},
		{"&#x31;", Pass, "a character reference is the character"},
		{" 1 ", Fail, "untrimmed: a space is not a digit"},
		{"\n1\n", Fail, "nor is a newline"},
		{"1<!---->1", Fail, "the whole text — a comment splits nothing — and 11 is not 1"},
		{"1.0", Fail, "not an integer"},
		{"<rdf:Description><rdf:value>1</rdf:value></rdf:Description>", Pass, "a qualified value is read from rdf:value"},
		{"\n  <rdf:Description>\n    <rdf:value>1</rdf:value>\n  </rdf:Description>\n", Pass, "pretty-printed: the indentation is not the value (R1 re-review)"},
		{"<rdf:Description><rdf:value>1</rdf:value><xmp:q xmlns:xmp=\"http://ns.adobe.com/xap/1.0/\">2</xmp:q></rdf:Description>", Pass, "and another qualifier is not part of it"},
		{`<rdf:Description rdf:value="1"/>`, Pass, "rdf:value written as an attribute (R1 round 3)"},
	} {
		if got := verdictOf(t, withUAPart(t, mdt, c.part), "5 t2"); got.Verdict != c.want {
			t.Errorf("pdfuaid:part %q reports %v (%s), want %v — %s (measured on veraPDF)", c.part, got.Verdict, got.Why, c.want, c.why)
		}
	}
	// And on the property element itself, which `withUAPart` cannot write (it wraps the value in an element).
	attr := withPacketBody(t, mdt, `<dc:title><rdf:Alt><rdf:li xml:lang="x-default">A named document</rdf:li></rdf:Alt></dc:title>`+
		`<pdfuaid:part xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" rdf:value="1"/>`)
	if got := verdictOf(t, attr, "5 t2"); got.Verdict != Pass {
		t.Errorf("<pdfuaid:part rdf:value=\"1\"/> reports %v (%s), want Pass — measured on veraPDF", got.Verdict, got.Why)
	}
}
