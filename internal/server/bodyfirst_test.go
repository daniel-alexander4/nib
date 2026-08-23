package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestAHandlerThatCommitsResolvesBeforeItReadsTheBody — /pending 261, made into a guard.
//
// Four handlers worked from POSTED bytes rather than from the open document, so they parsed the
// whole multipart body — up to maxPDFBytes — and ran the PDF operation before resolving the
// document the result would be installed into. The law was never broken: the resolve at the
// commit refused, and nothing landed in the wrong document. What it cost was a full parse and a
// page operation on a document that was already gone.
//
// The behavioural suite next door drives the OUTCOME at every mutating route. This checks the
// ORDERING, which no status code can show — a handler that resolves late still answers 409, just
// after spending the work. ADR-009: the rule gets one door, and the guard checks the door rather
// than the text each site prints.
//
// It walks the package instead of listing the handlers, because the population is the thing that
// changes: `/api/assemble` was the fourth member and had been excluded from the pinning inventory
// on a reason that was false, so a hand-kept list would have inherited the same hole.
func TestAHandlerThatCommitsResolvesBeforeItReadsTheBody(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["server"]
	if !ok {
		t.Fatal("setup: internal/server did not parse — every check below would pass on nothing")
	}

	population := 0
	for name, f := range pkg.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || !strings.HasPrefix(fd.Name.Name, "handle") {
				continue
			}
			var parsePos, resolvePos, commitPos token.Pos
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					if fn.Name == "parseMultipart" && !parsePos.IsValid() {
						parsePos = call.Pos()
					}
				case *ast.SelectorExpr:
					switch fn.Sel.Name {
					case "resolveDoc":
						if !resolvePos.IsValid() {
							resolvePos = call.Pos()
						}
					case "commitMutation", "commitBarrier":
						if !commitPos.IsValid() {
							commitPos = call.Pos()
						}
					}
				}
				return true
			})
			// The population is "reads a posted body AND installs the result into a document".
			// A handler that only downloads its result (handleEncrypt, handleFlags) is not in it:
			// nothing it does can reach the wrong document.
			if !parsePos.IsValid() || !commitPos.IsValid() {
				continue
			}
			population++
			if !resolvePos.IsValid() {
				t.Errorf("%s (%s) parses a body and commits, and never resolves the addressed "+
					"document at all", fd.Name.Name, name)
				continue
			}
			if resolvePos > parsePos {
				t.Errorf("%s (%s:%d) reads the body at line %d and does not resolve the addressed "+
					"document until line %d. The commit still refuses a misaddressed request, so "+
					"this is a cost rather than a hole — up to maxPDFBytes parsed and a whole PDF "+
					"operation run for a request that was never going to land. Resolve first; keep "+
					"the resolve at the commit, which is the authoritative one.",
					fd.Name.Name, name, fset.Position(fd.Pos()).Line,
					fset.Position(parsePos).Line, fset.Position(resolvePos).Line)
			}
		}
	}

	// STIMULUS: a matcher that stopped matching reports full coverage over an empty population.
	// Five handlers qualified at v1.117.126 — four that had to be fixed and handleAttachmentAdd,
	// which already resolved first and is what showed the shape was reachable.
	if population < 5 {
		t.Errorf("only %d handler(s) parse a body and commit; the census at v1.117.126 was 5, so "+
			"this guard is not seeing the population it thinks it is", population)
	}
}
