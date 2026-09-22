package nib

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The accessibility parity ledger — `PLAN-accessibility.md` P10.S04.
//
// `docs/accessibility-parity.md` is a claim document, so what is guarded is that its claims stay attached
// to evidence. Every Acrobat cell cites a source that resolves to an Adobe help page; every Nib test it
// cites exists; every row that claims Nib does something cites at least one test; every feature Adobe's
// cited pages name has a row; and every accessibility door Nib has — the routes, enumerated from
// `server.go`, and the `nib tag` subcommands, enumerated from `tag.go` — appears. The feature names are a
// list, not an enumeration, because Acrobat has no code here to enumerate; they are the headings and tool
// names on the five pages cited.
func TestTheAccessibilityParityLedgerNamesEveryFeatureAndCitesEveryClaim(t *testing.T) {
	b, err := os.ReadFile("docs/accessibility-parity.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)

	sources := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^- \[(A\d+)\] .*https://helpx\.adobe\.com/\S+`).FindAllStringSubmatch(section(doc, "## Sources"), -1) {
		sources[m[1]] = true
	}
	if len(sources) == 0 {
		t.Fatal("the ledger lists no Adobe source")
	}

	tests := goTestNames(t)
	statuses := map[string]bool{"Parity": true, "Partial": true, "Gap": true, "Nib only": true, "Unmeasured": true}
	refRE := regexp.MustCompile(`\[(A\d+)\]`)
	goRE := regexp.MustCompile("`(Test[A-Za-z0-9_]+)`")
	jsRE := regexp.MustCompile("`(test/[a-z]+/[a-z0-9-]+\\.test\\.mjs): '([^']+)'`")

	var features []string
	rows := 0
	for _, line := range strings.Split(section(doc, "## The ledger"), "\n") {
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "| Feature |") || strings.HasPrefix(line, "| ---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 5 {
			t.Errorf("a ledger row has %d cells, want 5: %.80s", len(cells), line)
			continue
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows++
		feature, acrobat, nib, evidence, status := cells[0], cells[1], cells[2], cells[3], cells[4]
		features = append(features, feature)
		if !statuses[status] {
			t.Errorf("%s: status %q is not one the ledger defines", feature, status)
		}
		refs := refRE.FindAllStringSubmatch(acrobat, -1)
		if len(refs) == 0 && acrobat != "Not described on the cited pages" {
			t.Errorf("%s: the Acrobat claim cites no source", feature)
		}
		for _, r := range refs {
			if !sources[r[1]] {
				t.Errorf("%s: cites [%s], which Sources does not list", feature, r[1])
			}
		}
		if nib == "" {
			t.Errorf("%s: the Nib cell is empty — a gap is named, not left blank", feature)
		}
		cited := 0
		for _, m := range goRE.FindAllStringSubmatch(evidence, -1) {
			cited++
			if !tests[m[1]] {
				t.Errorf("%s: cites %s, and no Go test has that name", feature, m[1])
			}
		}
		for _, m := range jsRE.FindAllStringSubmatch(evidence, -1) {
			cited++
			src, rerr := os.ReadFile(m[1])
			if rerr != nil || !strings.Contains(string(src), "test('"+m[2]+"'") {
				t.Errorf("%s: cites %s: %q, and that file has no such test", feature, m[1], m[2])
			}
		}
		if (status == "Parity" || status == "Partial" || status == "Nib only") && cited == 0 {
			t.Errorf("%s: claims %s and cites no test", feature, status)
		}
	}
	if rows < 15 {
		t.Fatalf("the ledger reads %d row(s) — too few to be the table", rows)
	}

	for _, name := range []string{
		"Accessibility Check", "Fix", "Automatically tag", "Remove and replace", "Reading Order", "Tag a selected region",
		"Tags panel", "Alternate text", "Artifact", "Table header", "Role map", "Document language", "Document title",
		"Form field", "Recognize text", "Make Accessible", "Read Out Loud", "Reflow", "accessible text", "Tag annotations",
	} {
		found := false
		for _, f := range features {
			found = found || strings.Contains(f, name)
		}
		if !found {
			t.Errorf("Adobe's cited pages name %q and the ledger has no row for it", name)
		}
	}

	server, err := os.ReadFile(filepath.Join("internal", "server", "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	routes := regexp.MustCompile(`HandleFunc\("(?:GET|POST) (/api/(?:tags/[a-z]+|uacheck|ocr))"`).FindAllStringSubmatch(string(server), -1)
	if len(routes) < 6 {
		t.Fatalf("found %d accessibility route(s) in server.go — the pattern no longer reads the registry", len(routes))
	}
	for _, r := range routes {
		if !strings.Contains(doc, "`"+r[1]+"`") {
			t.Errorf("the route %s appears nowhere in the ledger", r[1])
		}
	}
	tag, err := os.ReadFile(filepath.Join("internal", "cli", "tag.go"))
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(tag), "func cmdTag(")
	j := strings.Index(string(tag)[i:], "\n}\n")
	subs := regexp.MustCompile(`case "([a-z]+)":`).FindAllStringSubmatch(string(tag)[i:i+j], -1)
	named := 0
	for _, s := range subs {
		if s[1] == "help" {
			continue
		}
		named++
		if !strings.Contains(doc, "nib tag "+s[1]) {
			t.Errorf("nib tag %s appears nowhere in the ledger", s[1])
		}
	}
	if named < 4 {
		t.Fatalf("found %d nib tag subcommand(s) — the pattern no longer reads cmdTag", named)
	}
	for _, must := range []string{"--do ua", "--do tag", "47 of the 106"} {
		if !strings.Contains(doc, must) {
			t.Errorf("the ledger does not say %q", must)
		}
	}
}

// section is doc from heading to the next level-two heading.
func section(doc, heading string) string {
	i := strings.Index(doc, heading)
	if i < 0 {
		return ""
	}
	rest := doc[i+len(heading):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		return rest[:j]
	}
	return rest
}

// goTestNames is every Test function declared in the repository's Go test files.
func goTestNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	re := regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == ".claude") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			names[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 500 {
		t.Fatalf("found %d Go test name(s) — too few to be the repository", len(names))
	}
	return names
}
