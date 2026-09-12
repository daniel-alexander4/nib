package nib

import (
	"fmt"
	"sort"
	"testing"
)

// TestEveryAuthoredDocumentGetsATitle — `PLAN-accessibility.md` P03.S01.T03.
//
// # The rule
//
// Nib authors PDFs from three primitives — `CreateFromJSON`, `ImagesToPDF` and `ConvertDocToPDF`.
// Every document nib authors and then HANDS TO SOMEONE must carry the catalog floor (PDF/UA 7.1 t8,
// t9, t10), and the one door onto that floor is `pdfops.SetTitle`. So: every call site of an
// authoring primitive either routes through the door, or is NAMED here with the reason it is a
// fragment rather than a document.
//
// # Why it is written this way, and not as eight copies of a check
//
// ADR-009: a rule holding at more than one call site is written once and the guard asserts the
// ROUTING, not the text each site prints. Checking that five sites agree says nothing about a sixth
// added without one — which is the failure this exists to make impossible. The exemption map is
// read in both directions so a row that stops matching a real site goes red rather than rotting.
//
// # Anti-vacuity
//
// `assertScanIsNotVacuous`, shared with the language guard — a scan that matches nothing passes
// silently, and the two ways that happens are a renamed primitive and one nothing calls.
//
// # Declared blind spot
//
// Routing is "the enclosing function also calls SetTitle", not dataflow: the guard does not prove
// the title is applied to THAT primitive's output. Proving it needs type-checked dataflow, which is
// a large instrument for a one-line mistake that the compiler and `title_test.go`'s behavioural
// tests both reach first. What this catches is the case those miss — a new authoring site that
// calls the door nowhere at all.
func TestEveryAuthoredDocumentGetsATitle(t *testing.T) {
	// The named exemptions, keyed `<file>:<enclosing function>`. Every row carries its reason.
	exempt := map[string]string{
		"internal/pdfops/pdfops.go:RedactPages": "a one-page raster FRAGMENT, re-merged into the " +
			"document being redacted and never handed to anyone on its own. Titling it would put a " +
			"dc:title on an intermediate that is discarded three lines later, and the redacted " +
			"document keeps whatever title it arrived with.",
		"internal/p2p/readme.go:RenderReadme": "a FRAGMENT: its only production caller is " +
			"AppendReadme, which passes it to pdfops.Append as the SECOND argument. Append keeps " +
			"the first document's catalog wholesale — measured 2026-09-11, both when that document " +
			"has a title and when it has none — so a title written here is discarded on every path " +
			"that exists. The co-signed document's title is the user's document's own. Held " +
			"upright by TestAppendKeepsTheFirstDocumentsCatalog.",
		"internal/p2p/sigpages.go:renderPage": "a FRAGMENT, for the same measured reason as " +
			"RenderReadme: PrepareCeremonyDocument appends every signature page through " +
			"pdfops.Append, so their catalogs never reach the finished document.",
	}

	sites, declaredIn, calledOutside := scanAuthoringSites(t)
	assertScanIsNotVacuous(t, declaredIn, calledOutside)

	hit := map[string]bool{}
	var bad []string
	for _, s := range sites {
		// Either door counts. `TitleFromName` is the ordinary one — it derives the title from a
		// file name and guarantees a failed metadata write never costs the caller the document —
		// and `SetTitle` is the site that already knows its title and has made that call itself.
		if s.calls["TitleFromName"] || s.calls["SetTitle"] {
			continue
		}
		if _, ok := exempt[s.key]; ok {
			hit[s.key] = true
			continue
		}
		bad = append(bad, fmt.Sprintf("%s calls %s and never reaches pdfops.TitleFromName or pdfops.SetTitle", s.pos, s.ctor))
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("authored document with no title: %s\n\tEither route it through pdfops.TitleFromName (never costs the caller the document) or pdfops.SetTitle "+
			"with a title the caller knows, or add a row to `exempt` in this file naming the "+
			"reason it is a fragment rather than a document.", b)
	}
	for key := range exempt {
		if !hit[key] {
			t.Errorf("stale exemption %q: no un-titled authoring call site is attributed to it. "+
				"The site moved, was renamed, or now routes through the door — remove the row.", key)
		}
	}
}
