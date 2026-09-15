package uacheck

import (
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The catalog and metadata rules — `PLAN-accessibility.md` P07.S02.
//
// Every clause here is answerable from the catalog alone, which is what makes them the second slice:
// they need no structure tree, no content stream and no font reading. Each one's expected verdict is
// what veraPDF reported about a real nib document during P03 and P06, and the clause texts below are
// veraPDF's own descriptions rather than paraphrases.

func init() {
	register(Rule{
		Clause:  "7.1 t8",
		Summary: "the catalog shall contain a Metadata key whose value is a metadata stream",
		Check:   checkMetadataStream,
	})
	register(Rule{
		Clause:  "7.1 t9",
		Summary: "the document metadata stream shall contain a dc:title that describes the document",
		Check:   checkDocumentTitle,
	})
	register(Rule{
		Clause:  "7.1 t10",
		Summary: "the catalog shall include a ViewerPreferences dictionary with DisplayDocTitle true",
		Check:   checkDisplayDocTitle,
	})
	register(Rule{
		Clause:  "7.2 t33",
		Summary: "natural language for document metadata shall be determined",
		Check:   checkMetadataLanguage,
	})
	register(Rule{
		Clause:  "7.2 t34",
		Summary: "natural language for text in page content shall be determined",
		Check:   checkContentLanguage,
	})
	register(Rule{
		Clause:  "5 t1",
		Summary: "the PDF/UA version and conformance level shall be specified using the PDF/UA Identification extension schema",
		Check:   checkUAIdentification,
	})
	register(Rule{
		Clause:  "5 t2",
		Summary: `the value of "pdfuaid:part" shall be the part number of the International Standard to which the file conforms`,
		Check:   checkUAPartValue,
	})
	register(Rule{
		Clause:  "7.10 t1",
		Summary: "each optional content configuration dictionary shall contain the Name key",
		Check:   checkOptionalContentName,
	})
	register(Rule{
		Clause:  "7.10 t2",
		Summary: "the AS key shall not appear in any optional content configuration dictionary",
		Check:   checkOptionalContentAutoState,
	})
}

// checkMetadataStream evaluates ua1 7.1 t8.
//
// **The clause names three things and veraPDF's error names all three**: *"doesn't contain metadata
// key or metadata stream dictionary does not contain either entry Type with…"*. A rule checking only
// that the key is present would pass a document whose `/Metadata` is a dictionary rather than a
// stream, or a stream not identified as metadata — and P03 built `SetTitle` to write all three, so
// the rule that guards it has to read all three.
func checkMetadataStream(d *Document) Result {
	x := readXMP(d)
	if !x.Present {
		return Result{
			Verdict: Fail,
			Why:     "the document catalog has no /Metadata key, so the document carries no XMP metadata at all",
			Where:   "catalog",
		}
	}
	if x.StreamType != "Metadata" {
		return Result{
			Verdict: Fail,
			Why:     fmt.Sprintf("the metadata stream's /Type is %q, want /Metadata", x.StreamType),
			Where:   "catalog /Metadata",
		}
	}
	if x.StreamSubtype != "XML" {
		return Result{
			Verdict: Fail,
			Why:     fmt.Sprintf("the metadata stream's /Subtype is %q, want /XML", x.StreamSubtype),
			Where:   "catalog /Metadata",
		}
	}
	return Result{Verdict: Pass}
}

// checkDocumentTitle evaluates ua1 7.1 t9.
//
// `NotApplicable` with no packet is measured, not reasoned: veraPDF did not evaluate this clause at
// all on a nib document with no `/Metadata`. That is the distinction law 4's fourth value exists for
// — the clause has no subject, which is neither the document satisfying it nor nib failing to look.
// **7.1 t8 is what reports the missing packet**, and reporting it twice would make one defect look
// like two.
func checkDocumentTitle(d *Document) Result {
	x := readXMP(d)
	if !x.Present {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document has no metadata stream, so there is no dc:title to require (7.1 t8 reports the missing stream)",
		}
	}
	if !x.Readable {
		return Result{Verdict: CannotCheck, Why: x.Why, Where: "catalog /Metadata"}
	}
	if x.Title == "" {
		return Result{
			Verdict: Fail,
			Why:     "the metadata packet carries no dc:title, so a reader has no name for this document but its file name",
			Where:   "catalog /Metadata, dc:title",
		}
	}
	return Result{Verdict: Pass}
}

