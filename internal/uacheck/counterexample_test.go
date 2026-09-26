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
// **Measured at P07.S05b over veraPDF's own PDF/UA-1 corpus: nib reports 140 files conformant, and veraPDF fails none
// of them** (131 at P07.S04a; 132 at P07.S05a, when `7.21.4.2-t01-pass-a`'s Type1C program became readable; eight more
// at P07.S05b, whose CIDFontType0C programs did). The one rule nib
// does not check, 7.1 t12, is one veraPDF never fails (P03). 7.21.4.2 t1 — a Type 1 font's /CharSet against its
// program — landed WITH the Type1C metrics, as this test demanded: the corpus's two t01 fail files are asserted
// non-conformant below, so a regression in that clause shows here as well as in the oracle — and so is
// `7.21.8-t01-fail-a`, the .notdef glyph drawn in a CID-keyed CFF font, the file P07.S05b's acceptance names. A Type 1
// (/FontFile) program is read since P07.S06; veraPDF's corpus holds none, so its evidence is `type1Fixtures`.
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
	const readmeCount = 140
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
			t.Errorf("%s fails 7.21.4.2 t1, and nib calls it conformant — a false pass in the clause P07.S05a landed", f)
		}
	}
	notdef := filepath.Join(root, "7.21 Fonts", "7.21.8 Use of .notdef glyph", "7.21.8-t01-fail-a.pdf")
	if b, rerr := os.ReadFile(notdef); rerr != nil {
		t.Fatalf("the corpus lacks %s: %v", notdef, rerr)
	} else if r, cerr := Check(b); cerr != nil || r.Conformant() || verdictIn(t, r, "7.21.8 t1") != Fail {
		t.Errorf("7.21.8-t01-fail-a draws .notdef in a CID-keyed CFF font, and nib does not fail 7.21.8 t1 there — P07.S05b's acceptance")
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
