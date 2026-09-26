package uacheck

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The checker against veraPDF over REAL producers' files — PLAN-ua-coverage.md P08, decision D7.
//
// veraPDF's own corpus holds one construct per file and the oracle holds documents nib and LibreOffice wrote, so every
// agreement figure before this was a claim about synthetic input. Real producers are where the font family and the
// structure tree diverge. This test is law 5's STRICT judgment (`veraState.agrees`: Pass↔passed, Fail↔failed,
// NotApplicable↔no subject, CannotCheck permitted) over a directory of such files, with every disagreement named.
//
// **Where the corpus comes from.** `$NIB_UA_PRODUCERS`, else `~/nib/producers`, one directory per producer
// (`libreoffice/`, `ghostscript/`, `pdflatex/`, `word/`, `acrobat/` …). It is never committed — third-party documents
// carry their authors' rights — so an absent corpus or an absent veraPDF is a SKIP that says which, never a pass.
//
// **What is not asserted here, and why.** A refusal (`CannotCheck`) is permitted by law 1 and not required to be named,
// unlike the oracle's `knownCannotCheck`: this corpus is open-ended and grows. Reach per clause is LOGGED so a refusal
// that swallows a clause is visible; P08.S05's "agrees with veraPDF on N rules" figure is where it is read.

// producerDisagreements is every (file, clause) disagreement the corpus is known to hold, keyed
// "<producer>/<file> / <clause>" (or "/ open" for a file nib cannot read), each with the `/pending` item that owns it or
// the reason it stands. A row that stops disagreeing is an error: it is a claim about code that no longer behaves so.
var producerDisagreements = map[string]string{
	// Measured at P08.S02 over build/producers.sh's corpus (20 files, 5 producers); P08.S04 works each one.
	"libreoffice/form-untagged.pdf / 7.2 t30":   "/pending 674 — nib answers Pass on a page with no marked content where veraPDF has no subject; P08.S04",
	"libreoffice/form-untagged.pdf / 7.2 t31":   "/pending 674 — nib answers Pass on a page with no marked content where veraPDF has no subject; P08.S04",
	"libreoffice/form-untagged.pdf / 7.2 t32":   "/pending 674 — nib answers Pass on a page with no marked content where veraPDF has no subject; P08.S04",
	"libreoffice/writer-untagged.pdf / 7.2 t30": "/pending 674 — nib answers Pass on a page with no marked content where veraPDF has no subject; P08.S04",
	"libreoffice/writer-untagged.pdf / 7.2 t31": "/pending 674 — nib answers Pass on a page with no marked content where veraPDF has no subject; P08.S04",
	"libreoffice/writer-untagged.pdf / 7.2 t32": "/pending 674 — nib answers Pass on a page with no marked content where veraPDF has no subject; P08.S04",
	"ghostscript/pdflatex-article.pdf / 7.1 t9": "/pending 694 — a live false fail: nib fails a missing dc:title that veraPDF passes on Ghostscript's re-distil of pdfLaTeX output; P08.S04",
}

// refusedWhole is whether nib refused every clause of a document — the shape a recovered panic and `reportsNothing`
// both leave.
func refusedWhole(r Report) bool {
	if len(r.Results) == 0 {
		return false
	}
	for _, res := range r.Results {
		if res.Verdict != CannotCheck {
			return false
		}
	}
	return true
}

func producersDir() string {
	if d := os.Getenv("NIB_UA_PRODUCERS"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "nib", "producers")
}

// judgeProducers is the harness's whole judgment as a pure function, as `compareToOracle` is the oracle's: every
// disagreement in cmp or in unopened must be named in known, and every row in known must still disagree. Errors that are
// not disagreements (a lost job, a clause veraPDF does not list) are passed through: they are never excusable by name.
func judgeProducers(cmp oracleComparison, unopened map[string]string, known map[string]string) []string {
	var errs []string
	disagreeing := map[string]bool{}
	named := func(key, msg string) {
		disagreeing[key] = true
		if _, ok := known[key]; !ok {
			errs = append(errs, "DISAGREEMENT not named in producerDisagreements: "+msg)
		}
	}
	// Every error the comparison raises is either unexcusable or a keyed disagreement; one that is neither would be
	// dropped here without a signal, so the split is checked rather than trusted.
	if len(cmp.errors) != len(cmp.unexcusable)+len(cmp.disagree) {
		errs = append(errs, fmt.Sprintf("the comparison raised %d error(s) but split only %d unexcusable and %d disagreeing — "+
			"a branch of compareToOracle no longer feeds the split", len(cmp.errors), len(cmp.unexcusable), len(cmp.disagree)))
	}
	errs = append(errs, cmp.unexcusable...)
	for key, msg := range cmp.disagree {
		named(key, msg)
	}
	for key, msg := range unopened {
		named(key, msg)
	}
	for key := range known {
		if !disagreeing[key] {
			errs = append(errs, fmt.Sprintf("producerDisagreements has %q, which no longer disagrees — remove the row", key))
		}
	}
	sort.Strings(errs)
	return errs
}

