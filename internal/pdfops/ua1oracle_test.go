package pdfops

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The veraPDF `ua1` differential as a STANDING reader — `PLAN-accessibility.md` P03.S03, closing
// `/pending 469`.
//
// # What it closes
//
// P01's exit criterion 2 and P01.S06's first acceptance clause say the same thing: *an operation's
// output fails no ua1 clause its input did not already fail.* It was discharged by measurement on
// three documents — and **every one of those runs was by hand**, `~/verapdf/verapdf --flavour ua1`
// at a slice close, read by a person. Nothing in `go test ./...` took that differential, so the
// criterion could stop being true and the going-stale would be invisible.
//
// # Why it is affordable, which is the whole reason it can live in tier 1
//
// veraPDF is a JVM and costs ~1.5 s to start. Run once per document that is **49 operations × a
// start-up**, about three minutes, which is why this was never wired. veraPDF takes **many files in
// one invocation**: measured 2026-09-11, 12 files in 2.6 s against 1.5 s for one — roughly 90 ms
// per additional file once the JVM is up. The whole census is one batch.
//
// # The population is the census, not a list
//
// It drives `tagFates`, which ADR-031 law 2 already enumerates from the code with its own stale-row
// check and floor. A hand-kept list here would be a second population that drifts from the first.

type ua1Report struct {
	Jobs []struct {
		Item struct {
			Name string `xml:"name"`
		} `xml:"item"`
		Report struct {
			Status string `xml:"jobEndStatus,attr"`
			Rules  []struct {
				Clause string `xml:"clause,attr"`
				Test   string `xml:"testNumber,attr"`
				Status string `xml:"status,attr"`
			} `xml:"details>rule"`
		} `xml:"validationReport"`
	} `xml:"jobs>job"`
}

// ua1FailedClauses validates every file in ONE veraPDF invocation and returns, per file, the set of
// clauses it fails ("7.1 t3"). A file veraPDF could not validate maps to nil, which the caller must
// distinguish from an empty set — those two mean opposite things.
func ua1FailedClauses(t *testing.T, vp string, files []string) map[string]map[string]bool {
	t.Helper()
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1"}, files...)...).Output()
	var rep ua1Report
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("veraPDF report did not parse: %v\n%.600s", err, out)
	}
	got := map[string]map[string]bool{}
	for _, j := range rep.Jobs {
		name := filepath.Base(j.Item.Name)
		if j.Report.Status != "normal" {
			got[name] = nil
			continue
		}
		set := map[string]bool{}
		for _, r := range j.Report.Rules {
			if r.Status == "failed" {
				set[r.Clause+" t"+r.Test] = true
			}
		}
		got[name] = set
	}
	return got
}

