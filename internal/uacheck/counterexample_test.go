package uacheck

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestNoCorpusDocumentNibCallsConformantFailsVeraPDF — the docs' claim about a clean report, standing.
//
// For most of this checker's life the README, `nib ua`'s comment and the door's comment each cited a document that
// passed every clause nib checks and still failed veraPDF, and a test here held that document up and went red the day
// a rule caught it. **Three were retired that way** — a skipped heading level (`/pending 487`), a `/Formula` with no
// alternate text (P06.S04), and a font whose glyph widths disagree with its own program (P07.S04a, which taught the
// checker 7.21.5 t1). After the third, none was left to cite, and this test asserts what the docs now say instead.
//
// **Measured at P07.S04a over veraPDF's own PDF/UA-1 corpus: nib reports 131 files conformant, and veraPDF fails none
// of them.** The two rules nib does not check cannot produce such a file today, for different reasons: 7.1 t12 is one
// veraPDF never fails (P03), and 7.21.4.2 t1 — a Type 1 font's /CharSet against its program — applies only to an
// EMBEDDED Type 1 or CFF font, whose metrics nib reports as "could not check" until P07.S05/S06 read those programs.
// That second reason is a refusal, not a check, which is why the corpus's 7.21.4.2 t01 files are asserted here as NOT
// conformant rather than as agreeing: the day S05/S06 lands, those refusals become verdicts and the hole reopens
// unless 7.21.4.2 t1 lands with them.
//
// **This is still not a certificate**, and the docs keep saying so: nib's verdicts are tested against veraPDF on every
// build, not proven, and a corpus is evidence about the documents in it.
func TestNoCorpusDocumentNibCallsConformantFailsVeraPDF(t *testing.T) {
	root := corpusDir()
	if _, err := os.Stat(root); err != nil {
		t.Skipf("SKIP (unchecked): veraPDF's corpus is absent, so the docs' claim cannot be exercised in this run: %v", err)
	}
	var conformant []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(p) != ".pdf" {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if r, cerr := Check(b); cerr == nil && r.Conformant() {
			conformant = append(conformant, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// **The count is the README's**, so a set that shrank would leave the docs' figure standing over fewer files.
	const readmeCount = 131
	if len(conformant) != readmeCount {
		t.Errorf("nib calls %d corpus files conformant and the README says %d — measure it again and move both", len(conformant), readmeCount)
	}
	for _, f := range []string{"7.21.4.2-t01-fail-a.pdf", "7.21.4.2-t01-fail-b.pdf"} {
		p := filepath.Join(root, "7.21 Fonts", "7.21.4 Embedding", "7.21.4.2 Subset embedding", f)
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("the corpus lacks %s: %v", f, rerr)
		}
		if r, cerr := Check(b); cerr == nil && r.Conformant() {
			t.Errorf("%s fails 7.21.4.2 t1, which nib does not check, and nib now calls it conformant — the docs' claim "+
				"that a clean report has no measured counterexample is false; cite this file, or land 7.21.4.2 t1", f)
		}
	}
	vp := veraPDFPath()
	if vp == "" {
		t.Skipf("SKIP (half checked): nib calls %d corpus files conformant; veraPDF is absent, so that it fails none is unchecked", len(conformant))
	}
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1"}, conformant...)...).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v", err)
	}
	if len(rep.Jobs) != len(conformant) {
		t.Fatalf("veraPDF reported %d jobs for %d files", len(rep.Jobs), len(conformant))
	}
	for _, j := range rep.Jobs {
		// A job veraPDF did not finish lists no rules, which would read as "fails none".
		if j.Report.Status != "normal" {
			t.Errorf("veraPDF did not finish %s (jobEndStatus %q), so whether it fails that file is unchecked", j.Item.Name, j.Report.Status)
		}
		for _, r := range j.Report.Rules {
			if r.Status == "failed" {
				t.Errorf("nib calls %s conformant and veraPDF fails %s t%s — a counterexample the docs must cite (or a false "+
					"pass, if nib implements that clause)", j.Item.Name, r.Clause, r.Test)
			}
		}
	}
	t.Logf("nib calls %d corpus files conformant; veraPDF fails none of them", len(conformant))
}
