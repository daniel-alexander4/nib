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
		told     = "told"     // reaches pdfops.SetLang only when the user names a language, and otherwise carries
		carries  = "carries"  // the primitive's own output already has one, from something that knew
		none     = "none"     // no determination exists; the gap is named
		fragment = "fragment" // the catalog is discarded before anyone receives the document
	)
	type row struct{ class, why string }

	// Keyed `<file>:<enclosing function>`, the same keys the title guard uses.
	where := map[string]row{
		"internal/server/office.go:handleOffice": {carries, "an office conversion comes out of " +
			"LibreOffice already carrying a /Lang, and nib passes it through. **It is NOT derived " +
			"from the source document** — that was this slice's first answer and measurement " +
			"refuted it: three DOCX files declaring w:lang de-DE, th-TH and fr-FR, and an ODT " +
			"declaring fo:language=de in LibreOffice's OWN native format, all converted to " +
			"/Lang=(en-US) on an en_US machine, and the same ODT under LANG=de_DE.UTF-8 converted " +
			"to /Lang=(de-DE). The value is the CONVERTING MACHINE's locale. nib passes it " +
			"through anyway, and that is a decision: stripping it would take a correct " +
			"declaration off every document whose author and machine share a language — the " +
			"common case — to avoid a wrong one where they do not, and would move this path from " +
			"one failing ua1 clause to three. Correcting it needs someone who knows, which is the " +
			"user, and that is the /pending item this slice files. Held upright by " +
			"TestAConvertersLanguageDoesNotComeFromTheDocument and " +
			"TestNibDoesNotReplaceAConvertersLanguage. The markdown branch of the same door gets " +
			"no /Lang from anywhere — mdpdf is pure Go and was never told one."},
		"internal/cli/commands.go:cmdOffice": {told, "the CLI half of the same door. `--lang` is " +
			"the user saying what language the document is in, so it is declared through " +
			"pdfops.SetLang (whose LangTag door refuses anything it cannot declare); with no " +
			"--lang it carries, for handleOffice's reason. /pending 471. Named separately because " +
			"ADR-009 polices SITES, not packages."},
		"internal/server/export.go:handleAssemble": {none, "raster pages with NO text in page " +
			"content, so ua1 7.2 t34 does not fire on this output at all (measured 2026-09-11: a " +
			"titled raster document fails 5 t1, 6.2 t1, 7.1 t3 and 7.1 t11, and neither 7.2 " +
			"clause). The only language-bearing content is the title, taken from the user's file " +
			"name, whose language nib does not know."},
		"internal/p2p/readme.go:RenderReadme": {fragment, "Append discards a fragment's catalog, " +
			"so a /Lang written here reaches nobody — the same measurement that made its title " +
			"inert at v1.129.32. The REAL defect on this path is that nib's English prose is " +
			"stapled into a document whose /Lang may say something else, which no catalog key can " +
			"fix: it needs a language on the CONTENT, and P05 wraps this exact text in structure " +
			"elements anyway."},
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
