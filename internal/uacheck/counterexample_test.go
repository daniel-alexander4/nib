package uacheck

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
)

// TestADocumentCanPassEveryClauseNibChecksAndStillFailVeraPDF — the counterexample the docs cite, standing.
//
// The README, `nib ua`'s comment and the door's comment all say a document can pass every clause nib checks
// and still fail PDF/UA, and each named an example. The first, a skipped heading level, stopped being true
// when `/pending 487` taught the checker 7.4.2 t1 — and nothing noticed the sentence had gone stale. So the
// current example is built and asserted here: a tagged, titled, language-declared Markdown document with the
// identification, whose paragraph is retyped `/Formula` through the structure editor's door with no
// alternate text. nib's report is conformant (always asserted); veraPDF fails 7.7 t1 on it (asserted when
// veraPDF is present). If a future rule makes nib catch it, this test says the docs need a new example.
func TestADocumentCanPassEveryClauseNibChecksAndStillFailVeraPDF(t *testing.T) {
	md, err := pdfops.ConvertDocToPDF([]byte("# A heading\n\nA paragraph of body text.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	titled, err := pdfops.SetTitle(md, "A named document")
	if err != nil {
		t.Fatal(err)
	}
	withLang, err := pdfops.SetLang(titled, "en")
	if err != nil {
		t.Fatal(err)
	}
	base := withUAPart(t, withLang, "1")
	if r, err := Check(base); err != nil || !r.Conformant() {
		t.Fatalf("setup: the base document is not conformant by nib's own report (%v) — the example needs a base that passes everything", err)
	}
	tree, err := pdfops.ReadStructure(base)
	if err != nil {
		t.Fatal(err)
	}
	para := 0
	for _, e := range tree.Elements {
		if e.Standard == "P" && e.ID > 0 {
			para = e.ID
			break
		}
	}
	if para == 0 {
		t.Fatal("setup: no addressable paragraph to retype")
	}
	edited, err := pdfops.EditStructure(base, []pdfops.StructureEdit{{Kind: "retype", Element: para, Value: "Formula", Index: -1}})
	if err != nil {
		t.Fatal(err)
	}
	// Labelled AFTER the edit, the way a producer labels what it finishes: the structure editor is a change,
	// and a change drops an identification nib did not verify (ADR-032) — so the label written before it is
	// gone, and the example is a document that claims conformance at the moment it is handed over.
	formula := withUAPart(t, edited, "1")
	r, err := Check(formula)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Conformant() {
		var not []string
		for _, res := range r.Results {
			if res.Verdict != Pass && res.Verdict != NotApplicable {
				not = append(not, res.Clause)
			}
		}
		t.Fatalf("nib now reports the documented counterexample as NOT conformant (%v) — the checker catches it, "+
			"so the README, cmdUA's and door.go's comments cite an example that is no longer one: find a new one", not)
	}

	vp := veraPDFPath()
	if vp == "" {
		t.Skip("SKIP (half checked): nib's side holds; veraPDF is absent, so that the example FAILS PDF/UA is unchecked in this run")
	}
	p := filepath.Join(t.TempDir(), "formula.pdf")
	if err := os.WriteFile(p, formula, 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(vp, "--flavour", "ua1", "--passed", p).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v", err)
	}
	states := veraStates(rep, []string{p})
	if states[0] == nil || states[0]["7.7 t1"] != veraFailed {
		t.Errorf("veraPDF does not fail 7.7 t1 on the documented counterexample (state %q) — the docs' example "+
			"no longer shows a nib-conformant document failing PDF/UA", states[0]["7.7 t1"])
	}
}
