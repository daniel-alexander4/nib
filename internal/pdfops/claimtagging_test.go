package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The one door that claims tagging — P06's phase close, ADR-009.

// TestEveryClaimOfTaggingGoesThroughTheOneDoor — ADR-009's guard shape, which is the whole point of
// the refactor it guards.
//
// # Why this is a routing check and not an agreement check
//
// ADR-009's words: *"The guard asserts routing through the door, not the text each site prints —
// eight copies checked for agreement say nothing about a ninth site added without one."* By the end
// of P06 there were FOUR doors building a tree, each with its own copy of the same three-part law,
// and all four AGREED. A test comparing them would have been green while saying nothing about the
// fifth door P07 will add — which is the door that matters, because whoever writes it will read one
// of the four and copy what they found.
//
// So the assertion is: nothing writes `/MarkInfo` except `claimTagging`. A new tagging door either
// calls it or goes red here.
func TestEveryClaimOfTaggingGoesThroughTheOneDoor(t *testing.T) {
	const door = "claimTagging"

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	// funcsWriting maps a function name to the file it is in, for every function that assigns
	// "MarkInfo" or "Marked" into something.
	funcsWriting := map[string]string{}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		scanned++
		for _, d := range f.Decls {
			fd, isFunc := d.(*ast.FuncDecl)
			if !isFunc || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				idx, isIndex := n.(*ast.IndexExpr)
				if !isIndex {
					return true
				}
				lit, isLit := idx.Index.(*ast.BasicLit)
				if !isLit || lit.Kind != token.STRING {
					return true
				}
				switch strings.Trim(lit.Value, `"`) {
				case "MarkInfo", "Marked":
					funcsWriting[fd.Name.Name] = name
				}
				return true
			})
		}
	}

	// **Stimulus floor.** A scan that parsed nothing, or that stopped matching the shape it looks
	// for, reports zero writers — indistinguishable from a repo where the rule is perfectly kept.
	if scanned < 10 {
		t.Fatalf("the scan read %d non-test .go file(s) in this package, too few to be it — every "+
			"assertion below would be vacuous", scanned)
	}
	if len(funcsWriting) == 0 {
		t.Fatal("the scan found NO function writing /MarkInfo anywhere in this package. Either the " +
			"key is spelled differently now or the matcher has stopped working; a clean report and " +
			"a broken scan are the same output")
	}

	// `claimTagging` is the door; `inspectTags` and the fate table READ the key and are not claims.
	readers := map[string]bool{
		door:          true,
		"inspectTags": true,
	}
	var offenders []string
	for fn, file := range funcsWriting {
		if readers[fn] {
			continue
		}
		offenders = append(offenders, fn+" ("+file+")")
	}
	if len(offenders) > 0 {
		t.Errorf("these functions write the catalog's claim of tagging without going through %s: "+
			"%s.\n\tADR-009: a rule holding at more than one call site is written ONCE and every "+
			"site calls it. The three-part law — /MarkInfo, D4's tier, and the orphaned() "+
			"post-condition — had four agreeing copies at the P06 close, and agreement between "+
			"four says nothing about a fifth door added without one. Call %s, or name the function "+
			"here with the reason it is a reader rather than a claim.",
			door, strings.Join(offenders, ", "), door)
	}

	// And the door itself must still be there to route through.
	if funcsWriting[door] == "" {
		t.Errorf("%s no longer writes /MarkInfo, so this guard is asserting that nothing claims "+
			"tagging — which would be green on a repo that cannot tag at all", door)
	}
}

// TestTheDoorRefusesToClaimWhatTheContentCannotSupport — the door's own post-condition.
//
// Every caller's honest answer to an unsupportable claim is the same, which is why the door returns
// `ok` false rather than an error and why the check lives here rather than four times over.
func TestTheDoorRefusesToClaimWhatTheContentCannotSupport(t *testing.T) {
	// A document with no tree at all: claiming tagging over it is `orphaned()` by construction.
	plain, cerr := CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[` +
		`{"value":"no structure here","anchor":"TopLeft","position":[72,720],` +
		`"font":{"name":"Helvetica","size":12}}]}}}}`))
	if cerr != nil {
		t.Fatal(cerr)
	}
	// Floor: the fixture really has no tree, or "refused" below means nothing.
	if s := inspectTags(plain); s.tree {
		t.Fatalf("setup: the fixture already carries a structure tree (%+v), so the refusal below "+
			"is not being tested", s)
	}
	out, ok, err := claimTagging(plain, sourceInferred)
	if err != nil {
		t.Fatalf("the door errored instead of refusing: %v", err)
	}
	if ok {
		t.Errorf("the door claimed tagging over a document with no tree. That is ADR-031 law 1's "+
			"first case and `orphaned()` exists to catch it: a reader told a document is tagged "+
			"stops reaching for the fallbacks it would otherwise use (%+v)", inspectTags(out))
	}
}
