package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Every user-directed structure write (a commit, an edit) reaches one door — `PLAN-accessibility.md` P10.S02.
// The operations that author a tree while producing a document (OCR, forms, Markdown) do not route here;
// `internal/tagwrite`'s package comment names them.
//
// ADR-009's guard shape: routing. `internal/server` and `internal/cli` must each call `tagwrite.Commit`
// and `tagwrite.Edit`, and nothing outside `internal/tagwrite` may call `pdfops.CommitTags` or
// `pdfops.EditStructure` — a caller that did would skip the signed refusal, or compose its own.
func TestEveryStructureWriteReachesTheSignedDocumentDoor(t *testing.T) {
	doorCalls := map[string]map[string]int{"Commit": {}, "Edit": {}}
	var bypasses []string
	scanned := walkCallers(t, "internal/tagwrite", func(file, surface string, f *ast.File) {
		tagwrite, _ := importNames(f, "nib/internal/tagwrite")
		pdfops, dot := importNames(f, "nib/internal/pdfops")
		if dot {
			bypasses = append(bypasses, file+" dot-imports pdfops, which hides a CommitTags or EditStructure call from this guard")
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case tagwrite[pkg.Name] && doorCalls[sel.Sel.Name] != nil:
				doorCalls[sel.Sel.Name][surface]++
			case pdfops[pkg.Name] && (sel.Sel.Name == "CommitTags" || sel.Sel.Name == "EditStructure"):
				bypasses = append(bypasses, file+" calls pdfops."+sel.Sel.Name)
			}
			return true
		})
	})
	if scanned < 100 {
		t.Fatalf("the scan read %d non-test file(s) — too few to be the tree", scanned)
	}
	for door, by := range doorCalls {
		for _, surface := range []string{"server", "cli"} {
			if by[surface] == 0 {
				t.Errorf("internal/%s does not call tagwrite.%s, so a structure write there does not reach the "+
					"door that refuses a signed document", surface, door)
			}
		}
	}
	for _, b := range bypasses {
		t.Errorf("%s directly. Call the tagwrite door, so a signed document is refused there and not "+
			"re-checked here (ADR-009)", b)
	}
}

// walkCallers parses every non-test Go file of the MODULE — not `internal/` alone, because `cmd/` and
// `mdpdf/` can call a door as well as anything under it does — except the door's own package, and hands
// each to fn with its surface (`server` for internal/server, `nib` for cmd/nib, `mdpdf`). Vendored and
// third-party trees are not this module's callers and are skipped. It returns how many files it read.
func walkCallers(t *testing.T, door string, fn func(file, surface string, f *ast.File)) int {
	t.Helper()
	fset := token.NewFileSet()
	scanned := 0
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "third_party", "testdata", "vendor":
				return filepath.SkipDir
			}
			if filepath.ToSlash(p) == door {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		scanned++
		elems := strings.Split(filepath.ToSlash(p), "/")
		surface := elems[0]
		if len(elems) > 2 && (elems[0] == "internal" || elems[0] == "cmd") {
			surface = elems[1]
		}
		fn(filepath.ToSlash(p), surface, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return scanned
}

// importNames reports the local names a file refers to the package at importPath by — its own
// name, or whatever alias the import gives it — and whether the file dot-imports it. Matching the
// identifier `pdfops` literally (as this guard and uadoor_test.go did until /pending 808) misses
// `po "nib/internal/pdfops"`, and a guard a rename walks past is not checking the door.
func importNames(f *ast.File, importPath string) (names map[string]bool, dot bool) {
	names = map[string]bool{}
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil || p != importPath {
			continue
		}
		switch {
		case im.Name == nil:
			names[path.Base(p)] = true
		case im.Name.Name == ".":
			dot = true
		case im.Name.Name != "_":
			names[im.Name.Name] = true
		}
	}
	return names, dot
}
