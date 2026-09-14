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

// knownUA1Deltas records, per operation, the ua1 clauses its output adds to the ones its input
// already failed — **and the reason each one is there**. An operation with no row must add nothing.
//
// # Why a table of deltas and not a flat "adds nothing"
//
// P01 struck that acceptance for itself and wrote down why: *"dropping is what law 1 demands and
// necessarily ADDS failures, since PDF/UA requires a tree, so a ua1 failure COUNT scores honesty as
// a regression."* An operation declared `dropped` in `tagFates` gives up the tree on purpose, and
// 6.2 t1 / 7.1 t11 / 7.1 t3 are what a missing tree LOOKS like to a clause counter. A guard that
// failed on those would be demanding the dishonest option.
//
// So the rule is not "adds nothing"; it is **"adds nothing that is not written down here"** — and
// it is checked in both directions, like `tagFates`. A clause that appears is a regression; a
// clause that stops appearing means something was fixed and the row is now a claim about code that
// no longer exists.
var knownUA1Deltas = map[string]struct {
	clauses []string
	why     string
}{
	// ── The tree goes, on purpose. `tagFates` declares every one of these `dropped`, and these
	// three clauses are the shape of a missing tree.
	"Booklet":       {[]string{"6.2 t1", "7.1 t11", "7.1 t3"}, "declared `dropped`: the tree goes and these three are what its absence looks like"},
	"Collect":       {[]string{"6.2 t1", "7.1 t11", "7.1 t3"}, "the same"},
	"DuplicatePage": {[]string{"6.2 t1", "7.1 t11", "7.1 t3"}, "the same"},

	// ── These four also added 7.2 t34 when this table was first written, because they dropped the
	// catalog `/Lang` as well as the tree. **That was `/pending 472`, and it is fixed** at v1.129.36
	// — `splice` and both of `Crop`'s exits now route through `carryLang`. The rows shrank as part
	// of the fix, which is what the both-ways check is FOR: a delta table that only fails on added
	// clauses becomes a list of permanent excuses.
	"Crop":         {[]string{"6.2 t1", "7.1 t11", "7.1 t3"}, "tree loss only, since /pending 472"},
	"InsertPDF":    {[]string{"6.2 t1", "7.1 t11", "7.1 t3"}, "the same"},
	"SplitPage":    {[]string{"6.2 t1", "7.1 t11", "7.1 t3"}, "the same"},
	"SplitRegions": {[]string{"6.2 t1", "7.1 t11", "7.1 t3"}, "the same"},

	// `CarryAttachments` kept its 7.2 t34 and is NOT part of 472: the census drives it as
	// `CarryAttachments(fixture, untaggedFixture())`, so the destination is a genuinely different
	// document and having no `/Lang` of the source's is correct. It was in the filed item until the
	// drive call was read. A differential reports what it measured, not what it measured it on.
	"CarryAttachments": {[]string{"6.2 t1", "7.1 t11", "7.1 t3", "7.2 t34"}, "the destination is a DIFFERENT document — a property of the drive call, not a defect"},

	// ── Untagged content arrives from somewhere else. Both merge a second document in, and the
	// pages that come with it are neither tagged nor marked as artifact — true of the second
	// document, and nothing either operation did to the first.
	"Append":  {[]string{"7.1 t3"}, "the appended document's own pages are untagged"},
	"Combine": {[]string{"7.1 t3"}, "the same, for every document after the first"},

	// ── Stamping creates an optional-content group, and pdfcpu builds its configuration
	// dictionary with an `/AS` key and no `/Name`. 7.10 t2 forbids `/AS` in an OC configuration
	// dictionary outright; 7.10 t1 requires `/Name`. Two catalog keys, four operations, and
	// `/pending 473`.
	// ── The four stamping operations USED to add 7.10 t1 and 7.10 t2 here, and `/pending 473`
	// named them. `honestOptionalContent` closed it at v1.129.58: pdfcpu writes the catalog's
	// default optional-content configuration with an `/AS` array and no `/Name`, and both are
	// forbidden by name. All four now add nothing, and the rows are gone rather than kept as a
	// claim about code that no longer behaves that way. `StampTextLayer` was never in the census
	// at all and had the identical defect; it is driven now.

	// ── Annotations and form fields arriving without their accessibility metadata. This is not a
	// defect to file; it is P06's stated goal — "authored form fields with /TU names" — and these
	// clauses are the measurement of the gap it closes.
	//
	// **`AuthorForm` shrank from three clauses to one at P06.S05** (v1.129.57): `/TU` took 7.18.1 t3
	// and `/Tabs /S` took 7.18.3 t1. What is left is not a key and cannot be fixed by one —
	// veraPDF's words are *"A Widget annotation shall be nested within a Form tag"*, failing with
	// *"nested within null tag (standard type = null) instead of Form"*. That is a structure
	// element with an `OBJR` kid, which P05.S03 already models and no phase has yet emitted.
	"AuthorForm": {[]string{"7.18.4 t1"}, "the widget is not nested in a Form structure element — needs a structure element written, not a key"},
	"AddNotes":   {[]string{"7.18.1 t1", "7.18.3 t1"}, "annotations with no /Contents and untagged — P06"},

	// ── Supplying an artefact CREATES the object other clauses inspect. With no /Metadata stream
	// at all, 5 t1 has no subject and is not evaluated; an honest one makes it applicable. Clearing
	// it means writing `pdfuaid:part`, a conformance assertion over untagged content, which is the
	// third thing ADR-031 law 1 forbids by name.
	//
	// **The gate is P07, not P05, and this line said P05 until P07 opened.** Writing a conformance
	// assertion needs something that can CHECK conformance, which the tag-tree core is not.
	// `tagmarkdown_test.go` had it right ("P07's job") and these two records disagreed for a
	// phase — the shape /pending 433 is about: naming a gate is not checking it is still shut.
	"SetTitle":      {[]string{"5 t1"}, "the XMP packet makes the PDF/UA-identification clause applicable; asserting it would be the lie ADR-031 forbids"},
	"TitleFromName": {[]string{"5 t1"}, "the same door, reached the ordinary way — it is SetTitle with a file name and a best-effort contract"},
}