// checkDisplayDocTitle evaluates ua1 7.1 t10.
//
// The clause wants the viewer to show the document's title rather than its file name, and P03's
// `SetTitle` writes it beside the title itself through one door — *"a viewer told to display a title
// it cannot find shows an empty chrome bar"*, which is why the two travel together there and why
// this rule does not excuse a missing title.
func checkDisplayDocTitle(d *Document) Result {
	prefs := d.dict(d.Catalog["ViewerPreferences"])
	if prefs == nil {
		return Result{
			Verdict: Fail,
			Why:     "the catalog has no /ViewerPreferences dictionary, so nothing tells a viewer to display the document's title",
			Where:   "catalog",
		}
	}
	v, has := prefs["DisplayDocTitle"]
	if !has {
		return Result{
			Verdict: Fail,
			Why:     "/ViewerPreferences has no /DisplayDocTitle key",
			Where:   "catalog /ViewerPreferences",
		}
	}
	b, isBool := v.(types.Boolean)
	if !isBool {
		return Result{
			Verdict: Fail,
			Why:     fmt.Sprintf("/DisplayDocTitle is %T, want a boolean", v),
			Where:   "catalog /ViewerPreferences /DisplayDocTitle",
		}
	}
	if !bool(b) {
		return Result{
			Verdict: Fail,
			Why:     "/DisplayDocTitle is false, so a viewer shows the file name rather than the document's own title",
			Where:   "catalog /ViewerPreferences /DisplayDocTitle",
		}
	}
	return Result{Verdict: Pass}
}

// checkMetadataLanguage evaluates ua1 7.2 t33 — natural language for document metadata.
//
// # The subject and the ways to satisfy it, both measured by law 5's guard
//
// The first version of this rule said "a packet exists and the catalog has no /Lang → Fail". The
// S05 guard found veraPDF reporting NO SUBJECT on a packet holding only `xmp:CreateDate`, and the
// measurement that followed, one packet shape at a time with no catalog /Lang:
//
//	dc:title / dc:description / dc:rights as rdf:Alt, xml:lang="x-default"  → FAILED
//	dc:creator as rdf:Seq, or xmp:CreateDate alone                          → no subject
//	dc:title as rdf:Alt with xml:lang="en"                                  → PASSED
//
// So the subject is language-alternative text, and each alternative is determined either by its own
// `xml:lang` or — for `x-default`, which names no language — by the catalog /Lang. nib's own
// `SetTitle` writes `x-default`, which is why a titled document with no /Lang fails.
func checkMetadataLanguage(d *Document) Result {
	x := readXMP(d)
	if !x.Present {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document has no metadata stream, so there is no metadata whose language could be determined",
		}
	}
	if !x.Readable {
		return Result{Verdict: CannotCheck, Why: x.Why, Where: "catalog /Metadata"}
	}
	items := 0
	for _, alt := range x.LangAlts {
		items += len(alt)
	}
	if items == 0 {
		return Result{
			Verdict: NotApplicable,
			Why:     "the metadata packet holds no language-alternative text, so it has no natural language to determine",
		}
	}
	if catalogDeclaresLang(d) {
		return Result{Verdict: Pass}
	}
	// **An Alt is undetermined only when NO item in it names a language** (`/pending 489`). This rule
	// failed on the first `x-default` item, and veraPDF's own corpus has 16 passing files whose one Alt
	// holds `x-default` AND a real language such as `en-US` — every one a false Fail. veraPDF's test is
	// `xDefault == false || gContainsCatalogLang == true`; `xDefault` ships only as compiled code, so its
	// meaning is inferred from those files, and it agrees with P07.S05's own two measurements (`x-default`
	// alone fails, `xml:lang="en"` passes). The corpus run in veracorpus_test.go holds the inference.
	for _, alt := range x.LangAlts {
		if len(alt) == 0 {
			continue
		}
		determined := false
		for _, lang := range alt {
			if lang != "" && !strings.EqualFold(lang, "x-default") {
				determined = true
				break
			}
		}
		if !determined {
			return Result{
				Verdict: Fail,
				Why: fmt.Sprintf("a metadata text alternative offers only xml:lang %q, which names no language, "+
					"and the catalog declares no /Lang to supply one", alt[0]),
				Where: "catalog /Metadata, rdf:Alt",
			}
		}
	}
	return Result{Verdict: Pass}
}

