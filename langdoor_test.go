package nib

import (
	"fmt"
	"sort"
	"testing"
)

// TestEveryAuthoringDoorSaysWhereItsLanguageComesFrom — `PLAN-accessibility.md` P03.S02.
//
// # What this guard is, and what it deliberately is not
//
// The slice was planned as *"every authoring operation sets `/Lang`"*, and measurement overturned
// that before a line was written. `SetLang`'s single caller is not a one-door defect: it is the one
// place nib has ever been **told** a language, by a user choosing what to OCR. Every other door
// either already carries a better determination or has none available to it:
//
//   - the office conversion path carries `en-US` out of LibreOffice, derived from the source
//     document, and nib's pipeline passes it through unchanged;
//   - the markdown path renders the user's own text, whose language nib was never told;
//   - `ImagesToPDF` output has **no text in page content at all**, so the clause never fires;
//   - the two `p2p` doors know their language exactly — nib's own English prose — and writing it is
//     inert, because `Append` discards a fragment's catalog.
//
// So this guard does not demand a write. It demands that every authoring door has been **asked**
// where its language comes from, and that the answer is recorded where the next person will find
// it. Writing `"en"` on a document nib did not write the text of is a false statement about that
// document — ADR-031 law 1's principle in a different field — and an absent `/Lang` is silence.
//
// # The one claim it verifies rather than records
//
// A row claiming `declares` must actually reach `pdfops.SetLang`, and a row claiming anything else
// must not. Without that cross-check this is a comment table, and a comment table is exactly what a
// guard is supposed to replace: the classification would keep passing after the code stopped
// matching it.
func TestEveryAuthoringDoorSaysWhereItsLanguageComesFrom(t *testing.T) {
	const (
		declares = "declares" // routes through pdfops.SetLang — verified against the code
		told     = "told"     // reaches pdfops.SetLang when the request carries a language, and otherwise carries
		carries  = "carries"  // the primitive's own output already has one, from something that knew
		none     = "none"     // no determination exists; the gap is named
		fragment = "fragment" // the catalog is discarded before anyone receives the document
	)
	type row struct{ class, why string }

	// Keyed `<file>:<enclosing function>`, the same keys the title guard uses.
	where := map[string]row{
		"internal/pdfops/importraster.go:page": {none, "an image has no text, so there is no " +
			"natural language to determine. The door builds a page whose whole content is one image " +
			"XObject — no font, no show operator, nothing a /Lang would qualify. Declaring a language " +
			"here would be inventing one, which is what `none` exists to say. **If OCR is ever folded " +
			"into this route** the classification changes with it, because the recognised text WOULD " +
			"have a language and `pdfops.SetLang` is where it would be declared (/pending 400, 539)."},
		"internal/pdfops/importdoor.go:convertImport": {told, "the ONE conversion door (ADR-185, D13): every route — the " +
			"four opens, Import…, `nib office` — reaches ConvertDocToPDFWith here, and the Document language " +
			"field (/pending 471) reaches pdfops.SetLang here when the request carries a language. The server's " +
			"automatic open and Combine send none, so they declare nothing and an office conversion keeps " +
			"LibreOffice's /Lang, which is the CONVERTING MACHINE's locale, not the document's (measured: DOCX " +
			"declaring de-DE, th-TH and fr-FR and an ODT declaring fo:language=de all converted to the machine's " +
			"locale); nib passes it through on purpose — stripping it would move the common case, where author " +
			"and machine share a language, from one failing ua1 clause to three. The PDF/UA label is Markdown-only " +
			"and only for a CHOSEN language (ADR-033). Held upright by TestAConvertersLanguageDoesNotComeFromTheDocument " +
			"and TestNibDoesNotReplaceAConvertersLanguage. This row replaces the handleOffice and cmdOffice rows: " +
			"those doors no longer author, they call Import."},
		"internal/pdfops/office.go:ConvertDocToPDF": {none, "a one-line forward to ConvertDocToPDFWith with no options. " +
			"The language question belongs to its caller, `convertImport`, classified above; a wrapper that declared one would answer it twice."},
		"internal/server/export.go:handleAssemble": {none, "raster pages with NO text in page " +
			"content, so ua1 7.2 t34 does not fire on this output at all (measured 2026-09-11: a " +
			"titled raster document fails 5 t1, 6.2 t1, 7.1 t3 and 7.1 t11, and neither 7.2 " +
			"clause). The only language-bearing content is the title, taken from the user's file " +
			"name, whose language nib does not know."},
		"internal/p2p/readme.go:RenderReadme": {fragment, "Append discards a fragment's catalog, " +
			"so a /Lang written here reaches nobody — the same measurement that made its title " +
			"inert at v1.129.32. The REAL defect on this path is that nib's English prose is " +
			"stapled into a document whose /Lang may say something else, which no catalog key can " +
			"fix: it needs a language on the CONTENT. Since PLAN-ua-coverage.md P02.S09 this text is " +
			"ALSO wrapped in structure elements (pdfops.TagAuthoredPages, after the declaration), " +
			"and the content /Span stays the language carrier."},
		"internal/p2p/sigpages.go:renderPage": {fragment, "the same, for every signature page."},
		"internal/pdfops/pdfops.go:RedactPages": {fragment, "a one-page raster intermediate, " +
			"re-merged three lines later."},
	}

	sites, declaredIn, calledOutside := scanAuthoringSites(t)
	assertScanIsNotVacuous(t, declaredIn, calledOutside)

	hit := map[string]bool{}
	var unclassified, contradicted []string
	for _, s := range sites {
		r, ok := where[s.key]
		if !ok {
			unclassified = append(unclassified, fmt.Sprintf("%s calls %s", s.pos, s.ctor))
			continue
		}
		hit[s.key] = true
		switch {
		case (r.class == declares || r.class == told) && !s.calls["SetLang"]:
			contradicted = append(contradicted, fmt.Sprintf(
				"%s is classified %q and never reaches pdfops.SetLang", s.pos, r.class))
		case r.class != declares && r.class != told && s.calls["SetLang"]:
			contradicted = append(contradicted, fmt.Sprintf(
				"%s is classified %q and DOES reach pdfops.SetLang — the row is describing code "+
					"that no longer exists", s.pos, r.class))
		}
		if r.why == "" {
			contradicted = append(contradicted, fmt.Sprintf("%s carries no reason", s.pos))
		}
	}
	sort.Strings(unclassified)
	sort.Strings(contradicted)

	for _, u := range unclassified {
		t.Errorf("authoring door with no language classification: %s\n\tSay where this document's "+
			"language comes from by adding a row to `where` in this file: %q if it routes through "+
			"pdfops.SetLang, %q if it does so only when the user names a language, %q if its own "+
			"output already carries one, %q if nobody can determine it, %q if the catalog never "+
			"reaches a reader. A reason is required in every case.",
			u, declares, told, carries, none, fragment)
	}
	for _, c := range contradicted {
		t.Errorf("a language classification the code contradicts: %s", c)
	}
	for key := range where {
		if !hit[key] {
			t.Errorf("stale language classification %q: no authoring call site is attributed to "+
				"it. The site moved, was renamed, or is gone — remove the row.", key)
		}
	}
}