func TestTheCheckerAgreesWithVeraPDFOnRealProducers(t *testing.T) {
	dir := producersDir()
	var files, names []string
	if dir != "" {
		// The root may itself be a link (a corpus kept elsewhere); WalkDir does not follow one, and would read an
		// existing corpus as absent.
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
			if err != nil {
				if p != dir { // an absent root is the SKIP below; anything unreadable inside it is a hole
					t.Errorf("the producer corpus could not be read at %s, so part of it would go unscored: %v", p, err)
				}
				return nil
			}
			if e.Type()&os.ModeSymlink != 0 {
				if st, serr := os.Stat(p); serr == nil && st.IsDir() {
					t.Errorf("%s is a linked directory, which the walk does not follow — its files would go unscored; "+
						"copy or link the files themselves", p)
					return nil
				}
			}
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(p), ".pdf") {
				rel, _ := filepath.Rel(dir, p)
				files = append(files, p)
				names = append(names, filepath.ToSlash(rel))
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Skip("SKIP (not a pass): the real-producer corpus is absent, so D7's agreement is UNCHECKED. Set " +
			"NIB_UA_PRODUCERS, or build it into ~/nib/producers (PLAN-ua-coverage.md P08.S02/S03).")
	}
	vp := veraPDFPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the real-producer run is UNCHECKED. Set NIB_VERAPDF, put " +
			"verapdf on PATH, or install to ~/verapdf.")
	}
	vera := veraPDFBatch(t, vp, files)
	reports := make([]*Report, len(files))
	unopened := map[string]string{}
	perProducer := map[string]int{}
	for i, f := range files {
		perProducer[strings.SplitN(names[i], "/", 2)[0]]++
		// **A job with no rules is veraPDF not reporting on the file** (it could not parse or validate it), and every
		// clause would then read as "not listed" — an error no row could name. It is one nameable fact about the file.
		if vera[i] != nil && len(vera[i]) == 0 {
			unopened[names[i]+" / veraPDF"] = fmt.Sprintf("%s: veraPDF returned a job with no rules — it did not validate the file", names[i])
			continue // never scored clause by clause; its empty job is not a lost one, which stays nil and is reported
		}
		pdf, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		nr, err := Check(pdf)
		if err != nil {
			unopened[names[i]+" / open"] = fmt.Sprintf("%s: nib cannot open a file veraPDF reported on: %v", names[i], err)
			continue
		}
		// **A whole-document refusal is not agreement.** `agrees` permits a refusal per clause, which is law 1; but a
		// document every clause refuses — a recovered panic, or `reportsNothing` — would otherwise score as a file of
		// agreeing pairs. It is a fact about the file, and must be named like a disagreement.
		if refusedWhole(nr) {
			unopened[names[i]+" / refused"] = fmt.Sprintf("%s: nib refused every clause — %s", names[i], nr.Results[0].Why)
		}
		reports[i] = &nr
	}
	cmp := compareToOracle(names, vera, reports)
	for _, e := range judgeProducers(cmp, unopened, producerDisagreements) {
		t.Error(e)
	}

	var producers []string
	for p := range perProducer {
		producers = append(producers, p)
	}
	sort.Strings(producers)
	for _, p := range producers {
		t.Logf("producer %s: %d file(s)", p, perProducer[p])
	}
	// `agreed` counts a permitted refusal as agreeing (`agrees`); the figure logged is SETTLED agreement — a Pass or Fail
	// that veraPDF matched — with the refusals beside it, so neither can be read as the other.
	t.Logf("real producers: %d of %d (file, clause) pairs settled and agreeing, %d refused, over %d files; %d file-level "+
		"fact(s) (unopened, unreported, refused whole)", cmp.agreed-len(cmp.cannot), cmp.total, len(cmp.cannot), len(files), len(unopened))
	// Reach is a clause settled AND agreeing on a file — a disagreement, named or not, is not reach.
	reach := map[string]int{}
	for i, r := range reports {
		if r == nil || vera[i] == nil {
			continue
		}
		for _, res := range r.Results {
			if v, listed := vera[i][res.Clause]; listed && (res.Verdict == Pass || res.Verdict == Fail) && v.agrees(res.Verdict) {
				reach[res.Clause]++
			}
		}
	}
	for _, c := range Clauses() {
		t.Logf("reach %-12s %d", c, reach[c])
	}
}

