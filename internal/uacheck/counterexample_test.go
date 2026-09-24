package uacheck

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestADocumentCanPassEveryClauseNibChecksAndStillFailVeraPDF — the counterexample the docs cite, standing.
//
// The README, `nib ua`'s comment and the door's comment all say a document can pass every clause nib
// checks and still fail PDF/UA, and each named an example. **Two examples have now gone stale by being
// CAUGHT**, which is the healthy direction: a skipped heading level stopped being one when `/pending 487`
// taught the checker 7.4.2 t1, and a `/Formula` with no alternate text stopped being one when P06.S04
// taught it 7.7 t1. Each time this test went red and said to find a new one, which is what it is for.
//
// # The current example, and why it is a corpus document rather than a built one
//
// `7.21 Fonts/7.21.5 Font metrics/7.21.5-t01-fail-a.pdf`: a font whose glyph widths in the PDF disagree
// with the widths in the embedded font program. Measured — veraPDF fails it on `7.21.5 t1` and NOTHING
// else, and nib reports it conformant across all its clauses.
//
// **This is weaker than the two examples before it in one specific way, and that is stated rather than
// glossed**: they were built from nib's own doors, so the test ran on a fresh clone. This one needs
// veraPDF's corpus, so without it the test SKIPS and the docs' claim is unverified in that run. The
// reason is NOT that no buildable example exists — an earlier draft of this comment said the sixteen
// remaining clauses are all font-program rules about bytes nib does not write, and that was **false**:
// `7.20 t2` is among them, and `pdfops`'s own tests record nib producing a document that fails it (one
// marked-content form painted twice has two semantic parents). The reason is that `7.20 t2` is the very
// next slice, **P06.S05**, so an example built on it would be retired again within the week — and the
// whole point of this test is that its example survives until a rule catches it.
//
// **A mutation was tried and rejected, and the rejection is the more useful record.** Rewriting a
// `/ToUnicode` destination to `<0000>` in nib's own output produces a document nib calls conformant and
// veraPDF fails — but it fails `7.21.7 t1`, a clause nib IMPLEMENTS. So it is not a counterexample at
// all, it is a live FALSE PASS, filed critical as `/pending 657`. Building the docs' central honesty
// claim on a checker bug would be the worst of both, and the per-clause assertion at the end of this
// test is what makes that impossible to do by accident again.
func TestADocumentCanPassEveryClauseNibChecksAndStillFailVeraPDF(t *testing.T) {
	const rel = "7.21 Fonts/7.21.5 Font metrics/7.21.5-t01-fail-a.pdf"
	path := filepath.Join(corpusDir(), rel)
	pdf, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("SKIP (unchecked): veraPDF's corpus is absent, so the documented counterexample cannot "+
			"be exercised in this run: %v", err)
	}
	r, cerr := Check(pdf)
	if cerr != nil {
		t.Fatalf("nib cannot read the documented counterexample: %v", cerr)
	}
	if !r.Conformant() {
		var not []string
		for _, res := range r.Results {
			if res.Verdict != Pass && res.Verdict != NotApplicable {
				not = append(not, res.Clause+" ("+res.Why+")")
			}
		}
		t.Fatalf("nib now reports the documented counterexample as NOT conformant (%v) — the checker "+
			"catches it, so the README, cmdUA's and door.go's comments cite an example that is no "+
			"longer one: find a new one", not)
	}

	vp := veraPDFPath()
	if vp == "" {
		t.Skip("SKIP (half checked): nib's side holds; veraPDF is absent, so that the example FAILS PDF/UA is unchecked in this run")
	}
	out, _ := exec.Command(vp, "--flavour", "ua1", "--passed", path).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v", err)
	}
	states := veraStates(rep, []string{path})
	if states[0] == nil || states[0]["7.21.5 t1"] != veraFailed {
		t.Errorf("veraPDF does not fail 7.21.5 t1 on the documented counterexample (state %q) — the "+
			"docs' example no longer shows a nib-conformant document failing PDF/UA", states[0]["7.21.5 t1"])
	}
	// **The example must fail ONLY a clause nib does not implement.** Otherwise it is a false pass in
	// nib rather than a limit of its coverage, and the docs' sentence would be resting on a bug.
	implemented := map[string]bool{}
	for _, c := range Clauses() {
		implemented[c] = true
	}
	for clause, st := range states[0] {
		if st == veraFailed && implemented[clause] {
			t.Errorf("veraPDF fails %s on the counterexample and nib IMPLEMENTS that clause, so nib "+
				"passing the document is a false pass, not a coverage limit", clause)
		}
	}
}
