package sign

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestAWalkBoundaryIsReadOnce — a screened walk boundary paid pdfcpu's read and the sweep twice: once in
// `boundaryCandidate` to screen it, and again in `Revisions(prefix)` to re-verify it (/pending 803), up to
// `maxBoundaryScreens` times a request. The screen's read now carries through to `verifySwept`. The guard is of the
// route (ADR-009): `walkBoundaries` reads a prefix only through `boundaryCandidate` and re-verifies through
// `verifySwept`, never through a door that reads it again.
func TestAWalkBoundaryIsReadOnce(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "signedrevision.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]int{}
	found := false
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "walkBoundaries" {
			continue
		}
		found = true
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok {
					calls[id.Name]++
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("signedrevision.go has no walkBoundaries: the guard reads nothing")
	}
	for _, again := range []string{"Revisions", "verifyIndexed", "pdfcpuRead", "pdfcpuCanRead", "sweepRevisions", "sweep"} {
		if calls[again] > 0 {
			t.Errorf("walkBoundaries calls %s: a boundary boundaryCandidate has already read is read again", again)
		}
	}
	if calls["boundaryCandidate"] == 0 || calls["verifySwept"] == 0 {
		t.Errorf("walkBoundaries calls boundaryCandidate %d and verifySwept %d times, want both: the screen's read is "+
			"not the one the re-verify continues from", calls["boundaryCandidate"], calls["verifySwept"])
	}
}
