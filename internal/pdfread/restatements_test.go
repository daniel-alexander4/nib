package pdfread

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// restatements.tsv is D7's inventory (PLAN-pdfcpu-upgrade P01.S02): every place nib restates what pdfcpu does, with
// the verdict a port gives it. It lives in the repo and not in memory because a fresh clone has no memory directory.
// A table nothing reads goes stale silently, so this census is its caller. Its scope is .go files: the plan's wider
// figure (≈100 hits in 62 files) counts docs and scripts, which carry the same citations and are not walked here.

const restatementColumns = 7

// restatementCites is what a citation of the old version looks like: `v0.13.0`, and also the bare `v0.13` three sites
// wrote (the first version of this census matched only the long form and passed while three were unlisted).
var restatementCites = regexp.MustCompile(`v0\.13\b`)

// restatementWindow is how far from a row's site a citation may sit and still be that row's: adjacent lines of one
// comment block citing the same thing share a row.
const restatementWindow = 12

type restatementRow struct {
	file    string
	line    int
	kind    string
	verdict string
}

func readRestatements(t *testing.T) []restatementRow {
	t.Helper()
	f, err := os.Open("restatements.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []restatementRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 0; sc.Scan(); n++ {
		cols := strings.Split(sc.Text(), "\t")
		if n == 0 {
			if len(cols) != restatementColumns || cols[0] != "site" || cols[5] != "verdict" {
				t.Fatalf("restatements.tsv: the header is %q — the columns are the inventory's contract", sc.Text())
			}
			continue
		}
		if len(cols) != restatementColumns {
			t.Fatalf("restatements.tsv line %d has %d columns, want %d", n+1, len(cols), restatementColumns)
		}
		i := strings.LastIndex(cols[0], ":")
		if i < 0 {
			t.Fatalf("restatements.tsv line %d: site %q is not path:line", n+1, cols[0])
		}
		ln, err := strconv.Atoi(cols[0][i+1:])
		if err != nil {
			t.Fatalf("restatements.tsv line %d: site %q has no line number", n+1, cols[0])
		}
		rows = append(rows, restatementRow{file: cols[0][:i], line: ln, kind: cols[1], verdict: cols[5]})
	}
	return rows
}

// TestEveryV013CitationHasARestatementsRow — the census. Every .go line outside third_party that names `v0.13` is within
// the window of a row for its file: a new citation of the old version with no row is a restatement nobody will
// re-read at the port, which is the failure D7 exists to prevent.
func TestEveryV013CitationHasARestatementsRow(t *testing.T) {
	rows := readRestatements(t)
	if len(rows) == 0 {
		t.Fatal("restatements.tsv has no rows")
	}
	root := filepath.Join("..", "..")
	hits := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "third_party" || rel == "node_modules" || rel == ".git" || rel == "dist" || rel == "web" || rel == "test" {
				return filepath.SkipDir
			}
			return nil
		}
		// This census names the literal it searches for; it is not a citation of it.
		if !strings.HasSuffix(rel, ".go") || rel == "internal/pdfread/restatements_test.go" {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, ln := range strings.Split(string(raw), "\n") {
			if !restatementCites.MatchString(ln) {
				continue
			}
			hits++
			near := false
			for _, r := range rows {
				if r.file == rel && r.line-restatementWindow <= i+1 && i+1 <= r.line+restatementWindow {
					near = true
					break
				}
			}
			if !near {
				t.Errorf("%s:%d names v0.13 and restatements.tsv has no row for it — add one (what it restates, the v0.16.2 line), then the port re-reads it (D7)", rel, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if hits == 0 {
		t.Fatal("the census found no citation of v0.13.0 at all — it is looking in the wrong place")
	}
}

// TestRestatementRowsPointAtRealLines — a row whose site has moved or vanished describes nothing. The verdict is one of
// the four D7 names.
func TestRestatementRowsPointAtRealLines(t *testing.T) {
	root := filepath.Join("..", "..")
	open := 0
	for _, r := range readRestatements(t) {
		switch r.verdict {
		case "pending":
			open++
		case "delete", "re-cite", "re-derive":
		default:
			t.Errorf("%s:%d: verdict %q is none of pending, delete, re-cite, re-derive", r.file, r.line, r.verdict)
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(r.file)))
		if err != nil {
			t.Errorf("%s:%d: the file is gone: %v", r.file, r.line, err)
			continue
		}
		lines := strings.Split(string(raw), "\n")
		if r.line < 1 || r.line > len(lines) {
			t.Errorf("%s:%d: the file has %d lines", r.file, r.line, len(lines))
			continue
		}
		// A version-token row's site is a line that names the version: a row whose line has drifted describes nothing.
		if r.kind == "version-token" && !restatementCites.MatchString(lines[r.line-1]) {
			t.Errorf("%s:%d: a version-token row, and the line no longer names v0.13", r.file, r.line)
		}
	}
	t.Logf("%d rows, %d verdicts still pending (P02.S03 gives each one)", len(readRestatements(t)), open)
}