// checkContentLanguage evaluates ua1 7.2 t34 — with the tree walk P07.S02 deferred to this slice.
//
// With a catalog `/Lang`, every piece of text has a language and the clause passes. Without one, each
// text-showing operator is resolved individually:
//
//   - inside an `/Artifact` sequence — not text a reader is given, so it needs no language;
//   - inside a marked-content sequence whose property list declares `/Lang` — that is its language;
//   - inside an MCID sequence — the MCID resolves through the parent tree to an element, and the
//     language is that element's `/Lang` or its nearest ancestor's (ISO 32000-1 §14.9.2);
//   - inside none of those — nothing could declare its language.
//
// **A marked-content `/Lang` counts** (`/pending 489`). This rule used to skip it, citing a P06.S04
// measurement that its section never recorded. veraPDF's test is `gContainsCatalogLang == true ||
// Lang != null` on each text item, and veraPDF passes 7.2 t34 on its own corpus file
// `7.2-t34-pass-c.pdf`, whose text declares its language only that way — measured 2026-09-14, and held
// by veracorpus_test.go.
func checkContentLanguage(d *Document) Result {
	// A declared catalog /Lang determines every piece of text — present counts, even empty: veraPDF's
	// test is `gContainsCatalogLang`, and an empty value is 7.2 t29's failure, which nib does not check.
	if catalogDeclaresLang(d) {
		return Result{Verdict: Pass}
	}
	events, errWhy := d.contentEvents()
	if errWhy != "" {
		return Result{Verdict: CannotCheck, Why: errWhy}
	}
	texts := 0
	for _, ev := range events {
		// Appearance streams are outside "page content" for this clause, as they are for 7.1 t3.
		if !ev.text || ev.artifact || ev.appearance {
			continue
		}
		texts++
		if ev.lang {
			continue // the enclosing sequence's own /Lang determines this text's language
		}
		if ev.mcid < 0 {
			return Result{
				Verdict: Fail,
				Why: "the catalog declares no /Lang and this text is in no tagged sequence, so " +
					"nothing could declare its language",
				Where: ev.where,
			}
		}
		elem := d.elementForMCID(ev.spKey, ev.mcid)
		if elem == nil {
			return Result{
				Verdict: Fail,
				Why: fmt.Sprintf("the catalog declares no /Lang and MCID %d resolves to no structure "+
					"element, so no element could declare this text's language", ev.mcid),
				Where: ev.where,
			}
		}
		if !d.declaresLangFor(elem) {
			return Result{
				Verdict: Fail,
				Why: "the catalog declares no /Lang and neither the element describing this text " +
					"nor any of its ancestors declares one",
				Where: ev.where,
			}
		}
	}
	if texts == 0 {
		return Result{Verdict: NotApplicable, Why: "the document shows no text that a reader is given"}
	}
	return Result{Verdict: Pass}
}

// elementForMCID resolves an MCID on the stream whose /StructParents key is spKey to its element.
func (d *Document) elementForMCID(spKey, mcid int) types.Dict {
	if spKey < 0 {
		return nil
	}
	arr, err := d.Ctx.DereferenceArray(d.parentTree()[spKey])
	if err != nil || mcid < 0 || mcid >= len(arr) {
		return nil
	}
	return d.dict(arr[mcid])
}

// checkUAIdentification evaluates ua1 5 t1.
//
// **No nib document reaches `Pass` here, and that is decided rather than pending a slice.** Writing
// `pdfuaid:part` claims conformance to all of PDF/UA, and P07.S07 measured that nib's checker cannot
// support that claim: it implements 18 of the 106 rules veraPDF evaluates (15 when that was measured),
// and a document can pass all of them while failing one it does not check. So nib never writes the identification (ADR-031 law 1), and
// every nib document with a metadata packet fails this clause — which is what veraPDF says about them.
// A document from another producer that carries the identification can still pass it. How nib ever
// honestly could write it is `/pending 486`.
func checkUAIdentification(d *Document) Result {
	x := readXMP(d)
	if !x.Present {
		return Result{
			Verdict: NotApplicable,
			Why: "the document has no metadata stream, so the identification schema has nowhere to " +
				"live and the clause has no subject (7.1 t8 reports the missing stream)",
		}
	}
	if !x.Readable {
		return Result{Verdict: CannotCheck, Why: x.Why, Where: "catalog /Metadata"}
	}
	if x.UAPart == "" {
		return Result{
			Verdict: Fail,
			Why: "the metadata packet carries no pdfuaid:part, so nothing in the document states " +
				"which part of PDF/UA it claims to conform to",
			Where: "catalog /Metadata, pdfuaid:part",
		}
	}
	// **The part's VALUE is not this clause** (`/pending 489`). This rule also failed a packet whose part
	// is not "1", and veraPDF's corpus file `5-t02-fail-a.pdf` (pdfuaid:part "2") showed veraPDF passes
	// 5 t1 there and fails 5 t2: its 5 t1 test is `containsPDFUAIdentification == true` and its 5 t2 test
	// `part == 1`. So the value moved to its own clause, `checkUAPartValue`, where the report still names
	// it — a wrong identification stays distinguishable from a missing one, under the clause veraPDF uses.
	return Result{Verdict: Pass}
}

