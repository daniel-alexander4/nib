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
	if got := verdictOf(t, broken, "5 t1"); !strings.Contains(got.Why, "not well-formed XML") ||
		strings.Contains(got.Why, "invalid UTF-8") {
		t.Fatalf("setup: the unreadable packet is not the malformed XML this fixture supplies — "+
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
	tagged, n, err := pdfops.TagAuthored(plain)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("setup: nothing was wrapped, so the tagged case IS the untagged one")
	}
	// veraPDF FAILED this document at S02's live verification: TagAuthored writes no element /Lang.
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
// next guard catches an empty value too and returns `Fail` with a different message — a dead
// conjunct hidden by its own successor. The verdicts are the same and the REASONS are not, and a
// user told *"pdfuaid:part is \"\""* has been handed a different problem from *"there is no
// pdfuaid:part"*.
func TestAWrongIdentificationIsDistinguishedFromAMissingOne(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	missing := verdictOf(t, titled, "5 t1")
	if missing.Verdict != Fail {
		t.Fatalf("control: a titled document reports %v for 5 t1, want Fail", missing.Verdict)
	}
	if !strings.Contains(missing.Why, "no pdfuaid:part") {
		t.Errorf("an ABSENT identification is reported as %q, which does not say it is absent",
			missing.Why)
	}
	wrong := verdictOf(t, withUAPart(t, titled, "2"), "5 t1")
	if wrong.Verdict != Fail {
		t.Errorf("a document declaring pdfuaid:part 2 reports %v for 5 t1, want Fail — nib checks "+
			"against PDF/UA-1 and cannot speak for part 2", wrong.Verdict)
	}
	if !strings.Contains(wrong.Why, `"2"`) {
		t.Errorf("a WRONG identification is reported as %q, which does not name the value found — "+
			"so it reads as the absent case and the two branches are indistinguishable to a user",
			wrong.Why)
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
