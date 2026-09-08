package nib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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

// TestEveryJSDocCommentNamesItsOwnDeclaration — /pending 383, the web half of the rule above.
//
// # Why a second test rather than a wider walk
//
// The Go guard `SkipDir`s `web`, and that was correct: `go/parser` cannot read JavaScript, so
// there was no way to extend the walk. This one reads the same rule off the text. It is in this
// file because it is the same rule — ADR-009's one-door principle is about the RULE, and a
// reader who finds one half here must find the other.
//
// # The Go rule does not transfer, and mirroring it was measured before it was rejected
//
// Go's convention is that a doc opens with the name of the thing it documents, so the Go half
// asks only that the name appear in the first line. Applied to `web/*.js` that rule flags
// **131 of 302** doc blocks — because this file's actual convention is a section banner
// (`--- open / load ---`, `── The sidebar is an accordion of cards ──`) or a topic sentence
// ("Reduce file size. Renders the result…"). A guard with 131 flags and 2 real ones polices
// nothing; it would have been turned off within a day.
//
// So the rule here is narrower and fires only where the text COMMITS to a subject: a doc block
// whose first line opens `// <name>` where `<name>` is declared at column 0 somewhere in the
// same file. That block has named what it is about, and it must sit on that declaration. A
// banner names nothing and is silent; a topic sentence names nothing and is silent; and both of
// those are the cases the Go rule got wrong.
//
// **Both clauses are load-bearing and each was found by running it.** Without "declared in this
// file", every sentence opening with an ordinary capitalised word ("Presets set the label
// text…", "Move the selection to the front…") claims a subject it does not have. Without
// "column 0", a nested comment inside a function body binds to the next nested declaration.
//
// # The blind spot, which is declared because it is real and was measured
//
// The rule reads the block's FIRST line, so it cannot see a doc buried in the middle of a
// block — `documentText`'s three lines sat as the third paragraph under a `--- Compare two
// PDFs ---` banner, above `pageTexts`, and this guard would never have flagged them. The wider
// rule that does catch it — any line in the block opening with a declared name — was written
// and run rather than reasoned about: on the fixed tree it flags **22**, and all 22 are
// mid-sentence continuations ("// view.redactMarks (page-content top-left fractions…",
// "// relayoutRedactMarks does it for all rendered pages, and both run on…") where the previous
// line ends and the next begins with a name. Twenty-two false positives to catch one instance
// is a guard that gets ignored, so the narrow rule stands and this paragraph is the cost.
//
// # What it found
//
// Twelve, in a tree nothing had ever parsed for this: six where a companion `let`/`const` had
// been inserted between a doc and its function, five glued pairs, and one in `web/detect.js` —
// which a first cut of this guard could not see at all, because its regex did not accept
// `export function` and so reported **0 declarations in a 625-line file**. That is this guard's
// own vacuous-green shape, caught by the floors below rather than by inspection. Eleven of the
// twelve are caught by this guard on a replay against the pre-change tree; the twelfth is the
// blind spot above, and it was found by the wider scan before that scan was rejected.
func TestEveryJSDocCommentNamesItsOwnDeclaration(t *testing.T) {
	// A declaration at column 0: `function f`, `async function f`, `export function f`,
	// `const/let/var x =`, with an optional `export`. Column 0 is what makes it a top-level
	// declaration rather than a local inside some function's body.
	decl := regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)|^(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=`)
	// A doc block's first line claiming a subject: `// name …`.
	opens := regexp.MustCompile(`^//\s+([A-Za-z_$][\w$]*)\b`)

	files, err := filepath.Glob("web/*.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 2 {
		t.Fatalf("the glob found %d file(s) in web/ — it is not reading the tree", len(files))
	}

	var bad []string
	blocks, claiming := 0, 0

	for _, path := range files {
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatal(rerr)
		}
		lines := strings.Split(string(src), "\n")

		// Every identifier declared at column 0 in this file. Built first, because the rule's
		// "is this a name or an English word" test is membership in this set.
		declared := map[string]bool{}
		for _, ln := range lines {
			if m := decl.FindStringSubmatch(ln); m != nil {
				declared[m[1]+m[2]] = true // exactly one group matches; the other is ""
			}
		}

		for i, ln := range lines {
			m := decl.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			name := m[1] + m[2]
			if i == 0 || !strings.HasPrefix(lines[i-1], "//") {
				continue
			}
			j := i
			for j > 0 && strings.HasPrefix(lines[j-1], "//") {
				j--
			}
			blocks++
			o := opens.FindStringSubmatch(lines[j])
			if o == nil || !declared[o[1]] {
				continue // names no subject — a banner or a topic sentence, and not this rule's business
			}
			claiming++
			if o[1] == name {
				continue
			}
			first := lines[j]
			if len(first) > 78 {
				first = first[:78]
			}
			bad = append(bad, fmt.Sprintf("%s:%d %s — the doc above it opens about %q: %s", path, i+1, name, o[1], first))
		}
	}

	// STIMULUS, and the second floor is the load-bearing one — exactly as the Go half found.
	// Measured 2026-09-08: 326 doc blocks, 194 of which claim a subject by name. A regex that
	// stops matching declarations leaves the first floor plausible while making every doc
	// invisible; a regex that stops recognising a claimed subject leaves the walk finding
	// blocks and checking none of them. `web/detect.js` proved this is not hypothetical — the
	// first cut of `decl` had no `export` branch and reported 0 declarations for that file.
	if blocks < 280 {
		t.Fatalf("the scan found only %d doc blocks above a top-level declaration — it is not "+
			"reading the tree, and a clean result from a scan that read nothing is a vacuous green", blocks)
	}
	if claiming < 165 {
		t.Fatalf("the scan found only %d doc blocks that name a declared subject, of %d blocks — "+
			"it is finding blocks and checking none of them, which reads exactly like a clean run",
			claiming, blocks)
	}

	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("%d JS doc comment(s) sit above a declaration they do not name (/pending 383). "+
			"The block opens by naming a subject that is declared elsewhere in the same file, so "+
			"the named function has no doc and this declaration carries a paragraph about "+
			"something else:\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}