// TestTheProducerJudgeNamesEveryDisagreement drives every branch of `judgeProducers` without veraPDF — the harness's
// assertions otherwise fire only on a corpus that happens to hold the situation, and a guard nobody has seen fail is a
// claim.
func TestTheProducerJudgeNamesEveryDisagreement(t *testing.T) {
	names := []string{"libreoffice/a.pdf", "word/b.pdf"}
	vera := []map[string]veraState{{"7.1 t1": veraFailed}, {"7.1 t1": veraPassed}}
	nib := []*Report{
		{Results: []Result{{Clause: "7.1 t1", Verdict: Pass}}}, // a planted disagreement: veraPDF failed it
		{Results: []Result{{Clause: "7.1 t1", Verdict: Pass}}}, // agrees
	}
	cmp := compareToOracle(names, vera, nib)
	// The stimulus before the response: the planted input IS a disagreement, and only that one.
	if len(cmp.disagree) != 1 || cmp.disagree["libreoffice/a.pdf / 7.1 t1"] == "" {
		t.Fatalf("stimulus: the planted pair should be the one disagreement, got %v", cmp.disagree)
	}

	if errs := judgeProducers(cmp, nil, map[string]string{}); len(errs) != 1 || !strings.Contains(errs[0], "not named") {
		t.Errorf("an unnamed disagreement: %v, want exactly one \"not named\" error", errs)
	}
	named := map[string]string{"libreoffice/a.pdf / 7.1 t1": "/pending 0 — a test row"}
	if errs := judgeProducers(cmp, nil, named); len(errs) != 0 {
		t.Errorf("a named disagreement: %v, want none", errs)
	}
	stale := map[string]string{"libreoffice/a.pdf / 7.1 t1": "x", "word/b.pdf / 7.1 t1": "no longer disagrees"}
	if errs := judgeProducers(cmp, nil, stale); len(errs) != 1 || !strings.Contains(errs[0], "no longer disagrees") {
		t.Errorf("a stale row: %v, want exactly one stale-row error", errs)
	}
	unopened := map[string]string{"word/c.pdf / open": "word/c.pdf: nib cannot open it"}
	if errs := judgeProducers(cmp, unopened, named); len(errs) != 1 || !strings.Contains(errs[0], "word/c.pdf") {
		t.Errorf("a file nib cannot open, unnamed: %v, want exactly one error naming it", errs)
	}
	namedOpen := map[string]string{"libreoffice/a.pdf / 7.1 t1": "x", "word/c.pdf / open": "/pending 0 — a test row"}
	if errs := judgeProducers(cmp, unopened, namedOpen); len(errs) != 0 {
		t.Errorf("a file nib cannot open, named: %v, want none — a named file-level row is neither an error nor stale", errs)
	}
	// A clause veraPDF does not list is never excusable by name either.
	unlisted := compareToOracle(names, []map[string]veraState{{"7.1 t1": veraFailed}, {"7.1 t2": veraPassed}}, nib)
	if errs := judgeProducers(unlisted, nil, named); len(errs) != 1 || !strings.Contains(errs[0], "does not list") {
		t.Errorf("an unlisted clause: %v, want exactly the unlisted-clause error", errs)
	}
	// The split is checked: an error the comparison raised that is in neither half is reported, not dropped.
	leaky := cmp
	leaky.errors = append(append([]string{}, cmp.errors...), "an error in neither half")
	if errs := judgeProducers(leaky, nil, named); len(errs) != 1 || !strings.Contains(errs[0], "no longer feeds the split") {
		t.Errorf("an error outside the split: %v, want the split error", errs)
	}
	if !refusedWhole(Report{Results: []Result{{Verdict: CannotCheck}, {Verdict: CannotCheck}}}) ||
		refusedWhole(Report{Results: []Result{{Verdict: CannotCheck}, {Verdict: Pass}}}) || refusedWhole(Report{}) {
		t.Error("refusedWhole must be true only for a report every clause of which refuses")
	}
	// A lost job is never excusable by name, however the table reads.
	lost := compareToOracle(names, []map[string]veraState{nil, {"7.1 t1": veraPassed}}, nib)
	if errs := judgeProducers(lost, nil, map[string]string{"libreoffice/a.pdf / 7.1 t1": "x"}); len(errs) != 2 ||
		!strings.Contains(strings.Join(errs, "\n"), "no job") {
		t.Errorf("a lost job: %v, want the lost-job error and the now-stale row", errs)
	}
}
