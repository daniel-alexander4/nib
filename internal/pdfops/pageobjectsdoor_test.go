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
	"exportvis.go:visibleRuns":       "the export's visibility filter drops OFF-layer pictures in place before the export reads them",
	"pageobjectedit.go:taggingOf":    "the edit door's structure verdict for each marked picture, which the listing then carries (`pageObjectsOf`)",
	"pageobjectedit.go:tagVerdict":   "the same verdict, one picture",
	"pageobjectedit.go:removeOnPage": "the edit door reads the removed picture's box in user space to count links over it; the address came from the listing",
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
	fset, files, info := typeCheckedPackage(t, dir)
	out := map[string][]int{}
	for sel, s := range info.Selections {
		if sel.Sel.Name != "images" || s.Kind() != types.FieldVal {
			continue
		}
		if !receiverIs(s, "pageRuns") {
			continue
		}
		pos := fset.Position(sel.Pos())
		site := filepath.Base(pos.Filename) + ":" + enclosingFunc(files, sel.Pos())
		out[site] = append(out[site], pos.Line)
	}
	return out
}

// receiverIs says the selection's receiver is the named type (or a pointer to it).
func receiverIs(s *types.Selection, name string) bool {
	recv := s.Recv()
	if p, ok := recv.(*types.Pointer); ok {
		recv = p.Elem()
	}
	named, ok := recv.(*types.Named)
	return ok && named.Obj().Name() == name
}

// typeCheckedPackage parses and type-checks the package in dir (non-test files, this platform's).
func typeCheckedPackage(t *testing.T, dir string) (*token.FileSet, []*ast.File, *types.Info) {
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
	info := &types.Info{Selections: map[*ast.SelectorExpr]*types.Selection{}, Uses: map[*ast.Ident]types.Object{}}
	if _, err := conf.Check("nib/internal/pdfops", fset, files, info); err != nil {
		t.Fatalf("type-checking the package: %v", err)
	}
	return fset, files, info
}

// pictureCutters are the edit door's own functions and the named exemptions: a function that both reads a picture's
// operator span (`drawnImage.span`, or the listing's `PageObject.span`) and writes a page's content (`setPageContent`) may only be one of these
// (PLAN-image-edit P01.S02, plan-review pin: guard shape). Anything else would cut a picture out of a page without
// the address check, the structure, the prune or the after-check.
var pictureCutters = map[string]string{
	"pageobjectedit.go:removeOnPage": "the door: `EditPageObjects` → `removeObjects` → here",
	"structregion.go:regionElement": "exempt — the region edit (ADR-125) brackets a picture's span in marked content to tag it; " +
		"it inserts around the operator and removes nothing",
}

// TestEveryPageObjectEditRoutesThroughTheDoorInsidePdfops is the bypass half of the page objects' door census
// (the routing half is the repo root's `pageobjectdoor_test.go`): by TYPE, every function that selects a
// `drawnImage`'s or a `PageObject`'s `span` and calls `setPageContent` is the door or a named exemption.
func TestEveryPageObjectEditRoutesThroughTheDoorInsidePdfops(t *testing.T) {
	fset, files, info := typeCheckedPackage(t, ".")
	spans, writes := map[string]bool{}, map[string]bool{}
	for sel, s := range info.Selections {
		if sel.Sel.Name == "span" && s.Kind() == types.FieldVal && (receiverIs(s, "drawnImage") || receiverIs(s, "PageObject")) {
			spans[filepath.Base(fset.Position(sel.Pos()).Filename)+":"+enclosingFunc(files, sel.Pos())] = true
		}
	}
	for id, obj := range info.Uses {
		if fn, ok := obj.(*types.Func); ok && fn.Name() == "setPageContent" && fn.Pkg() != nil && fn.Pkg().Path() == "nib/internal/pdfops" {
			writes[filepath.Base(fset.Position(id.Pos()).Filename)+":"+enclosingFunc(files, id.Pos())] = true
		}
	}
	if len(writes) < 5 {
		t.Fatalf("the census found %d function(s) calling setPageContent — too few to be seeing the package", len(writes))
	}
	if !spans["pageobjectedit.go:removeOnPage"] || !writes["pageobjectedit.go:removeOnPage"] {
		t.Fatal("the census does not see the door itself (removeOnPage reading a picture's span and writing the page): it sees nothing")
	}
	for site := range spans {
		if !writes[site] {
			continue
		}
		if _, ok := pictureCutters[site]; !ok {
			t.Errorf("%s reads a picture's operator span and writes the page's content: cut a picture out of a page "+
				"through pdfops.EditPageObjects (the objedit door), or name the function in pictureCutters with its reason", site)
		}
	}
	for site := range pictureCutters {
		if !spans[site] || !writes[site] {
			t.Errorf("%s is named in pictureCutters but no longer both reads a picture's span and writes a page: drop it", site)
		}
	}
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