func sortedClauses(s map[string]bool) []string {
	out := make([]string, 0, len(s))
	for c := range s {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// knownUA1Deltas records, per operation, the ua1 clauses its output adds to the conformant census
// document — **and the reason each one is there**. An operation with no row must add nothing.
//
// # Why a table of deltas and not a flat "adds nothing"
//
// P01 struck that acceptance for itself and wrote down why: *"dropping is what law 1 demands and
// necessarily ADDS failures, since PDF/UA requires a tree, so a ua1 failure COUNT scores honesty as
// a regression."* An operation that gives up the tree on purpose adds the clauses a missing tree
// looks like, and a guard that failed on those would be demanding the dishonest option.
//
// So the rule is not "adds nothing"; it is **"adds nothing that is not written down here"** — and it
// is checked in both directions, like `tagFates`. A clause that appears is a regression; a clause
// that stops appearing means something was fixed and the row is now a claim about code that no
// longer exists. Every row names the phase or item that owes it (`PLAN-ua-coverage.md`), or why the
// loss is permanent.
//
// **5 t1 is excluded for every operation** (ADR-032: any change drops the PDF/UA identification), so
// it appears in no row.
var pageSetLoss = []string{"6.2 t1", "7.1 t10", "7.1 t11", "7.1 t3", "7.1 t8"}

var knownUA1Deltas = map[string]struct {
	clauses []string
	why     string
}{
	// ── Annotations and form fields.
	"AuthorTaggedForm": {[]string{"7.21.4.1 t1"}, "Helvetica field text (/pending 479, pdfcpu cannot fill an embedded face)"},
	"AuthorForm":       {[]string{"7.18.4 t1", "7.21.4.1 t1"}, "the UNTAGGED door; the app authors through AuthorTaggedForm. Fonts /pending 479"},

	// ── **`Collect`, `DuplicatePage`, `Booklet` and `CarryAttachments` have NO row since P02.S04b**,
	// and their absence is the slice's strongest measurement: a subset prunes the source tree onto
	// the pages it keeps, so each adds nothing at all to a conformant document where it previously
	// added all five of `pageSetLoss`. `RemovePages` is the one subset that still adds a clause, and
	// it is not a loss of structure:
	"RemovePages": {[]string{"7.4.2 t1"}, "its drive removes page 1, which carries the document's " +
		"only /H1 — so the remaining document genuinely starts at /H2 and genuinely mis-nests. " +
		"The tree is carried; the heading it needed went with the page the caller asked to delete"},

	// ── The tree goes, and with it the metadata. `tagFates` declares these `dropped`, each because
	// its own slice is blocked on Dan: crop P02.S05, the splits P02.S06, the merge graft P02.S07.
	"Crop":         {pageSetLoss, "rebuilt page by page — P02.S05, blocked"},
	"SplitPage":    {pageSetLoss, "rebuilt page by page — P02.S06, blocked"},
	"SplitRegions": {pageSetLoss, "rebuilt page by page — P02.S06, blocked"},
	"InsertPDF":    {append(append([]string{}, pageSetLoss...), "7.21.4.1 t1"), "spliced through the NON-carrying door, with an untagged Base-14 document inserted — P02.S07, blocked"},
	// **`NUp` had a `7.20 t2` row until P02.S02 and no longer does.** The clause was veraPDF's
	// `isUniqueSemanticParent` — *"Form XObject contains MCIDs and is referenced more than once"* —
	// and the cause was nib's own carry anchoring a form that pdfcpu's optimize pass had fused
	// across sheets. The carry un-fuses now, so the census n-up fails nothing but `5 t1`, which
	// every operation fails by design (ADR-032). Measured, not assumed: this row's removal is the
	// both-ways half of that fix, and leaving it here would fail this table for a clause that no
	// longer appears.

	// ── Untagged content arrives from somewhere else: the second document's own pages.
	"Append":  {[]string{"7.1 t3", "7.21.4.1 t1"}, "the appended document is untagged and Base-14: ADR-031's recorded `partial` decision"},
	"Combine": {[]string{"7.1 t3", "7.21.4.1 t1"}, "as Append, for every document after the first"},

	"StripMetadata": {[]string{"7.1 t8"}, "permanent: removing identifying metadata is what the operation is for"},
}

// knownUnvalidatable records operations whose output on THIS document veraPDF cannot validate at all,
// with the reason. A row here is a claim that the cause is the document rather than the operation.
//
// Empty since the census was rebased onto a document of several pages: `RemovePages` was the one row,
// because the old one-page fixture lost its only page. The map and both of its checks stay, so the
// next unvalidatable output has to be named rather than read as clean.
var knownUnvalidatable = map[string]string{}

// censusMarkdown is long enough to lay out across several pages, so the page-set operations act on a
// document they can take pages out of.
func censusMarkdown() string {
	var b strings.Builder
	b.WriteString("# The census document\n\n")
	for i := 1; i <= 40; i++ {
		b.WriteString("## Section\n\nA paragraph of ordinary prose, long enough to wrap across the measure of the page and give each page real content.\n\n- a list item\n- another\n\n")
	}
	return b.String()
}

// TestNoOperationAddsAUA1ClauseItsInputDidNotFail is the criterion, asked of every operation in the
// census at once, against the table above.
//
// **The skip is loud and says it is not a pass.** `/pending 411` records three seed tests reporting
// SKIP on a condition that was silently always true; a quiet skip here would turn the one standing
// reader of P01's strongest criterion back into nothing, while looking green.
func TestNoOperationAddsAUA1ClauseItsInputDidNotFail(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so PLAN-accessibility.md P01's exit " +
			"criterion 2 — no operation adds a ua1 clause its input did not fail — is UNCHECKED " +
			"in this run. Set NIB_VERAPDF, put verapdf on PATH, or install to ~/verapdf.")
	}

	// The census document: a Markdown conversion nib labels PDF/UA, which veraPDF scores conformant.
	//
	// **Rebased from `taggedFixture()` (2026-09-15, PLAN-ua-coverage.md P01.S01), and why.** That fixture is
	// one hand-built page that already fails 7.1 t8, 7.1 t10 and 7.21.4.1 t1 — so any operation ADDING one
	// of those was invisible to a differential against it, and three stamping operations, the form door
	// and every page-set operation were adding exactly those. On a conformant document every added clause
	// shows. Several pages, because `RemovePages` left the one-page fixture with nothing to validate and
	// `NUp`'s 7.20 t2 needs sheets to compose.
	src, lerr := LabelUA(labelReady(t, censusMarkdown()), true)
	if lerr != nil {
		t.Fatalf("setup: the census document could not be labelled: %v", lerr)
	}
	if n, perr := PageCount(src); perr != nil || n < 3 {
		t.Fatalf("setup: the census document has %d page(s) (err %v); the page-set operations need several", n, perr)
	}

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "00-base.pdf")
	if err := os.WriteFile(srcPath, src, 0o600); err != nil {
		t.Fatal(err)
	}
	files := []string{srcPath}
	byFile := map[string]string{} // basename -> operation name

	names := make([]string, 0, len(tagFates))
	for name := range tagFates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		f := tagFates[name]
		if f.drive == nil {
			continue
		}
		out, err := f.drive(src)
		if err != nil {
			// An operation that refuses the fixture cannot damage it. Recorded, not failed — the
			// corpus is one document and not every operation applies to it.
			t.Logf("%s: not exercised on this fixture (%v)", name, err)
			continue
		}
		p := filepath.Join(dir, name+".pdf")
		if err := os.WriteFile(p, out, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
		byFile[name+".pdf"] = name
	}
	if len(byFile) < 20 {
		t.Fatalf("only %d operation(s) produced output; the differential is barely testing anything",
			len(byFile))
	}

	clauses := ua1FailedClauses(t, vp, files)
	base, ok := clauses["00-base.pdf"]
	if !ok || base == nil {
		t.Fatalf("veraPDF could not validate the input fixture, so there is no baseline to "+
			"differ against (report had %d job(s))", len(clauses))
	}
	// Stimulus before response: the document the operations were given passes every clause, or every
	// "added" below is measured against a baseline that already failed — the gap this census was rebased
	// to close.
	if len(base) > 0 {
		t.Fatalf("setup: the census document is not PDF/UA-1 conformant by veraPDF (%v), so an added clause "+
			"it already fails could not be seen", sortedClauses(base))
	}

	seenOps := map[string]bool{}
	for file, name := range byFile {
		seenOps[name] = true
		got, seen := clauses[file]
		if !seen {
			t.Errorf("%s: veraPDF returned no job for its output — the batch lost a file, and a "+
				"missing job reads exactly like a clean one", name)
			continue
		}
		if got == nil {
			if why, known := knownUnvalidatable[name]; known {
				t.Logf("%s: output not validatable, recorded — %s", name, why)
				continue
			}
			t.Errorf("%s: veraPDF could not validate its output at all. That is not a pass: an "+
				"output the reference validator cannot parse is a worse result than one that "+
				"fails clauses. If the cause is the fixture rather than the operation, add a row "+
				"to `knownUnvalidatable` saying which", name)
			continue
		}
		if _, known := knownUnvalidatable[name]; known {
			t.Errorf("%s: `knownUnvalidatable` says veraPDF cannot validate this output, and it "+
				"just did. Remove the row", name)
		}

		var added []string
		for c := range got {
			if !base[c] && c != "5 t1" { // ADR-032: every change drops the identification
				added = append(added, c)
			}
		}
		sort.Strings(added)
		want := append([]string(nil), knownUA1Deltas[name].clauses...)
		sort.Strings(want)
		if strings.Join(added, ",") == strings.Join(want, ",") {
			continue
		}
		if len(want) == 0 {
			t.Errorf("%s adds ua1 clause(s) its input did not fail: %s\n\tPLAN-accessibility.md "+
				"P01 exit criterion 2. The input fails %v. Either the operation damaged something "+
				"the input had, or it made a claim the document cannot support. If neither — if "+
				"the clause is the honest shape of a loss the census already declares — add a row "+
				"to `knownUA1Deltas` with the reason.",
				name, strings.Join(added, ", "), sortedClauses(base))
			continue
		}
		t.Errorf("%s's recorded ua1 delta no longer matches: recorded %v, measured %v (%s)\n\t"+
			"A clause that APPEARED is a regression. A clause that went AWAY means something was "+
			"fixed and this row is now a claim about code that does not exist — shrink it, and say "+
			"in the commit what fixed it.",
			name, want, added, knownUA1Deltas[name].why)
	}

	// Both directions, like `tagFates`: a row for an operation the census no longer drives is a
	// claim nothing tests.
	for name := range knownUA1Deltas {
		if !seenOps[name] {
			t.Errorf("`knownUA1Deltas` has a row for %q, which produced no output in this run — "+
				"the operation is gone, renamed, or now refuses the fixture. Remove the row", name)
		}
	}
	for name := range knownUnvalidatable {
		if !seenOps[name] {
			t.Errorf("`knownUnvalidatable` has a row for %q, which produced no output in this run",
				name)
		}
	}
}
