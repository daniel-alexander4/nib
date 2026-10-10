package nib

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestEveryFileDerivedFromVeraPDFSaysSo — `/pending 638`.
//
// # The rule
//
// A file of nib's that is derived from veraPDF's SOURCE carries, as its first line,
// `// SPDX-License-Identifier: MPL-2.0 OR AGPL-3.0-only`. veraPDF is GPLv3+ or MPLv2+; the header takes
// its MPL option, which MPL 2.0 section 3.3 lets nib distribute under the AGPL as well. Without it the
// repository says "ported step for step" about another project's code and nothing about on what terms.
//
// # What "derived" is read as, and what it is not
//
// Two marks, both things the file says about itself: it names a veraPDF source file (`GFSETable.java`),
// or it says a veraPDF test, predicate or algorithm is transcribed or ported. **Reproducing veraPDF's
// observed behaviour is not a mark** — most of the checker was settled by running 1.30.2 and measuring
// what it answered, which is observation of a program and derives from no file of it.
//
// Test files are outside the rule: they quote the oracle they are checking against.
//
// # Both directions
//
// A file with a mark and no header fails; so does a file with the header and no mark, because the
// header offers that file under MPL and nothing should be offered that way by accident.
func TestEveryFileDerivedFromVeraPDFSaysSo(t *testing.T) {
	const header = "// SPDX-License-Identifier: MPL-2.0 OR AGPL-3.0-only"
	names := regexp.MustCompile(`\b[A-Z][A-Za-z0-9]+\.java\b`)
	ported := regexp.MustCompile(`veraPDF's (test|predicate|algorithm),? (transcribed|ported)|ported from veraPDF`)
	var marked, headed []string
	for _, root := range []string{"internal", "cmd", "mdpdf"} {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			src := string(b)
			if names.MatchString(src) || ported.MatchString(src) {
				marked = append(marked, p)
			}
			if strings.HasPrefix(src, header+"\n") {
				headed = append(headed, p)
			} else if strings.Contains(src, header) {
				t.Errorf("%s carries the header somewhere other than its first line, where a licence scanner reads it", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(marked)
	sort.Strings(headed)
	// Anti-vacuity: a renamed directory or a changed citation style would make both lists empty and
	// the comparison below pass on nothing.
	if len(marked) < 10 {
		t.Fatalf("setup: only %d files are read as derived from veraPDF (%v); twelve were when this was written, so the scan has stopped seeing them", len(marked), marked)
	}
	in := func(list []string, p string) bool {
		i := sort.SearchStrings(list, p)
		return i < len(list) && list[i] == p
	}
	for _, p := range marked {
		if !in(headed, p) {
			t.Errorf("%s cites veraPDF's source or says it transcribes it, and does not start with %q", p, header)
		}
	}
	for _, p := range headed {
		if !in(marked, p) {
			t.Errorf("%s starts with the MPL header and neither names a veraPDF source file nor says it is ported — it offers that file under MPL for no stated reason", p)
		}
	}

	// And a recipient is told: the notices name every headed file.
	notices, err := os.ReadFile("THIRD-PARTY-NOTICES.md")
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(notices), "\n## veraPDF\n")
	if i < 0 {
		t.Fatal("THIRD-PARTY-NOTICES.md has no veraPDF section")
	}
	section := string(notices)[i:]
	if j := strings.Index(section, "\n---\n"); j >= 0 {
		section = section[:j]
	}
	for _, p := range headed {
		if !strings.Contains(section, "`"+filepath.ToSlash(p)+"`") {
			t.Errorf("the notices' veraPDF section does not name %s", p)
		}
	}
}
