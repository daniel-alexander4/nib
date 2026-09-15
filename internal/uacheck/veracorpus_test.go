package uacheck

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The checker against veraPDF's OWN PDF/UA-1 corpus — `/pending 489`.
//
// Law 5's oracle test (`oracle_test.go`) runs over documents nib and LibreOffice produced, and both write
// the easy encodings: a direct string `/Lang`, a direct boolean `/Marked`. veraPDF's corpus uses every
// legal encoding, and a first run over it found 34 false `Fail` verdicts on files veraPDF passes — all of
// them readers breaking on a hex string, an indirect object, or a rule stricter than veraPDF's. This test
// is that run, standing.
//
// **What it asserts, and against what.** For every corpus file and every clause nib implements:
//   - a FALSE PASS is nib `Pass` where veraPDF failed the clause;
//   - a FALSE FAIL is nib `Fail` where veraPDF did not fail it.
// Both are errors unless the (file, clause) pair is in `corpusAllow` with its reason. `CannotCheck` is a
// declared limit and never either. A file nib cannot open is reported, not scored — nib's `open` error is
// outside every rule.
//
// **Where the corpus comes from.** `$NIB_UA_CORPUS`, else `~/nib/verapdfs/PDF_UA-1` — a sparse clone of
// `github.com/veraPDF/veraPDF-corpus`. Absent corpus or absent veraPDF is a SKIP that says so, the way
// law 5's own test skips: a green run over an absent oracle verifies nothing.

// corpusAllow is every (file, clause) pair this test excuses, with why. Keyed "<dir>/<file> / <clause>".
var corpusAllow = map[string]string{}

// corpusUnreadable is every corpus file nib's reader cannot open, with why. Reported, not scored. Both
// are pdfcpu's validator refusing the file before any rule runs, which is outside every clause nib checks.
var corpusUnreadable = map[string]string{
	"7.18 Annotations/7.18.2 Annotation types/7.18.2-t01-fail-a.pdf": "pdfcpu: a TrapNet annotation without its required /F",
	"7.21 Fonts/7.21.6 Character encodings/7.21.6-t03-fail-a.pdf":    "pdfcpu: an /Encoding named Custom, which it refuses",
}

func corpusDir() string {
	if d := os.Getenv("NIB_UA_CORPUS"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "nib", "verapdfs", "PDF_UA-1")
}

func TestTheCheckerAgreesWithVeraPDFsOwnCorpus(t *testing.T) {
	dir := corpusDir()
	var files []string
	if dir != "" {
		filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasSuffix(strings.ToLower(p), ".pdf") {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Skip("SKIP (not a pass): veraPDF's PDF/UA-1 corpus is absent, so /pending 489's standing run is " +
			"UNCHECKED. Set NIB_UA_CORPUS, or sparse-clone veraPDF-corpus's PDF_UA-1 to ~/nib/verapdfs.")
	}
	vp := veraPDFPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the corpus run is UNCHECKED. Set NIB_VERAPDF, put " +
			"verapdf on PATH, or install to ~/verapdf.")
	}
	sort.Strings(files)
	// A floor on the population: a corpus that shrank to a handful would pass every assertion below.
	if len(files) < 250 {
		t.Fatalf("the corpus holds %d PDF(s) — too few to be veraPDF's PDF/UA-1 set (297 when this was written)", len(files))
	}

	// veraPDF names each job by base name; the corpus reuses names across directories, so run it over
	// uniquely-named links in a temp dir.
	tmp := t.TempDir()
	links := make([]string, len(files))
	names := make([]string, len(files))
	for i, f := range files {
		rel, _ := filepath.Rel(dir, f)
		names[i] = filepath.ToSlash(rel)
		links[i] = filepath.Join(tmp, strings.NewReplacer("/", "__", " ", "_").Replace(names[i]))
		if err := os.Symlink(f, links[i]); err != nil {
			t.Fatal(err)
		}
	}
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1", "--passed"}, links...)...).Output()
	var rep veraReport
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the veraPDF report did not parse: %v\n%.500s", err, out)
	}
	vera := veraStates(rep, links)

	implemented := map[string]bool{}
	for _, c := range Clauses() {
		implemented[c] = true
	}
	var falsePass, falseFail []string
	scored, unreadable := 0, 0
	seenUnreadable := map[string]bool{}
	for i, f := range files {
		pdf, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if vera[i] == nil {
			t.Errorf("%s: veraPDF returned no job for it — a lost file reads exactly like a clean one", names[i])
			continue
		}
		nr, err := Check(pdf)
		if err != nil {
			unreadable++
			seenUnreadable[names[i]] = true
			if _, known := corpusUnreadable[names[i]]; !known {
				t.Errorf("%s: nib cannot open a corpus file and it is not recorded in corpusUnreadable: %v", names[i], err)
			}
			continue
		}
		for _, r := range nr.Results {
			if !implemented[r.Clause] || r.Verdict == CannotCheck {
				continue
			}
			v, listed := vera[i][r.Clause]
			if !listed {
				continue
			}
			scored++
			key := names[i] + " / " + r.Clause
			if _, allowed := corpusAllow[key]; allowed {
				continue
			}
			switch {
			case r.Verdict == Pass && v == veraFailed:
				falsePass = append(falsePass, key+" — veraPDF failed it")
			case r.Verdict == Fail && v != veraFailed:
				falseFail = append(falseFail, key+" — veraPDF "+string(v)+"; nib: "+r.Why+" ("+r.Where+")")
			}
		}
	}
	for k := range corpusUnreadable {
		if !seenUnreadable[k] {
			t.Errorf("corpusUnreadable has %q, which nib now opens — remove the row", k)
		}
	}
	t.Logf("veraPDF corpus: %d file(s), %d (file, clause) pair(s) scored, %d unreadable, %d false pass, %d false fail",
		len(files), scored, unreadable, len(falsePass), len(falseFail))
	for _, e := range falsePass {
		t.Errorf("FALSE PASS: %s", e)
	}
	for _, e := range falseFail {
		t.Errorf("FALSE FAIL: %s", e)
	}
}