// knownUnvalidatable records operations whose output on THIS fixture veraPDF cannot validate at
// all, with the reason. A row here is a claim that the cause is the fixture rather than the
// operation, and it has to say which.
var knownUnvalidatable = map[string]string{
	"RemovePages": "the census fixture is one page and the drive call removes page 1, so the " +
		"output has ZERO pages and veraPDF has no document to validate. A fixture artefact, not a " +
		"product result — `RemovePages` on a real multi-page document is exercised by " +
		"pdfops_test.go. It is recorded rather than skipped because an unvalidatable output and a " +
		"clean one are indistinguishable to a differential.",
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

	// Stimulus floor. A fixture that is not tagged, or that already fails everything, makes every
	// assertion below satisfiable by an operation that destroys the document.
	src := taggedFixture()
	if s := inspectTags(src); !s.claims() || s.anchored < 1 {
		t.Fatal("setup: the corpus fixture is not a tagged document, so this differential would " +
			"be measuring nothing")
	}

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "00-input.pdf")
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
	base, ok := clauses["00-input.pdf"]
	if !ok || base == nil {
		t.Fatalf("veraPDF could not validate the input fixture, so there is no baseline to "+
			"differ against (report had %d job(s))", len(clauses))
	}
	if len(base) == 0 {
		t.Fatal("setup: the fixture fails NO ua1 clause, which makes every `adds nothing new` " +
			"result below vacuous in the other direction — check the fixture is the tagged corpus")
	}
	t.Logf("baseline: the tagged fixture fails %d ua1 clause(s): %v", len(base), sortedClauses(base))

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
			if !base[c] {
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
