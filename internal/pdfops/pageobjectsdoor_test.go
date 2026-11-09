package pdfops

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// imageReadersExempt are the functions (file, then function) that may read `pageRuns.images` without going through
// `ReadPageObjects` (PLAN-image-edit D1's one-door pin), each for a reason: none of them LISTS pictures.
var imageReadersExempt = map[string]string{
	"pageobjects.go:pageObjectsOf":    "the door itself",
	"grouping.go:readPageTableLayout": "the Figure proposer places figures from boxes (ADR-122); it lists nothing",
	"grouping.go:readPageExportLayout": "the export model reads every picture's bytes and box to write it, not a listing; " +
		"declared (code-review 2026-11-09 #7): the slice it returns reaches exportpage.go, and a listing built there would escape this census",
	"exportvis.go:visibleRuns": "the export's visibility filter drops OFF-layer pictures in place before the export reads them",
	"structregion.go:regionElement": "the region edit takes the pictures inside a rectangle as content to tag (ADR-125), " +
		"never a listing; the listing beside it (`untaggedPieces`) takes rows from the door",
}

// TestEveryImageListingRoutesThroughReadPageObjects is PLAN-image-edit D1's census: a selector `.images` whose receiver
// is a `pageRuns` (by TYPE — `tagcommit.go`'s field of the same name is another type's) appears only in the door and
// the named exemptions. A second listing of a page's pictures would answer "which pictures" differently from the
// addresses an edit is given.
func TestEveryImageListingRoutesThroughReadPageObjects(t *testing.T) {
	found := pageRunsImageReaders(t, ".")
	if len(found["pageobjects.go:pageObjectsOf"]) == 0 {
		t.Fatal("the census found no read of pageRuns.images in pageobjects.go: it is not seeing the door, so it sees nothing")
	}
	for site, lines := range found {
		if _, ok := imageReadersExempt[site]; !ok {
			t.Errorf("%s reads pageRuns.images at line(s) %v: list a page's pictures through ReadPageObjects "+
				"(pageObjectsOf), or name the function in imageReadersExempt with its reason", site, lines)
		}
	}
	for site := range imageReadersExempt {
		if len(found[site]) == 0 {
			t.Errorf("%s is exempt but reads no pageRuns.images: drop the exemption", site)
		}
	}
}

// pageRunsImageReaders type-checks the package in dir (non-test files) and returns, per "file:function", the lines
// that select the `images` field of a `pageRuns` ("file:" for one outside any function).
func pageRunsImageReaders(t *testing.T, dir string) map[string][]int {
	t.Helper()
	fset := token.NewFileSet()
	bp, err := build.Default.ImportDir(dir, 0) // the files this platform compiles: build-tagged siblings excluded
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, name := range append(append([]string{}, bp.GoFiles...), bp.CgoFiles...) {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	exports := exportData(t, dir)
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		f, ok := exports[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return os.Open(f)
	})}
	info := &types.Info{Selections: map[*ast.SelectorExpr]*types.Selection{}}
	if _, err := conf.Check("nib/internal/pdfops", fset, files, info); err != nil {
		t.Fatalf("type-checking the package: %v", err)
	}
	out := map[string][]int{}
	for sel, s := range info.Selections {
		if sel.Sel.Name != "images" || s.Kind() != types.FieldVal {
			continue
		}
		recv := s.Recv()
		if p, ok := recv.(*types.Pointer); ok {
			recv = p.Elem()
		}
		if named, ok := recv.(*types.Named); !ok || named.Obj().Name() != "pageRuns" {
			continue
		}
		pos := fset.Position(sel.Pos())
		site := filepath.Base(pos.Filename) + ":" + enclosingFunc(files, sel.Pos())
		out[site] = append(out[site], pos.Line)
	}
	return out
}

// exportData maps each dependency of the package in dir to its compiled export data, as `go list -export` reports it.
func exportData(t *testing.T, dir string) map[string]string {
	t.Helper()
	cmd := exec.Command("go", "list", "-export", "-deps", "-f", "{{.ImportPath}} {{.Export}}", ".")
	cmd.Dir = dir
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -export: %v", err)
	}
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		if path, file, ok := strings.Cut(sc.Text(), " "); ok && file != "" {
			out[path] = file
		}
	}
	return out
}

// enclosingFunc is the name of the top-level function declaration holding pos, or "".
func enclosingFunc(files []*ast.File, pos token.Pos) string {
	for _, f := range files {
		if pos < f.Pos() || pos > f.End() {
			continue
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Pos() <= pos && pos <= fd.End() {
				return fd.Name.Name
			}
		}
	}
	return ""
}
