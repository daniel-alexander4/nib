package uacheck

import (
	"fmt"

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

// checkMetadataLanguage evaluates ua1 7.2 t33.
//
// **Only the catalog `/Lang` determines it, and that is measured.** P06's close recorded that a
// tagged document with an XMP packet and no catalog `/Lang` fails this clause, and that adding the
// key clears it — nib's own `x-default` on `dc:title` does not satisfy it.
func checkMetadataLanguage(d *Document) Result {
	x := readXMP(d)
	if !x.Present {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document has no metadata stream, so there is no metadata whose language could be determined",
		}
	}
	if lang := catalogLang(d); lang != "" {
		return Result{Verdict: Pass}
	}
	return Result{
		Verdict: Fail,
		Why: "the catalog declares no /Lang, so the language of the document's metadata cannot be " +
			"determined. Measured at P06: nib's own `x-default` on dc:title does not satisfy this " +
			"clause and the catalog key does",
		Where: "catalog",
	}
}

// checkContentLanguage evaluates ua1 7.2 t34 — and is the rule that cannot be answered two ways.
//
// # Why an untagged document is a provable Fail and a tagged one is not
//
// A structure element may carry its own `/Lang`, covering the content beneath it. So with no catalog
// `/Lang`:
//
//   - **no structure tree** — nothing in the document could declare a language for its content, and
//     the failure is established rather than suspected.
//   - **a structure tree** — some or all of that content may be covered by an element's `/Lang`, and
//     deciding it means walking the tree, which is P07.S03's. Reporting `Fail` here would be
//     claiming a breach nib has not established: the exact mirror of the pass law 4 forbids, and the
//     reason law 4's third verdict is not decoration.
//
// P06.S04 measured the other half of this: a `/Lang` on the CONTENT via `/Span <</Lang (en)>> BDC`
// does NOT satisfy the clause, so marked content is not a route to a pass here.
func checkContentLanguage(d *Document) Result {
	if lang := catalogLang(d); lang != "" {
		return Result{Verdict: Pass}
	}
	if _, hasTree := d.Catalog["StructTreeRoot"]; !hasTree {
		return Result{
			Verdict: Fail,
			Why: "the catalog declares no /Lang and the document has no structure tree, so nothing " +
				"in it could declare a language for its page content",
			Where: "catalog",
		}
	}
	return Result{
		Verdict: CannotCheck,
		Why: "the catalog declares no /Lang, and whether every piece of content is covered by a " +
			"structure element that declares one is not evaluated yet (P07.S03). Reporting a " +
			"failure here would claim a breach that has not been established",
		Where: "catalog",
	}
}

// checkUAIdentification evaluates ua1 5 t1.
//
// **This rule cannot reach `Pass` until P07.S07, by design.** Writing `pdfuaid:part` is a claim that
// the document conforms to PDF/UA, and ADR-031's law 1 forbids a conformance assertion nib cannot
// support — so P03.S01 refused to write it and P07.S07 is where the checker's own verdict becomes
// the thing that authorises it. Until then every nib document with a metadata packet fails this
// clause, which is exactly what veraPDF says about them and exactly what the plan recorded.
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
	if x.UAPart != "1" {
		return Result{
			Verdict: Fail,
			Why:     fmt.Sprintf("pdfuaid:part is %q; nib checks against PDF/UA-1", x.UAPart),
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
