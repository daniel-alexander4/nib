package nib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryDocCommentNamesItsOwnFunction — /pending 352, repo-wide.
//
// # The defect, which is silent and reads correctly top to bottom
//
// A Go doc block with no blank line before the next function's own comment binds to **that**
// function. The result is that the documented function has no doc at all and its neighbour
// carries a paragraph about something else — and a reader who opens a function, reads the
// paragraph above it and acts on it is acting on a description of its neighbour. `CLAUDE.md`'s
// rule that a doc comment is a claim to check assumes the doc is at least attached to the code
// it describes.
//
// **Nothing this repo runs could see it.** `gofmt` does not care, `go vet` does not check it,
// and the compiler cannot. It was found by hand three times in two days before anyone parsed
// for it, so the count was unknown rather than zero: the first parse found **19** across 12
// files and 9 packages, one of which had been introduced that same day by the review that was
// fixing another instance of it.
//
// # Why it is at the root, like the goroutine census beside it
//
// The population is every package, and a hand-listed set is this same defect one level up. The
// walk discovers; it never lists.
//
// # The rule, and why it is the first line specifically
//
// Go's own convention is that a doc comment begins with the name of the thing it documents, so
// the first line is where a misattachment shows. Checking the whole block instead would pass a
// glued pair — two docs, one naming each function — which is exactly the shape found most often.
func TestEveryDocCommentNamesItsOwnFunction(t *testing.T) {
	// Docs that legitimately do not open with the function's name. Every entry carries its
	// reason, because an unexplained exemption is how a real instance gets parked and forgotten
	// — the same rule the published-shape scan states for its own exclusions.
	//
	// **Two, and both were found by the first run rather than assumed.** The entry that filed
	// this work claimed "no false positives in this run"; there were two, and they are the two
	// shapes a name-first rule cannot express: a long doc that opens with a markdown heading,
	// and one doc covering two adjacent one-line functions.
	exempt := map[string]string{
		"internal/rendezvous/seeds.go:seedNodes": "a 90-line doc opening with a `#` heading — the " +
			"name appears below it, and requiring the heading to come second would make the doc worse",
		"internal/cli/register_other.go:cmdRegister": "one doc for cmdRegister and cmdUnregister, " +
			"two one-line siblings on the following lines; it names neither because it is about both",
	}

	fset := token.NewFileSet()
	var bad []string
	seenExempt := map[string]bool{}
	funcs := 0
	documented := 0

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			// `.claude` holds a second session's git worktrees — a full copy of the tree, and
			// walking it doubles every count here exactly as it does for the goroutine census.
			case ".git", ".claude", "node_modules", "web", "docs", "test":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return perr
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			funcs++
			if fd.Doc == nil || len(fd.Doc.List) == 0 {
				continue
			}
			documented++
			key := filepath.ToSlash(path) + ":" + fd.Name.Name
			first := strings.TrimSpace(strings.TrimPrefix(fd.Doc.List[0].Text, "//"))
			if strings.Contains(first, fd.Name.Name) {
				continue
			}
			if _, ok := exempt[key]; ok {
				seenExempt[key] = true
				continue
			}
			line := fset.Position(fd.Pos()).Line
			if len(first) > 72 {
				first = first[:72]
			}
			bad = append(bad, fmt.Sprintf("%s:%d %s — doc opens %q", filepath.ToSlash(path), line, fd.Name.Name, first))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// STIMULUS, and it is the half a scan like this gets wrong: a walk that parsed nothing
	// reports every function as correctly documented. The floors are lower bounds on a tree
	// that only grows, and they are what makes a clean result mean something.
	// Measured 2026-09-02: 1208 functions, 1021 of them documented. Both floors are lower
	// bounds with a little headroom, on a tree that only grows — and the second is the one that
	// matters most, because dropping `parser.ParseComments` leaves the first floor untouched
	// and silently makes every doc invisible.
	if funcs < 1100 {
		t.Fatalf("the walk found only %d functions — it is not reading the tree, and a clean "+
			"result from a walk that read nothing is the vacuous green this guard exists to refuse", funcs)
	}
	if documented < 950 {
		t.Fatalf("the walk found only %d documented functions of %d — it is not parsing comments "+
			"(0), so no doc could ever be checked", documented, funcs)
	}

	// And the exemptions are still real. An entry that stops matching is a rule that quietly
	// covers one case fewer, and it reads exactly like a clean run.
	for key := range exempt {
		if !seenExempt[key] {
			t.Errorf("the exemption %q no longer matches anything: either the doc was fixed and "+
				"the entry should go, or the function was renamed and the exemption now covers "+
				"nothing while reading as though it covers something", key)
		}
	}

	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("%d doc comment(s) are attached to a function they do not name (/pending 352). "+
			"A Go doc block with no blank line before the next function's own comment binds to "+
			"THAT function, so the documented function has no doc and its neighbour carries a "+
			"paragraph about something else:\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}