// checkUAPartValue evaluates ua1 5 t2: `pdfuaid:part` is 1 (`/pending 489`).
//
// veraPDF's subject is the identification itself (`PDFUAIdentification`), so a packet that carries none
// has no subject here — 5 t1 reports the absence — and only a present identification can pass or fail.
func checkUAPartValue(d *Document) Result {
	x := readXMP(d)
	if !x.Present {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document has no metadata stream, so there is no identification whose part could be checked",
		}
	}
	if !x.Readable {
		return Result{Verdict: CannotCheck, Why: x.Why, Where: "catalog /Metadata"}
	}
	if x.UAPart == "" {
		return Result{
			Verdict: NotApplicable,
			Why:     "the metadata packet carries no pdfuaid:part, so there is no value to check (5 t1 reports the absence)",
		}
	}
	if x.UAPart != "1" {
		return Result{
			Verdict: Fail,
			Why:     fmt.Sprintf("pdfuaid:part is %q; a PDF/UA-1 file declares part 1", x.UAPart),
			Where:   "catalog /Metadata, pdfuaid:part",
		}
	}
	return Result{Verdict: Pass}
}

// checkOptionalContentName evaluates ua1 7.10 t1, over EVERY configuration dictionary.
//
// The clause says *"each optional content configuration dictionary"*, so the population is the
// default one under `/D` and every alternate under `/Configs` — the same population
// `honestOptionalContent` corrects, and for the same reason: a rule reaching one site of two is
// ADR-009's defect inside a checker.
func checkOptionalContentName(d *Document) Result {
	configs, res := optionalContentConfigs(d)
	if res != nil {
		return *res
	}
	for _, c := range configs {
		n, isStr := c.dict["Name"].(types.StringLiteral)
		if !isStr || len(n) == 0 {
			return Result{
				Verdict: Fail,
				Why: "an optional-content configuration dictionary has no /Name, or an empty one. " +
					"pdfcpu writes it that way for every stamp; nib corrects it at the stamping door",
				Where: c.where,
			}
		}
	}
	return Result{Verdict: Pass}
}

// checkOptionalContentAutoState evaluates ua1 7.10 t2.
//
// `/AS` is forbidden in a configuration dictionary by name. `/pending 473` measured that removing it
// is safe here — the written configuration names its group in `/ON`, carries no `/OFF` and no
// `/BaseState`, and all three `/AS` entries set that same group ON — and settled it with pixels:
// 4000 red pixels with the key and 4000 without.
func checkOptionalContentAutoState(d *Document) Result {
	configs, res := optionalContentConfigs(d)
	if res != nil {
		return *res
	}
	for _, c := range configs {
		if _, has := c.dict["AS"]; has {
			return Result{
				Verdict: Fail,
				Why:     "an optional-content configuration dictionary carries /AS, which this clause forbids by name",
				Where:   c.where,
			}
		}
	}
	return Result{Verdict: Pass}
}

// ocConfig is one optional-content configuration dictionary and where it was found, so a failure can
// say which of several it was.
type ocConfig struct {
	dict  types.Dict
	where string
}

// optionalContentConfigs returns every configuration dictionary, or a Result when there is nothing
// for the two 7.10 rules to inspect.
//
// A document with no optional content has no configuration dictionary, so both clauses have no
// subject — `NotApplicable`, not a pass. Most documents nib touches are in this state, and rating
// them `Pass` would make the two rules look exercised across a corpus where they were never asked.
func optionalContentConfigs(d *Document) ([]ocConfig, *Result) {
	ocp := d.dict(d.Catalog["OCProperties"])
	if ocp == nil {
		return nil, &Result{
			Verdict: NotApplicable,
			Why:     "the document declares no optional content, so it has no configuration dictionary",
		}
	}
	var out []ocConfig
	if def := d.dict(ocp["D"]); def != nil {
		out = append(out, ocConfig{dict: def, where: "catalog /OCProperties /D"})
	}
	if arr, err := d.Ctx.DereferenceArray(ocp["Configs"]); err == nil {
		for i, c := range arr {
			if cd := d.dict(c); cd != nil {
				out = append(out, ocConfig{dict: cd, where: fmt.Sprintf("catalog /OCProperties /Configs[%d]", i)})
			}
		}
	}
	if len(out) == 0 {
		return nil, &Result{
			Verdict: Fail,
			Why: "the document declares /OCProperties with no configuration dictionary in it, so " +
				"the optional content it names has no default state",
			Where: "catalog /OCProperties",
		}
	}
	return out, nil
}
